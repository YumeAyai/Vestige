# 桌面封装研究：Edge WebView / Wails 路线

本文记录把见迹从“浏览器打开本地服务”迁移到 Win/mac 桌面应用的初步判断。

## 结论

推荐路线：用 **Wails** 做桌面壳。

原因：

- 项目后端已经是 Go 单体应用，Wails 的主进程也是 Go，迁移成本比 Tauri/Electron 小。
- Windows 端复用 Microsoft Edge WebView2，能拿到 Chromium/WebView2 的性能、更新和系统集成。
- macOS 端不会使用 Edge WebView2，而是使用系统 WKWebView。这是当前跨平台 WebView 桌面壳的常见现实边界。
- 不需要用户登录、多租户、云端账号体系。身份边界就是本机应用实例、本地数据库和用户自己的 SMTP 配置。
- 可以继续保留现有 Vue 前端，大部分 UI 代码不用重写。

因此，“Edge toolkit”更准确地说应该落成：

```text
Windows: Go app + Wails + WebView2
macOS:   Go app + Wails + WKWebView
Cloud:   匿名埋点云继续独立部署
```

## 为什么不是直接 WebView2

Microsoft WebView2 是很适合 Windows 的嵌入式 Chromium WebView，但官方支持重点在 Windows、Xbox、HoloLens 这条线。macOS 上不能假设存在同一个 WebView2 Runtime。

如果直接写 WebView2 原生壳，会遇到几个问题：

- Windows 需要 Win32/.NET/WinUI 接入层，本项目会被迫增加一套非 Go 的桌面主工程。
- macOS 仍然要单独做 WKWebView 壳，等于维护两套桌面桥接。
- 前端调用本地能力、窗口菜单、文件选择、打包签名都要自己补。

Wails 的价值是把这些差异收起来：Windows 用 WebView2，macOS 用 WKWebView，同时把 Go 方法暴露给前端。

## 备选方案对比

| 方案 | 适配度 | 优点 | 问题 |
| --- | --- | --- | --- |
| Wails | 高 | Go 原生、Vue 友好、Windows WebView2、macOS WKWebView、包体小 | 需要把当前 HTTP API 逐步改成 Go-JS 调用，或先保留本地 HTTP |
| Tauri | 中 | 安全模型成熟、包体小、跨平台 WebView | 后端主语言是 Rust，本项目 Go 后端要做 sidecar 或重写桥接 |
| Electron | 中低 | 生态最大、Chromium 一致 | 包体大，和“本地私有化轻应用”的气质不完全一致 |
| 直接 WebView2 + WKWebView | 低 | 最大控制权 | 要维护 Windows/macOS 两套壳，不适合当前阶段 |

## 建议迁移路径

### 第一阶段：最小桌面壳

目标：先证明桌面启动体验，不动业务核心。

- 新增 Wails 入口，例如 `desktop/main.go`。
- 复用现有 Vue 前端构建产物。
- 启动时打开本地 SQLite 数据库，路径改到用户应用数据目录。
- API 层先有两种选择：
  - 保守方案：桌面主进程仍启动 Gin，但只监听 `127.0.0.1` 随机端口，WebView 加载本地地址。
  - 理想方案：把 Gin handler 后面的业务函数抽出来，前端通过 Wails bridge 直接调用 Go 方法。
- 窗口默认打开 `/`，也就是现在的欢迎/起始页；工作区入口进入 `/dashboard`。

建议第一阶段采用保守方案，因为它对现有代码改动最少，也便于继续跑现有 Go API 测试。

### 第二阶段：本地化体验

目标：让它像桌面应用，而不是浏览器页面。

- 关闭对外监听，只允许本机访问，或完全改为 bridge 调用。
- 数据库路径默认放在系统应用数据目录：
  - Windows: `%APPDATA%/Jianji/app.db`
  - macOS: `~/Library/Application Support/Jianji/app.db`
- 配置文件也迁入应用数据目录，首次启动从内置默认配置生成。
- 加入原生菜单：
  - 导入联系人
  - 导出任务 CSV
  - 打开数据目录
  - 诊断 SMTP/埋点云连接
- 加入单实例锁，避免两个窗口同时写同一个 SQLite。

### 第三阶段：分发

目标：交给真实用户安装。

- Windows：
  - 使用 WebView2 Evergreen Runtime。
  - 安装器检查 Runtime；Windows 11 通常预装，Windows 10 需要兜底安装。
  - 后续补代码签名，减少 SmartScreen 拦截。
- macOS：
  - 打包 `.app` / `.dmg`。
  - 补签名与 notarization。
  - 使用 WKWebView，验证 CSS/文件上传/下载行为和 Chromium 下的差异。

## 当前项目需要调整的点

### 1. 后端入口拆分

现在入口是 `backend/cmd/server/main.go`，职责包括：

- 读取配置
- 打开 SQLite
- 执行迁移
- 创建 Gin server
- 监听 `cfg.Client.Addr`

