# icebreaker 现场抽奖系统设计（匿名签到 + Keycloak 管理台）

本文是 grilling 环节确认下来的设计树。**未确认前不写代码**；文末"待确认"两条定了就开工。

## 1. 目标与非目标

目标

- 现场大屏抽奖：大屏公开可访问，任何时候打开都能看到当前状态，不需要登录。
- 组织账号签到进池：到场的人用组织账号登录后一键签到，姓名取账号里的真名。
- 管理端远程驱动大屏：发起轮次、推进/回退揭晓、跳过动画，全部在管理台操作。
- Keycloak 保护所有写操作：签到要"登录过"，管理台要"登录 + 白名单"（RBAC）。
- 名单、轮次、中奖结果落 SQLite，后端重启不丢。

非目标（明确不做）

- 用户表、会话表、refresh token（登录态就是换发的 HMAC token）。
- 结果墙、"我的获奖情况"、按姓名查询中奖。
- 签到防伪自建方案：不做验证码、不做设备指纹、不做审核队列（身份交给组织账号）。
- WebSocket / SSE 长连接（大屏用轮询）。
- 免 Keycloak 的 dev 旁路（沿用 wordle 原则：没有旁路）。
- 多场次活动管理（"清空整场"即换场）。

## 2. 已确认决策（决策日志）

| 决策 | 结论 |
| --- | --- |
| 后端 | 新建 Go 服务，照搬 wordle 的 `config` / `auth` / `store` 结构与 `go:embed` 托管方式 |
| Keycloak | 复用现有 Keycloak 的 **master realm**，只新建 client `icebreaker`（confidential + standard flow；不带 PKCE，靠 client secret 与 state/nonce） |
| 账号来源 | 部员账号由组织统一注册流程建立；icebreaker 不开放自助注册 |
| 谁登录 | 所有人都登录：部员用组织账号登录后签到（姓名取账号真名），管理员用它进管理台。大屏仍公开 |
| 拓扑 | 用户设备（手机/电脑均可）扫码签到；管理端（另一台设备或同一台）驱动大屏 |
| 大屏驱动 | 管理端控制推进，大屏只显示（轮询） |
| 抽奖权威 | 服务端算结果并落库；前端只负责演动画 |
| RBAC | `config.yaml` 里的 `admin_subs` 白名单；白名单为空则无人可管理（fail closed） |
| 持久化 | SQLite 单文件（WAL + 单写连接） |
| 名单来源 | 签到为主，管理台可手动加单人、可文件导入（追加/覆盖） |
| 重置 | 两级：重置中奖（保留名单）/ 清空整场（连名单一起），硬删 + 二次确认 |
| seed | 服务端 `crypto/rand` 生成，管理台可覆盖；seed 与快照 hash 只在管理台展示与导出 |
| 头像 | 不引入。动画只用 姓名 / theme / 树莓娘图，后两者服务端入池时随机定 |
| 昵称 | `name` claim，空则回退 `preferred_username` → `sub` |
| 大屏空闲态 | "已签到 M 人" + 常驻签到二维码 |
| 前端托管 | Vue 构建出 dist，由 Go 二进制同源 `go:embed` 托管；令牌只存内存，缺失/401 静默重走 `/auth/login` |
| 现场地址 | 部署到服务器域名（HTTPS），LAN 配置 + 第二个注册回调作为兜底 |
| 旧页面 | `/directors`、`/lottery-random`、`/lottery-ordered` 保留原样、不加鉴权 |

## 3. 角色与权限矩阵

能力矩阵（`internal/rbac`，代码里就是这张表）：

| 能力 | 未登录 | user | minister | operator | admin |
| --- | :---: | :---: | :---: | :---: | :---: |
| 看大屏状态 / 大屏轮次 / 部长介绍列表 | ✓ | ✓ | ✓ | ✓ | ✓ |
| 签到（姓名取账号真名，一个账号只算一次） | ✗ | ✓ | ✓ | ✓ | ✓ |
| 看自己是谁（`GET /api/v1/me`） | ✗ | ✓ | ✓ | ✓ | ✓ |
| 写/删自己的部长介绍 | ✗ | ✗ | ✓ | ✗ | ✓ |
| 开关签到、名单增删改导入、发起抽奖、大屏控制、结束本轮 | ✗ | ✗ | ✗ | ✓ | ✓ |
| 重置中奖 / 清空整场 / 导出 | ✗ | ✗ | ✗ | ✓ | ✓ |
| 查看 / 授予 / 撤销角色 | ✗ | ✗ | ✗ | ✗ | ✓ |

