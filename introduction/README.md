# bitnp-icebreaker

## Project setup
```
npm install
```

### Compiles and hot-reloads for development
```
npm run serve
```

### Compiles and minifies for production
```
npm run build
```

### Lints and fixes files
```
npm run lint
```

### Customize configuration
See [Configuration Reference](https://cli.vuejs.org/config/).

## 后端（icebreaker）

现场抽奖的服务端：Keycloak 只用来保护管理台，签到与大屏都是公开的。

### 构建与运行

``` sh
npm install && npm run build        # 产出 dist/，会被 go:embed 进二进制
go build -o icebreaker ./cmd/icebreaker

cp config.example.yaml config.yaml  # 填 hmac_secret、Keycloak、admin_subs
./icebreaker -config config.yaml
```

没构建前端也能 `go build`（仓库里只有 `dist/.gitkeep`），首页会返回一句
"前端还没构建"的提示页。

### 页面

| 路由 | 谁能看 | 干什么 |
| --- | --- | --- |
| `/#/lottery` | 登录即可 | 大屏：待机显示"已签到 M 人 + 签到二维码"；开轮后播放中奖动画，推进由管理台控制 |
| `/#/checkin` | 登录即可 | 签到：确认姓名（取账号真名）即可，一个账号只算一次 |
| `/#/admin` | operator / admin | 开关签到、名单管理、发起抽奖、上一位/下一位/跳过动画/结束本轮、重置、导出；admin 还多一块角色面板 |
| `/#/directors` | 登录即可（minister/admin 可编辑自己那条） | 部长介绍；部长登录后右下角有"我的部长介绍"表单 |
| `/#/lottery-random`、`/#/lottery-ordered` | 登录即可 | 原有页面，未改动 |

**全站强制登录**：任何页面都要先过 Keycloak（登录后回到原页面）。只有静态资源、
`/healthz` 与签到二维码是公开的。所以投影机也要登录一次；token 只在内存里，
刷新页面会走 SSO 静默重登。

### 角色

登录后默认是 `user`（只能签到）。四个角色：`user` 签到、`minister` 写部长介绍、
`operator` 控抽奖与大屏、`admin` 全部权限含管角色。`config.yaml` 里 `auth.admin_subs`
填的是**根管理员**（永远 admin、库里撤不掉）；其余角色由 admin 在管理台点选。

### 管理台

最省事是跑向导：它会带着你在 Keycloak 里建好 client、抓 secret、写好 `config.yaml`，
并把你登录后的 sub 填进白名单（每一步都会说清点什么、复制什么）。

``` sh
./scripts/setup-keycloak.sh
```

手做的话：

1. 在组织 Keycloak 的 master realm 建 confidential client `icebreaker`：
   Client authentication = On、只开 Standard flow（**不要** requirement PKCE，本服务不带
   PKCE），Valid redirect URI 填 `<现场地址>/auth/callback`。
2. `config.yaml` 里填 issuer / client_id / client_secret / redirect_url。
3. `auth.admin_subs` 填管理员的 `sub`（登录后看服务端日志即可拿到）；留空 = 谁都不能管理。

开发期在 `npm run serve`（3000 端口）下，`/api` 与 `/auth` 会被代理到本机 8080 的 Go 服务。

### 测试

``` sh
go test ./...
```

设计与取舍见 [docs/design.md](docs/design.md)。
部署与发布流程见 [docs/deploy.md](docs/deploy.md)。