桌面模式建议拆成可复用函数：

```text
backend/internal/runtime
  OpenApp(config) -> db + migrated store + app handler

backend/cmd/server
  继续用于浏览器/开发/服务器模式

desktop
  用 Wails 启动窗口，并复用 runtime
```

### 2. API 调用层预留 base URL

当前前端 `frontend/src/services/api.js` 用相对路径请求 `/api/...`。这对浏览器和 Gin 嵌入很友好。

如果 Wails 第一阶段仍加载本地 HTTP 地址，可以保持不动。

如果改成 Wails bridge，则建议新增一层服务适配：

```text
frontend/src/services/api.js
  HTTP 实现

frontend/src/services/desktopApi.js
  Wails bridge 实现

frontend/src/services/client.js
  根据运行环境选择实现
```

### 3. 配置和数据库路径

桌面应用不应该默认把 `data/app.db` 放在启动目录。需要在桌面入口里处理默认路径，避免用户把应用拖来拖去时数据丢失或权限异常。

### 4. 安全边界

即使不做登录，也要避免“本地服务被同网段访问”：

- `client.addr` 在桌面模式下必须绑定 `127.0.0.1`，不要绑定 `:8080`。
- 如果保留 HTTP API，建议使用随机端口和一次性 session token。
- WebView 禁止打开任意外部 URL；点击外部链接交给系统浏览器。
- 文件导入导出走原生对话框，不让页面随意读写本机路径。

## 建议的技术 Spike

1. 安装 Wails CLI，运行 `wails doctor`。
2. 新建最小 Wails 入口，不改现有浏览器模式。
3. 让 Wails `AssetServer.Handler` 直接复用现有 Gin handler。
4. 使用系统应用数据目录保存桌面模式 SQLite。
5. 打包 macOS 本机二进制，验证首页、联系人导入、模板预览、发送测试。
6. 在 Windows 机器或 CI 上验证 WebView2 Runtime/安装器策略。
7. 再决定是否把 HTTP API 迁到 Wails bridge。

## Spike 进展

已新增最小桌面入口：

```text
backend/cmd/desktop/main.go
backend/cmd/desktop/wails.json
scripts/build-desktop-spike.sh
```

当前实现没有启动 localhost 端口，而是把现有 Gin engine 直接作为 Wails AssetServer 的 handler。好处是：

- 现有 `/api/...`、静态前端 fallback、上传下载接口都继续走同一套路由。
- 不暴露本机端口，桌面安全边界比“随机 localhost 端口”更干净。
- 现有浏览器模式 `backend/cmd/server` 不受影响。

桌面模式默认数据库路径：

```text
os.UserConfigDir()/Jianji/app.db
```

如果设置了 `CLIENT_DB_PATH` 或 `VESTIGE_CONFIG`，则尊重外部配置。

本机 macOS arm64 已验证：

```bash
GOCACHE="$PWD/.cache/go-build" go build ./backend/cmd/desktop
```

生产 tag 下，macOS 15 SDK 链接 Wails 时需要额外补：

```bash
CGO_LDFLAGS="-framework UniformTypeIdentifiers"
```

直接 `go build -tags desktop,wv2runtime.download,production` 已通过。Wails CLI 的 `wails build` 目前仍没有稳定透传这条链接参数，后续完整 `.app/.dmg` 打包要继续处理这个包装层问题。

## DMG 打包

当前已新增独立 DMG 脚本，绕过 Wails CLI 的 macOS 链接参数问题：

```bash
sh scripts/build-desktop-dmg.sh
```

脚本做的事情：

1. 构建 Vue 前端，把产物写入 `backend/internal/webui/dist`。
2. 创建 `dist/desktop-dmg/stage/见迹.app`。
3. 使用 Wails 桌面入口编译 `Contents/MacOS/Jianji`。
4. 对 `.app` 做本机 ad-hoc codesign。
5. 使用 `hdiutil create` 生成压缩 DMG。

本机已验证输出：

```text
dist/desktop-dmg/Jianji-darwin-arm64.dmg
```

校验命令：

```bash
hdiutil verify dist/desktop-dmg/Jianji-darwin-arm64.dmg
```

注意：当前 DMG 还不是正式分发包，只适合内部测试。公开分发前还需要：

- Developer ID 签名。
- notarization 公证。
- 更正式的 `.icns` 图标和 DMG 背景。
- 首次启动的数据目录、升级策略和单实例写库验证。

## 参考资料

- Microsoft Edge WebView2: https://learn.microsoft.com/en-us/microsoft-edge/webview2/
- WebView2 Runtime distribution: https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/distribution
- Wails introduction: https://wails.io/docs/introduction/
- Wails installation/platform requirements: https://wails.io/docs/gettingstarted/installation/
- Wails project config: https://wails.io/docs/reference/project-config/
- Tauri overview: https://v2.tauri.app/start/
- Tauri WebView versions: https://v2.tauri.app/reference/webview-versions/