**任何账号登录后默认就是 `user`**；`minister` 与 `operator` 是并列的两摊，互不覆盖；
`admin` 是唯一能管角色的角色，同时拥有全部能力。

角色怎么来：配置文件 `auth.admin_subs` 里的账号是**根管理员**（永远 admin、库里不可撤销），
对应老说法的"白名单"；其余角色由 admin 在管理台点选，落在 `roles` 表（一个账号一个角色）。
权限一律在服务端判定（`Server.gate(能力, handler)`），前端只做可见性。

## 4. 认证与管理员判定

沿用你给的 auth 包（`wordle-competition/internal/auth`），只服务管理台：

1. `GET /auth/login`：生成无状态 `state = nonce.hmac(nonce)`，nonce 同时作 OIDC nonce，302 到 Keycloak。
2. Keycloak 回跳 `GET /auth/callback`：`CheckState` → `Exchange`（验签/issuer/audience/exp/nonce）→ 取 `sub` 与昵称 → 签发 HMAC token。
3. 302 到 `/?token=<hmac>#/admin`；SPA 读出 token 存内存、把 URL 还原成 `/#/admin`。
4. 管理接口带 `Authorization: Bearer <token>`；`Verify` 失败返回 401，前端静默重走 `/auth/login`（Keycloak SSO cookie 让它无感）。
5. 管理员判定：`sub ∈ config.auth.admin_subs`。不在名单里 → 403，前端明说"该账号无管理权限"。

配置项（`config.yaml`，照 wordle 风格，必填留空即启动失败）：

```yaml
server:
  addr: ":8080"
db:
  path: "data/icebreaker.db"
auth:
  hmac_secret: ""              # openssl rand -base64 32
  token_ttl: 2h
  admin_subs: []               # master realm 里管理员的 sub；空 = 无人可管理
  display_name_claim: "name"
  oidc:
    issuer: "https://<keycloak>/realms/master"
    client_id: "icebreaker"
    client_secret: "..."
    redirect_url: "https://<现场地址>/auth/callback"
```

## 5. 数据模型（SQLite）

```sql
-- 签到名单：一人一行，允许重名（不防伪，不做唯一约束）
participants(
  id            INTEGER PRIMARY KEY,
  name          TEXT NOT NULL,          -- 展示用（归一化后）
  theme         TEXT NOT NULL,          -- 入池时随机定
  shumeiniang   TEXT NOT NULL,          -- 入池时随机定
  source        TEXT NOT NULL,          -- checkin | manual | import
  created_at    INTEGER NOT NULL
);

-- 轮次：一次"发起抽奖"一行
rounds(
  no            INTEGER PRIMARY KEY,    -- 1..N
  label         TEXT NOT NULL DEFAULT '',-- "三等奖"/"特别奖"
  draw_count    INTEGER NOT NULL,
  remove_winners INTEGER NOT NULL,      -- 中奖者是否退出后续轮次
  seed          TEXT NOT NULL,
  snapshot_hash TEXT NOT NULL,          -- sha256(参与名单+seed+no)
  snapshot      TEXT NOT NULL,          -- 参与人 id 顺序（复核用）
  created_at    INTEGER NOT NULL
);

-- 中奖记录：按名次存
draws(
  round_no      INTEGER NOT NULL,
  position      INTEGER NOT NULL,       -- 1..draw_count，决定大屏顺序
  participant_id INTEGER NOT NULL,
  PRIMARY KEY (round_no, position)
);

-- 运行态单行表：重启后大屏与签到开关状态不丢
state(
  id             INTEGER PRIMARY KEY CHECK (id = 1),
  signup_open    INTEGER NOT NULL DEFAULT 1,
  current_round  INTEGER,               -- NULL = 待机
  revealed_index INTEGER NOT NULL DEFAULT -1,  -- -1 = 还没揭晓任何人
  skip_seq       INTEGER NOT NULL DEFAULT 0    -- 管理端"跳过动画"信号
);
```

候选池 = `participants` 里所有参与人 **减去** 历史轮次中 `remove_winners=1` 的中奖者。

## 6. 接口清单

公开（无鉴权）

