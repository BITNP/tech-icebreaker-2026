# bitnp-icebreaker 网络开拓者协会破冰会工具箱
------------------------------------------------

## 1. 项目内容
1. 抽卡动画 (./introduction)
2. 共享画布 (./shared-canvas)

------------------------------------------------
## 2. 安装
### 2.1 抽卡动画安装
``` sh
cd ./introduction
npm install
```

### 2.2 共享画布安装
``` sh
cd ./shared-canvas
pip install flask flask-socketio flask-cors eventlet
```

------------------------------------------------

## 3. 运行
### 3.1 运行抽卡动画
``` sh
cd ./introduction
npm install && npm run build          # 前端产物 dist/，会被 go:embed 进二进制
go build -o icebreaker ./cmd/icebreaker
cp config.example.yaml config.yaml    # 填 hmac_secret、Keycloak、admin_subs
./icebreaker -config config.yaml
```
然后在浏览器打开 `localhost:8080`。签到名单与抽奖结果存在 SQLite 里，后端重启不丢；
Keycloak 只用来保护管理台，签到与大屏都是公开的。

提供以下页面：
- `localhost:8080/#/lottery`: 大屏。待机时显示"已签到 M 人 + 签到二维码"，开轮后播放中奖动画；
- `localhost:8080/#/checkin`: 现场签到页（姓名取组织账号真名，一个账号只算一次）；
- `localhost:8080/#/admin`: 管理台（需 Keycloak 登录且角色为 operator/admin；admin 另有角色管理面板）；
- `localhost:8080/#/directors`: 部长介绍页；
- `localhost:8080/#/lottery-random`、`/#/lottery-ordered`: 原来的抽奖页与伪抽奖页（保留未改）。

**全站强制登录**：所有页面都要先用组织账号过 Keycloak（登录后回到原页面）；只有静态资源、
`/healthz` 和签到二维码是公开的。投影机也要登录一次，刷新会走 SSO 静默重登。

现场流程：大家扫码、用组织账号登录后签到（姓名取账号真名，一个账号只算一次）→
管理台"发起抽奖"（填抽取人数/标签/是否让中奖者出局）→
管理台按"下一位"逐位揭晓，大屏 1.5 秒轮询跟上 → 必要时"跳过动画"、"结束本轮"。
大屏只保留 `F` 全屏（翻页统一由管理台控制）；一次点击"进入全屏演示"同时解锁自动播放。

只做前端调试时（后端已在 8080 跑着）：
``` sh
npm run serve   # http://localhost:3000，/api 与 /auth 自动代理到 8080
```

**旧的两个抽奖页**（`/#/lottery-random`、`/#/lottery-ordered`）保持原样，
仍用下箭头键/上箭头键来播放动画 (类似PPT)：
_如果觉得动画视频或音频太长，可以再按一次下箭头键，来跳过动画_

**快捷键列表**
| 快捷键 | 功能 |
| --- | --- |
| ⬇️ | 播放动画（类似 PPT），再次按下可跳过动画 |
| ⬆️ | 播放动画（类似 PPT） |
| 空格 | 跳过动画视频或音频 （BanG Dream 抽卡音乐有点长，建议名字出来之后使用空格键跳过） |
| F | 进入/退出全屏 |
| U | 上传抽奖名单文件 (.json 或 .txt) |

_**关于抽奖名单文件格式**_
（管理台"名单 → 导入文件"用同一份格式；`/#/lottery` 大屏自己不再读文件。）
接受`.json`或`.txt`两种格式。
- `.json`文件格式示例：
``` json
[
  {"name": "林子煊", "avatarUrl": "/avatar/azusa.jpeg", "theme": "GenshinImpact", "shumeiniangImage": "guitar2"},
  {"name": "张家诚", "avatarUrl": "/avatar/mugi.jpeg", "theme": "Arknights", "shumeiniangImage": "keyboard"},
  {"name": "袁靖", "avatarUrl": "/avatar/mio.jpeg", "theme": "WutheringWaves", "shumeiniangImage": "bass"},
  {"name": "杨浩天", "avatarUrl": "/avatar/ritsu.jpeg", "theme": "PJSK", "shumeiniangImage": "drum"},
  {"name": "郭逸戈", "avatarUrl": "/avatar/yui.jpeg", "theme": "BanGDream", "shumeiniangImage": "guitar1"}
]
```
其中， `avatarUrl`、`theme` 以及 `shumeiniangImage` 字段可以省略。

- `.txt`文件格式示例：(仅描述name列表，用换行符`\n`分隔)
```
林子煊
张家诚
袁靖
杨浩天
郭逸戈
```


### 3.2 运行共享画布
#### 3.2.1 后端
``` sh
cd ./shared-canvas
python backend.py
```
#### 3.2.2 前端
``` sh
cd ./shared-canvas/frontend
python -m http.server 8000
```

_注意：共享画布需要公网 IP 才能使用户正常访问，最好部署在服务器上，或使用允许 http 服务费的内网穿透工具_


------------------------------------------------
作者：`25网协技术部`

如有问题，请通过邮箱`indexguo@163.com`随时联系

------------------------------------------------
## 4. 部署

抽卡动画那部分（`./introduction`）已经是"单二进制 + SQLite"的服务：Keycloak 登录、
全站强制登录、RBAC 四角色、签到与抽奖都在一起。镜像由 GitHub Actions 构建并推送到
`ghcr.io/bitnp/icebreaker`。

部署步骤、开演前检查与回滚方式见 [`./introduction/docs/deploy.md`](./introduction/docs/deploy.md)，
设计与取舍见 [`./introduction/docs/design.md`](./introduction/docs/design.md)。
