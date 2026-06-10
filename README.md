# 见迹

见迹是一个隐私优先的邮件触达与匿名埋点项目。

它不把联系人、邮箱、模板、发送记录这些敏感数据放到云上，而是把系统拆成两部分：

- **本地邮件系统**：用户自己运行，负责联系人、模板、发件邮箱、发送调度、统计导入与后续分析。
- **匿名埋点云**：开放、轻量、无状态，负责接收邮件打开、链接点击、图片加载等匿名事件。

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
  /img         图片埋点
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
backend                 本地后端服务：API、发送、统计导入、前端 dist 嵌入
tracker                 匿名埋点服务：open/click/image 事件收集与聚合
frontend                Vue 3 前端应用
pkg                     两个 Go 服务共享的 db/model/tracker 基础包
docs/tracking.md        埋点服务接口与集成契约
```

## 本地开发

项目配置集中在 `config.yaml`，也可以用 `VESTIGE_CONFIG=/path/to/config.yaml` 指定其他配置文件。环境变量会覆盖 YAML 配置。

常用配置：

```yaml
local_backend:
  addr: ":8080"
  db_path: "data/app.db"
  tracking_base_url: "https://xray-7g6vc4y2d2fc01be-1309857796.ap-shanghai.app.tcloudbase.com/jianji"

frontend:
  dev_port: 5173
  api_proxy: "http://localhost:8080"
```

启动本地邮件系统：

```bash
go run ./backend/cmd/server
```

本地调试匿名埋点云：

```bash
go run ./tracker/cmd/scf
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

## 构建交付

```bash
sh scripts/build-app.sh
sh scripts/build-tracker-function.sh
```

- `dist/app/vestige-linux-amd64`
- `dist/app/vestige-linux-arm64`
- `dist/app/vestige-windows-amd64.exe`
- `dist/app/vestige-darwin-amd64`
- `dist/app/vestige-darwin-arm64`
- `dist/main`：云函数 Linux amd64 可执行文件
- `dist/tracker.zip`：云函数部署包

开发模式下，本地邮件系统会读取 `config.yaml` 的 `local_backend.tracking_base_url`。如果临时覆盖埋点云地址，也可以启动前设置：

```bash
TRACKING_BASE_URL=https://track.example.com go run ./backend/cmd/server
```

## 生产构建

```bash
sh scripts/build-app.sh
sh scripts/build-tracker-function.sh
```

运行本地邮件系统：

```bash
./dist/app/vestige-darwin-arm64
```

部署匿名埋点云：

```bash
dist/tracker.zip
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

图片埋点：

```html
<img src="https://track.example.com/img?type=qr&token=random-token&target=https%3A%2F%2Fexample.com%2Fsurvey" />
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