| 接口 | 说明 |
| --- | --- |
| `POST /api/v1/checkin` | 入池。**需要登录**；姓名取账号真名，不接受前端传名字；一个 sub 只会有一条记录（重复签到返回 `already:true`，不报错）。`signup_open=0` 返回 409 |
| `GET /api/v1/me` | 需要登录。返回 `{sub, name, admin, checkedIn}`，前端据此决定是"确认签到"还是"已经签过" |
| `GET /api/v1/screen/state` | 大屏轮询：`{signupOpen, participantCount, round: {no, label, revealedIndex, skipSeq} \| null}` |
| `GET /api/v1/screen/round` | 当前轮完整有序中奖名单 `[{name, theme, shumeiniang}]`（大屏开轮时预载，之后本地播放） |

管理（Bearer + `admin_subs`）

| 接口 | 说明 |
| --- | --- |
| `GET /auth/login`、`GET /auth/callback` | 登录入口与回调 |
| `GET /api/v1/admin/state` | 全名单 + 轮次历史 + 当前运行态 + seed/快照 hash |
| `POST /api/v1/admin/checkin/{open,close}` | 开关签到 |
| `POST /api/v1/admin/participants` `{name}` | 手动加单人 |
| `DELETE /api/v1/admin/participants/{id}` | 删人 |
| `POST /api/v1/admin/import` | 文件导入名单（`mode=append\|replace`） |
| `POST /api/v1/admin/rounds` `{count, label, removeWinners, seed?}` | 发起抽奖（服务端算 + 落库 + 置为 current_round、revealed_index=-1） |
| `POST /api/v1/admin/rounds/{no}/cursor` `{revealedIndex}` | 推进/回退一位 |
| `POST /api/v1/admin/rounds/{no}/skip` | 跳过当前动画（`skip_seq++`） |
| `POST /api/v1/admin/rounds/current/end` | 结束本轮，回到待机 |
| `POST /api/v1/admin/reset` `{scope: "winners"\|"all"}` | 重置中奖 / 清空整场 |
| `GET /api/v1/admin/export/{participants,results}` | 导出 TXT/CSV |

大屏轮询间隔 1.5s；`round.no` 变化 → 重新 `GET /screen/round` 预载名单；`revealedIndex`/`skipSeq` 变化 → 播放或跳动画。轮询失败时大屏显示"连接中断"，不静默停留。

## 7. 抽奖算法与可复核性

- seed：`crypto/rand` 生成 16 字节 base64；管理台可手填覆盖（覆盖记日志）。
- 洗牌：对候选池快照做 Fisher-Yates，随机源用 seed 派生的 `PCG`/`ChaCha20`，**替换掉现有前端 `Math.sin` 的弱实现**；取前 `draw_count` 个。
- 快照与 hash 落库，只在管理台展示/导出，大屏不露——给事后"没作弊"留证据，不给现场剧透。

## 8. 前端改动

路由

| 路由 | 鉴权 | 内容 |
| --- | --- | --- |
| `/#/lottery` | 公开 | 大屏：待机 = 已签到 M 人 + 签到二维码；有轮次 = 预载名单后按服务端游标播放动画 |
| `/#/checkin` | 公开 | 一个输入框 + 一个按钮；`localStorage` 记已签，签过不再显示表单 |
| `/#/admin` | 需登录 + 白名单 | 管理台：签到开关、名单、加人/删人/导入、发起抽奖、游标推进、跳过、结束本轮、重置、导出、seed/hash 只读展示 |
| `/#/directors`、`/#/lottery-random`、`/#/lottery-ordered` | 公开 | 原样不动 |

大屏收敛

- 移除页面上的"导入文件/导出名单/清空名单/一发入魂"与键盘驱动（⬇️/⬆️/空格）：这些能力全部搬到管理台，大屏只跟随服务端状态。
- 保留 `F` 全屏。
- `RecruitAnimation` 的 8 个 theme 与 5 张树莓娘图清单从服务端下发（现在写死在两个 .vue 文件里），前台不再各自维护。

开发期：`vue.config.js` 加 `devServer.proxy`，把 `/api`、`/auth` 代理到 Go 服务，避免跨域。

## 9. 部署与演练

- 目录：`go.mod` 放在 `introduction/`（`go:embed` 不能引用上级目录，dist 必须在模块内），代码放 `introduction/cmd/icebreaker/`、`internal/`，`npm run build` 产出 `introduction/dist`。
- 交付：Dockerfile（构建 Go + 内嵌 dist）+ `docker-compose.yml`（挂载 `config.yaml` 与 `data/`），照 wordle 的 Traefik 方式上线。
- Keycloak：client `icebreaker` 登记回调 `https://<现场域名>/auth/callback`（LAN 兜底再登记一条对应地址）。
- 验收演练：冷启动 → 开签到 → 签到 3 人（另加 1 个手动、1 个导入）→ 发起轮次（2 人）→ 推进/回退/跳过 → 结束本轮 → 发起第二轮（验证中奖者已出局）→ 重置中奖 → 清空整场 → 后端重启后状态不丢。
- 回滚：前端 dist 每次构建留 tag；出问题换回上一版静态页 + 关掉 admin 路由即可，数据库文件独立不受影响。

