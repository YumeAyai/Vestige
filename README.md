# 见迹

见迹是一个隐私优先的邮件触达与匿名埋点项目。

它不把联系人、邮箱、模板、发送记录这些敏感数据放到云上，而是把系统拆成两部分：

- **本地邮件系统**：用户自己运行，负责联系人、模板、发件邮箱、发送调度、统计导入与后续分析。
- **匿名埋点云**：开放、轻量、无状态，负责接收邮件打开、链接点击、二维码加载等匿名事件。

简单说，用户的数据留在用户本地；云端只留下“某个匿名 token 在某个时间触发了某类事件”的痕迹。这也是“见迹”这个名字的含义：只见行为痕迹，不见用户身份。

## 架构

```text
用户本地环境
  联系人 / 模板 / SMTP / 发送 / AB 测试 / 本地统计
        |
        | 邮件中植入匿名埋点 URL
        v
匿名埋点云
  /p           打开像素
  /r           点击重定向
  /qrcode.png  二维码图片加载
  /api/stats   聚合统计
  /api/events  匿名事件拉取
        |
        | 本地系统按需拉取匿名事件
        v
用户本地环境
  用 token 和本地收件人/任务数据做归因分析
```

## 目录

```text
local-backend           本地后端服务：API、发送、统计导入、前端 dist 嵌入
tracking-server         匿名埋点服务：open/click/qrcode 事件收集与聚合
frontend                Vue 3 前端应用
pkg                     两个 Go 服务共享的 db/model/tracker 基础包
docs/tracking.md        埋点服务接口与集成契约
```

## 本地开发

启动本地邮件系统：

```bash
go run ./local-backend/cmd/server
```

启动匿名埋点云：

```bash
go run ./tracking-server/cmd/server
```

启动前端开发服务：

```bash
cd frontend
npm install
npm run dev
```

默认地址：

- 本地邮件系统：`http://localhost:8080`
- 匿名埋点云：`http://localhost:8081`
- Vite 前端：`http://localhost:5173`

开发模式下，本地邮件系统默认把埋点 URL 指向 `http://localhost:8081`。如果埋点云部署到公网，启动本地邮件系统前设置：

```bash
TRACKING_BASE_URL=https://track.example.com go run ./local-backend/cmd/server
```

## 生产构建

```bash
cd frontend
npm run build
cd ..
go build -o jianji-local ./local-backend/cmd/server
go build -o jianji-tracking ./tracking-server/cmd/server
```

运行本地邮件系统：

```bash
./jianji-local
```

运行匿名埋点云：

```bash
TRACKING_ADDR=:8081 TRACKING_DB_PATH=data/tracking.db ./jianji-tracking
```

## 埋点 URL

打开像素：

```html
<img src="https://track.example.com/p?c=123&rid=random-token" width="1" height="1" alt="" />
```

点击重定向：

```html
<a href="https://track.example.com/r?c=123&l=hero&rid=random-token&dest=https%3A%2F%2Fexample.com">
  查看详情
</a>
```

二维码图片：

```html
<img src="https://track.example.com/qrcode.png?token=random-token&target=https%3A%2F%2Fexample.com%2Fsurvey" />
```

详见 [docs/tracking.md](docs/tracking.md)。

## 设计原则

- **私有数据本地化**：联系人、邮箱、发送记录、模板内容不进入埋点云。
- **埋点事件匿名化**：云端只记录 campaign、link、token、事件类型和时间。
- **服务边界清晰**：本地系统做业务决策，埋点云只做收集与聚合。
- **低成本扩展**：埋点云可以迁移到 Cloudflare Workers、Lambda、API Gateway 等 Serverless 环境。
- **可替换**：用户只需替换邮件里的埋点域名，就能切换埋点云。

## 验证

```bash
go test ./...
```
