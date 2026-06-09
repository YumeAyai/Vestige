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
| `MONGODB_URI` | 腾讯云文档数据库 MongoDB 连接串 |
| `MONGODB_DATABASE` | 数据库名，默认 `nousmail_tracking` |
| `MONGODB_EVENTS_COLLECTION` | 事件集合名，默认 `tracking_events` |
| `MONGODB_ASSETS_COLLECTION` | 图片资产集合名，默认 `tracking_assets` |

这些值也可以写入 `config.yaml` 的 `scf.mongodb`，SCF 环境变量优先级更高。

## 部署流程

1. 进入 GitHub Actions。
2. 选择 `Tracking SCF CD`。
3. 点击 `Run workflow`。
4. 先不勾选 `deploy`，确认能成功产出 `tracking-scf-package` artifact。
5. 确认 SCF 使用 Go 事件函数运行时，Handler 为 `main`，环境变量已配置后，再勾选 `deploy`。

## 注意

- 当前 CD 云函数入口位于 `tracking-server/cmd/scf`，用腾讯云 SCF Go event handler 接 API 网关事件。
- 文档数据库连接串不要写进代码，放在 SCF 环境变量中。
- 生产部署建议给部署用 CAM 子账号只授予 SCF 更新函数代码的最小权限。