## 10. 明确接受的风险

1. 公开接口能拿到本轮完整中奖顺序，懂 curl 的人可以提前剧透（A1(a) 信任现场）。
2. 签到身份交给组织账号：没有账号（或登不上）的人进不了池，只能在管理台手动加人；
   一个人若有多个组织账号则能签多次——这不解决，属于账号治理问题。
3. 大屏与管理端都依赖同一后端：后端挂了台上就停（大屏会明确显示"连接中断"，不做离线接管）。
4. 大屏页面公开，等于把在场人员姓名暴露在公网 URL 上（见待确认 ①）。
5. 只支持单场活动：换场靠"清空整场"。

## 11. 已确认（2026-10-11，开工前最后两条）

① **空闲态**：大屏只显示"已签到 M 人 + 签到二维码"，姓名只在管理台可见。
（原先选过的"投影页照旧显示完整名单"作废。）

② **签到开关默认值**：默认开启，部署即可签到，管理员随时关。

## 12. 实施顺序

M1 auth 包移植 + 管理台门禁（含 fail-closed 白名单）
M2 SQLite 数据层 + 签到/名单接口
M3 抽奖算法 + 轮次/游标/skip 接口
M4 大屏改造 + 签到页 + 管理台页面
M5 构建内嵌、Docker/compose、本地 Keycloak 联调与演练

## 13. 实现记录（M1–M5 已完成，2026-10-11）

已落地的代码

| 部分 | 位置 |
| --- | --- |
| 配置 | `internal/config/config.go`、`config.example.yaml` |
| Keycloak 登录 + HMAC token | `internal/auth/auth.go`（沿用 wordle 的实现，只服务管理台） |
| 抽奖与快照凭证 | `internal/lottery/lottery.go`（ChaCha8 洗牌、sha256 快照） |
| SQLite 四张表 | `internal/store/store.go` |
| 接口与静态托管 | `internal/api/api.go` |
| 入口 | `cmd/icebreaker/main.go` |
| 前端 | `src/pages/{Lottery,CheckinPage,AdminPage}.vue`、`src/api.js` |
| 构建 | `Dockerfile`、`docker-compose.yml`、`dist/.gitkeep` |

验证情况：`go vet` / `go test ./...` 全绿（api 层覆盖签到、三道鉴权门、发起抽奖、
游标与夹取、跳过、结束本轮、删人/导入/重置、导出、静态托管）；`npm run build`
与 `eslint` 无错；本地起二进制跑通了"签到 → 发起抽奖 → 大屏取名单 → 推游标 →
导出结果"的端到端（管理接口用本地自签 token，**没有经过真实 Keycloak**）。

与原设计不同的地方（都是实现时才发现的选择）

1. **二维码由服务端出**：`GET /api/v1/qr?data=...` 返回 PNG，前端不引第三方库。
2. **大屏需要一次点击解锁自动播放**：旧版靠方向键驱动，每次播放都在用户手势里；
   现在轮到管理端驱动，浏览器会拦带声音的自动播放。待机页那个"点击进入全屏演示"
   按钮同时承担解锁职责（若 `play()` 仍被拦，会显示提示并直接亮出名字）。
3. **`/#/admin` 无 token 时自动跳 Keycloak**，且一个标签页只自动跳一次，避免
   "没权限 → 跳登录 → 回来还是没 token" 来回弹。
4. **中奖记录冗余存展示字段**（姓名/主题/树莓娘图）：删人或改名单不会牵动已经产生的
   结果，导出与复核拿到的是当时那份。
5. **`revealed_index` / `skip_seq` 存在轮次行上**而不是 state 行，便于逐轮回看历史。
6. **十连抽分组展示暂未实现**：管理端现在是逐位推进，轮次标签与 `remove_winners`
   都在，但"一抽/十连抽"的展示分组没有了。要加回来，让游标一次步进 groupSize 即可。
