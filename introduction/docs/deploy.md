# 部署与发布

一次现场部署照着做完就行。设计取舍见 [design.md](design.md)，配置项说明见
[config.example.yaml](../config.example.yaml)。

## 0. 前置（只有一次）

组织 Keycloak（master realm）里建好 confidential client `icebreaker`：

- Client authentication = On；只勾 Standard flow；**不要**要求 PKCE（本服务不带 PKCE）。
- Valid redirect URIs 登记要用的地址，一字不差：
  - `https://<现场域名>/auth/callback`
  - 兜底的局域网地址也登记一条，例如 `http://192.168.1.10:8080/auth/callback`
- 记下 client secret，以及管理员的 Keycloak `sub`（登录一次看服务端日志即可拿到）。

## 1. 镜像

CI（`.github/workflows/build.yml`）在推送 main、打 `v*` tag 或手动触发时构建并推送到
`ghcr.io/bitnp/icebreaker`，tag 包含 `latest`、分支名与 `sha-xxxxxxx`（生产建议 pin 到 sha）。

本地构建也可以：

``` sh
cd introduction && podman build -t icebreaker:local .
```

镜像是多阶段的：node 阶段出前端 dist，go 阶段把 dist 嵌进单个静态二进制，运行时镜像里
只有一个可执行文件，外部依赖只有挂进来的 `config.yaml` 和 `data/`。

## 2. 服务器目录

```
/opt/icebreaker/
  docker-compose.yml   # 从仓库 introduction/ 拷
  config.yaml          # 现场配置，绝不入库
  data/                # SQLite，挂出来重建容器不丢
```

``` sh
cd /opt/icebreaker
cp <仓库>/introduction/config.example.yaml config.yaml
$EDITOR config.yaml      # 至少填 hmac_secret / oidc.* / admin_subs
```

- `auth.hmac_secret`：`openssl rand -base64 32`（别用自动生成的，重启会让管理端 token 全失效）
- `auth.oidc.*`：issuer、client_id、client_secret、redirect_url
- `auth.admin_subs`：根管理员的 sub 列表；**留空谁都进不了管理台**（启动日志会警告）

## 3. 起服务

``` sh
podman-compose up -d
curl -s localhost:8080/healthz        # {"status":"ok"}
```

反向代理按 compose 里的 Traefik label 走（域名在 `traefik.http.routers.icebreaker.rule`）。
不跑容器也行：`./icebreaker -config config.yaml`，一样是单文件 + SQLite。

启动日志应该看到两行：`监听 :8080…` 和 `OIDC 发现文档就绪…`。第二行是把"重启后第一次
登录等十几秒"预热掉的；Keycloak 暂时不可达时这里只会警告，服务照常起。

## 4. 开演前 15 分钟

1. 管理台：打开 `https://<域名>/#/admin` 用组织账号登录，确认能看到控制台（不是"没有控制台权限"）。
2. 确认签到开着、名单人数对；`config.yaml` 里的 `admin_subs` 里有自己。
3. **投影机**：打开 `https://<域名>/#/lottery` 并登录一次，点一下"点击进入全屏演示"
   （这一次点击解锁整场的音视频自动播放）。token 只在内存里，刷新会走 SSO 静默重登。
4. 给要用到的人点好角色：`operator`（控场）、`minister`（写部长介绍）。
5. 演练一遍：扫码签到 → 发起抽奖（人数/标签/是否移出）→ 下一位 → 跳过动画 → 上一位 →
   结束本轮 → 重置中奖 → 再发起一轮确认中奖者已出局。

## 5. 回滚

``` sh
podman-compose down
# 把 compose 里的镜像 tag 换回上一个 sha，或使用本地构建的镜像
podman-compose up -d
```

- 数据独立：`data/icebreaker.db` 挂在容器外，换镜像不丢。要备份先停服务再 `cp data/icebreaker.db*`。
- Keycloak/网络出问题时服务照常起，只是没人登得进来；恢复后第一次登录会自动重试。
- 轮次号是单调计数（重置中奖也不回退），所以大屏不会串到上一轮的人。

## 6. 排错速查

| 现象 | 先看什么 |
| --- | --- |
| 登录后没控制台权限 | 账号的 sub 在不在 `auth.admin_subs`；或在管理台角色面板里给没给角色 |
| `/auth/login` 返回 503 | 服务器能不能解析并访问 issuer：`curl <issuer>/.well-known/openid-configuration` |
| 大屏只有名字没有动画 | 投影机点过"进入全屏演示"没有；顶部有没有红条提示点一下解锁 |
| 签到提示"你已经签过了" | 正常：一个组织账号只能签一次 |
| 管理台看不到角色面板 | 那一块只给 `admin`；`operator` 能用控制台但看不到角色 |
| 大屏人数不动 | 大屏页面还登着吗（1.5 秒轮询，`/api/v1/screen/state` 401 会显示"连接中断"） |
