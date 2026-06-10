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

SCF 运行时使用腾讯云 Go SDK 的 `RunCommands` 调用 CloudBase 文档型数据库，不需要 `MONGODB_URI`。这些值也可以写入 `config.yaml` 的 `scf.tcb`，SCF 环境变量优先级更高。

## CloudBase CLI 部署

项目根目录已经提供 `cloudbaserc.json`，函数名是 `jianji`，运行时是 `Go1`，Handler 是 `main`，上传目录是 `functions/jianji`。

首次部署前先构建 Linux 可执行文件：

```sh
sh scripts/build-tcb-function.sh
```

然后部署 HTTP 云函数并绑定访问路径：

```sh
tcb fn deploy -e xray-7g6vc4y2d2fc01be jianji --httpFn --path /jianji --runtime Go1 --force
```

`cloudbaserc.json` 会写入 `TCB_ENV_ID`、地域和集合名。`TENCENTCLOUD_SECRET_ID` / `TENCENTCLOUD_SECRET_KEY` 属于敏感值，不写入仓库，请在腾讯云函数控制台配置，或给函数绑定具备 TCB 文档型数据库 `RunCommands` 权限的运行角色。

## 部署流程

1. 进入 GitHub Actions。
2. 选择 `Tracking SCF CD`。
3. 点击 `Run workflow`。
4. 先不勾选 `deploy`，确认能成功产出 `tracking-scf-package` artifact。
5. 确认 SCF 使用 Go 事件函数运行时，Handler 为 `main`，环境变量已配置后，再勾选 `deploy`。

## 注意

- 当前 CD 云函数入口位于 `tracking-server/cmd/scf`，用腾讯云 SCF Go event handler 接 API 网关事件；通过 CloudBase CLI 部署时要使用 HTTP 函数参数 `--httpFn --path /jianji`，否则默认 Event 函数没有公网 HTTP 访问路径。
- 文档型数据库访问走腾讯云 TCB OpenAPI 的 `RunCommands`，不要再配置 `MONGODB_URI`。
- 生产部署建议给部署用 CAM 子账号只授予 SCF 更新函数代码的最小权限。