7. **仓库只提交 `dist/.gitkeep`**：没构建前端时 `go build` 照样过，首页返回 503
   提示页说明书式引导；构建产物被 `.gitignore` 忽略，不入库。

上线前还需要你这边做的三件事（我拿不到）

1. 在组织 Keycloak 的 master realm 建 confidential client `icebreaker`：
   Client authentication = On、Standard flow = On，PKCE 留空（本服务的回调不发
   code_challenge），Valid redirect URI 填现场访问地址的 `/auth/callback`。
2. 把 issuer / client_id / client_secret / redirect_url 填进 `config.yaml`。
3. 把管理员的 `sub` 填进 `auth.admin_subs`（登录一次后看服务端日志即可拿到）。

## 14. 联调中发现并修掉的两个坑（2026-10-11）

第一次真实登录 + 大屏走查时暴露出来的，都不是设计问题，是实现细节没想透：

1. **轮次号必须单调递增。** 原先按 `MAX(no)+1` 续号，而"重置中奖"会删掉所有轮次，
   于是重置后下一轮又叫 1 号。大屏判断"是不是新一轮"只看轮次号，所以在
   "重置中奖 → 立刻再抽"（两次操作挤在 1.5 秒轮询间隔之内，大屏没看到待机那一帧）
   时会接着播**上一轮的人**。现在轮次号来自 `state.next_round` 单调计数，重置不回退；
   `store.Open` 会给老库就地补这一列（`pragma_table_info` 检查 + `ALTER TABLE` +
   按 `MAX(no)+1` 回填），现场升级二进制不用删库。回归测试：`TestRoundNoKeepsIncreasingAfterReset`、
   `TestOpenMigratesOldStateTable`。

2. **自动播放被拦时要有看门狗。** `RecruitAnimation.play()` 只在视频 `ended` 时 resolve，
   而且它把内部 `videoPlayer.play()` 的拒绝自己 `catch` 掉了 —— 浏览器拦下带声音的
   自动播放时，外层 promise **既不 resolve 也不 reject**，大屏就永远停在那一位上
   （只能靠管理台按"跳过动画"救场），表现就是"过场动画没了"。现在每次播放挂一个
   1.5 秒闹钟：还 `paused` 就认为被拦，立刻亮出名字 + 顶部红条提示，并记住是哪一位；
   用户第一次点屏幕或按键时自动把这一位重播一遍（Chrome 规则：文档上有过一次用户手势
   就允许带声音的自动播放）。旧版是在大屏上按方向键驱动，那次按键天然就是手势，
   所以从没暴露过这个问题；改由管理端远程推进后才有。**现场开服后先在大屏上点一次
   "点击进入全屏演示"，整场都不会再遇到。**

## 15. 设计变更（2026-10-11，联调通过后）：签到改为必须登录

原来的"匿名输名字签到"按用户要求换掉了：**签到必须用组织账号登录，姓名取账号里的真名，
同一个账号只能签到一次**。改动落在这些地方：

| 位置 | 变化 |
| --- | --- |
| `POST /api/v1/checkin` | 从公开接口变成"登录即可"（新中间件 `user`），不再接受请求体里的姓名 |
| 姓名来源 | token 里的 `name` claim（空则退回 `sub`，超长截断到 20 字）——前端不再有输入框 |
| 去重 | `participants.sub` 列 + `CREATE UNIQUE INDEX`，一个组织账号一条；重复签到回 `already:true` 与当前名次，不报错 |
| 新接口 | `GET /api/v1/me` → `{sub, name, admin, checkedIn}`，前端不必解 token |
| `/auth/callback` | **不再拦非白名单用户**（他们正是来签到的）；白名单只守管理接口。登录后按 state 里带的目标回跳 |
| 登录回跳 | `state = nonce\|next` 一起签名，`next` 只放行本站相对路径；`tokenRedirect` 保证 token 落在 hash 之前（`/?token=…#/checkin`） |
| 数据结构迁移 | 老库就地补 `participants.sub` 与唯一索引（`TestOpenMigratesParticipantsSub`），不用删库 |

两处刻意保留的出口：**管理台仍可"手动加一个人"**（给没账号/登不上的人兜底，sub 为空，
多个空 sub 互不冲突）；**管理员自己也能用签到页**（他就是个普通账号 + 白名单）。

## 16. RBAC 四角色（2026-10-11 追加）

在"登录即 user"的基础上加了三个角色，能力矩阵见 §3，实现在 `internal/rbac`。

