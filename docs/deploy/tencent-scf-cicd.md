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

## 部署流程

1. 进入 GitHub Actions。
2. 选择 `Tracking SCF CD`。
3. 点击 `Run workflow`。
4. 先不勾选 `deploy`，确认能成功产出 `tracking-scf-package` artifact。
5. SCF 入口、运行时和环境变量确认后，再勾选 `deploy`。

## 注意

- 当前 CD 预期云函数入口位于 `tracking-server/cmd/scf`，下一步接入 SCF Go event 时会补上。
- 文档数据库连接串不要写进代码，后续放在 SCF 环境变量中，例如 `MONGODB_URI`、`MONGODB_DATABASE`、`MONGODB_COLLECTION`。
- 生产部署建议给部署用 CAM 子账号只授予 SCF 更新函数代码的最小权限。
