# 腾讯云 SCF CI/CD

当前仓库有两条 GitHub Actions 流水线：

- `CI`：push 到 `main` 或 PR 时自动运行，执行前端构建和 Go 单元测试。
- `Tracking SCF CD`：手动触发，打包追踪云函数；勾选 `deploy` 后部署到腾讯云 SCF。

## GitHub Secrets

在仓库 `Settings -> Secrets and variables -> Actions` 中配置：

| Secret | 说明 |
| :--- | :--- |
| `TENCENTCLOUD_SECRET_ID` | 腾讯云 API SecretId |
| `TENCENTCLOUD_SECRET_KEY` | 腾讯云 API SecretKey |
| `TENCENTCLOUD_REGION` | SCF 地域，例如 `ap-guangzhou` |
| `SCF_FUNCTION_NAME` | 追踪云函数名称 |
| `SCF_NAMESPACE` | SCF 命名空间，默认可填 `default` |

## SCF 环境变量

在腾讯云 SCF 函数配置中设置：

| 环境变量 | 说明 |
| :--- | :--- |
| `TCB_ENV_ID` | 云开发环境 ID，例如 `xray-7g6vc4y2d2fc01be` |
| `TCB_REGION` | 腾讯云地域，例如 `ap-shanghai` |
| `TCB_EVENTS_COLLECTION` | 事件集合名，默认 `tracking_events` |
| `TCB_ASSETS_COLLECTION` | 图片资产集合名，默认 `tracking_assets` |
| `TENCENTCLOUD_SECRET_ID` | 调用 TCB OpenAPI 的 SecretId |
| `TENCENTCLOUD_SECRET_KEY` | 调用 TCB OpenAPI 的 SecretKey |

SCF 运行时使用腾讯云 Go SDK 的 `RunCommands` 调用 CloudBase 文档型数据库。`TCB_ENV_ID`、地域和集合名可以写入 `config.yaml` 的 `scf.tcb`，SCF 环境变量优先级更高。

## 云函数部署包

构建 Linux amd64 云函数可执行文件和部署包：

```sh
sh scripts/build-tracker-function.sh
```

构建产物：

- `dist/main`：云函数 Linux amd64 可执行文件。
- `dist/tracker.zip`：部署包，包含 `main` 和 `scf_bootstrap`。

函数名建议使用 `jianji`，HTTP 路径绑定 `/jianji`，运行时使用 Go 自定义启动或兼容 `scf_bootstrap` 的运行方式。`TCB_ENV_ID`、地域和集合名可以写入 `config.yaml` 的 `scf.tcb` 或函数环境变量。`TENCENTCLOUD_SECRET_ID` / `TENCENTCLOUD_SECRET_KEY` 属于敏感值，不写入仓库，请在腾讯云函数控制台配置，或给函数绑定具备 TCB 文档型数据库 `RunCommands` 权限的运行角色。

HTTP 函数启动时会先启动 Gin 监听 `PORT`，TCB 数据库连接会在首次写入/查询事件时延迟初始化。因此 `/health` 可以用于确认函数进程和 HTTP 入口是否已经正常启动；如果 `/health` 正常但 `/p` 或 `/api/events` 报错，优先检查 `TENCENTCLOUD_SECRET_ID` / `TENCENTCLOUD_SECRET_KEY` 和 TCB OpenAPI 权限。

## 部署流程

1. 进入 GitHub Actions。
2. 选择 `Release`。
3. 点击 `Run workflow`。
4. 在 `dev` 分支运行时，确认能成功产出 `tracker-*` artifact。
5. 确认 SCF 使用 Go 事件函数运行时，上传 `tracker-*` artifact 中的 `tracker.zip`，Handler 为 `main`，环境变量已配置。

## 注意

- 当前云函数入口位于 `tracker/cmd/scf`，用腾讯云 SCF Go event handler 接 API 网关事件；部署时需要把 HTTP 路径绑定到 `/jianji`，否则默认 Event 函数没有公网 HTTP 访问路径。
- 文档型数据库访问走腾讯云 TCB OpenAPI 的 `RunCommands`，配置入口是 `scf.tcb` 和对应的 `TCB_*` 环境变量。
- 生产部署建议给部署用 CAM 子账号只授予 SCF 更新函数代码的最小权限。