| 角色 | 能干什么 | 谁来给 |
| --- | --- | --- |
| `admin` | 全部，含查看/授予/撤销角色 | `config.yaml` 的 `auth.admin_subs`（根管理员，不可撤销），或另一个 admin 在管理台点 |
| `operator` | 控制抽奖与大屏：开关签到、名单增删改导入导出、发起抽奖、推进/跳过/结束本轮、重置、导出 | admin |
| `minister` | 往"部长介绍"里写自己那一条（主题、树莓娘图、一句话），可删自己的 | admin |
| `user` | 只能签到 | 默认，谁都拿得到 |
| （未登录） | 只能看大屏与部长介绍列表 | —— |

落地的细节：

- **存储**：`roles(sub PK, role, name, updated_at)`，一个人一个角色；`directors(sub UNIQUE, name, theme, shumeiniang, note)`，
  一条介绍对应一个账号，重复提交即修改自己的那条。
- **角色解析**（`Server.role`）：配置里的 `admin_subs` → 永远 `admin`（"最后一个保险"，库里改不动，
  接口会回 409 `config_admin_immutable`）；否则读 `roles` 表；再否则 `user`。撤销角色 = 立刻回到 `user`。
- **守门**：`Server.gate(cap, handler)` 统一做"登录 + 具备能力"，403 回 `{"error":"no_permission","need":..., "role":...}`，
  前端据此显示"需要 operator/admin"。管理台给 operator，角色面板只给 admin。
- **接口**：`GET /api/v1/directors` 要登录（`user` 及以上）；`POST /api/v1/directors`、`DELETE /api/v1/directors/me` 要 `directors` 能力；
  `POST` 支持可选的 `sub`——**admin 可以带 `sub` 代任何人建/改**（`DELETE /api/v1/directors/{sub}` 同理），
  minister 传别人的 `sub` 会被 403；姓名一律从组织账号取（自己用 token 里的真名，代别人时从签到名单取，
  取不到就 400 `unknown_sub`），不接受前端传名字；
  `GET|POST /api/v1/admin/roles`、`DELETE /api/v1/admin/roles/{sub}` 要 `roles` 能力；原来的 `/api/v1/admin/*` 全部改成要 `lottery` 能力。
- **素材名不信任前端**：主题/树莓娘图只接受已知集合里的值，非法值随机换成合法值（`pickOne`）。
- **前端**：`/api/v1/me` 返回 `{sub, name, role, rootAdmin, checkedIn, can:{...}}`，
  签到页据此显示"确认签到 / 已经签过"，管理台据此决定显示不显示角色面板，部长介绍页据此显示不显示编辑框。
- 边界：`minister` 只能碰自己那条（改/删别人的找 admin，admin 全都能改能删）；
  **给既没签到、也没做过介绍的人凭空建一条做不到**——姓名必须来自组织账号；
  也没做"多人共管一个部门介绍"。

## 17. 收尾变更（2026-10-11）：全站强制登录 + 标题 + 预热

1. **全站强制登录**：原先公开的读接口也改成要登录（`user` 及以上，即任何登录账号）：
   `GET /api/v1/screen/state`、`GET /api/v1/screen/round`、`GET /api/v1/directors`。
   前端路由守卫改成**每个页面**没令牌都先去 Keycloak，登录后回到原页面
   （`next` 带的是当前 hash 路由，例如 `/#/lottery`）。
   例外只有三样：静态资源（JS/CSS/媒体，否则页面根本加载不出来）、`/healthz`、签到二维码
   `GET /api/v1/qr`——二维码是一张包含本站地址的 PNG，`<img>` 带不了 `Authorization` 头，
   而它所在的大屏本身要登录才看得到，不泄露任何东西。
   回归测试：`TestEverythingRequiresLogin`。
   代价要知道：**投影机也要登录一次**（token 只在内存，刷新后会走 Keycloak SSO 静默重登；
   SSO 会话不在时才需要重新输密码）。
2. **文案与标题**：首页日期改为 `2026年10月11日`；标签页标题由路由决定，
   `技术部破冰会 - 抽奖 / 管理 / 签到 / 部长介绍 / 首页 / …`（`router.afterEach` 设置），
   骨架里的默认标题是 `技术部破冰会`。
3. **OIDC 预热**：`Server.Warmup` 在启动后台拉一次发现文档，把"重启后第一次登录要等
   十几秒"挪到无人时段；Keycloak 不可达时只记一行警告，**服务照常启动**
   （实测：issuer 解析不到时仍是 `监听 → 预热失败警告 → 正常服务`）。
