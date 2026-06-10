# 匿名埋点云设计

本项目采用分离架构：

- 用户本地系统：运行 `backend/cmd/server`，保存联系人、邮箱、模板、发送记录、AB 测试分组等敏感数据。
- 埋点云服务：运行 `tracker/cmd/scf`，只接收匿名 open/click/image 事件并做聚合。

云端不保存联系人、邮箱、公司名、模板正文等 PII。邮件里只植入本地生成的随机 `rid` / `token`，云端只能看到某个匿名 token 在某个时间触发了某类事件。

## 本地系统

启动：

```bash
TRACKING_BASE_URL=https://track.example.com go run ./backend/cmd/server
```

也可以在 `config.yaml` 中设置：

```yaml
local_backend:
  tracking_base_url: "https://track.example.com"
```

本地系统负责：

- 导入和保存联系人
- 管理模板与发件邮箱
- 生成 campaign recipient 的随机 `tracking_id`
- 把透明像素植入邮件正文
- 为 `{{.QRCode}}` 生成随机 token
- 从埋点云拉取匿名事件后导入本地数据库
- 用本地数据计算打开率、二维码加载率、AB 测试结果

## 埋点云

本地调试启动：

```bash
go run ./tracker/cmd/scf
```

对应的 YAML 配置：

```yaml
scf:
  tcb:
    env_id: "xray-7g6vc4y2d2fc01be"
    region: "ap-shanghai"
    events_collection: "tracking_events"
    assets_collection: "tracking_assets"
```

埋点云负责：

- `GET /p`：记录打开事件，返回 1x1 GIF。
- `GET /r`：记录点击事件，然后 302 到 `dest`。
- `GET /img`：记录图片加载事件，可返回上传资产、二维码 PNG 或 1x1 GIF。
- `GET /api/stats`：返回匿名聚合统计。
- `GET /api/events`：返回匿名事件列表，供本地系统拉取导入。

## 腾讯云 SCF 版本

SCF 入口位于：

```text
tracker/cmd/scf
```

它使用腾讯云 SCF Go event handler 接 API 网关事件，接口路径保持一致：

- `GET /health`
- `GET /p`
- `GET /r`
- `GET /img`
- `GET /api/stats`
- `GET /api/events`
- `POST /api/assets`

SCF 版不依赖本地磁盘，数据通过腾讯云 TCB OpenAPI `RunCommands` 写入 CloudBase 文档型数据库：

- `tracking_events`：匿名 open/click/image 事件，包含按当前时间生成的 `id`，本地同步仍使用 `after_id` 游标。
- `tracking_assets`：上传后的图片埋点资产，邮件客户端访问 `/img?asset=...` 时从文档数据库读取。

SCF 环境变量：

| 环境变量 | 含义 |
| :--- | :--- |
| `TCB_ENV_ID` | 云开发环境 ID |
| `TCB_REGION` | 腾讯云地域，默认 `ap-shanghai` |
| `TCB_EVENTS_COLLECTION` | 事件集合名，默认 `tracking_events` |
| `TCB_ASSETS_COLLECTION` | 图片资产集合名，默认 `tracking_assets` |
| `TENCENTCLOUD_SECRET_ID` | 调用 TCB OpenAPI 的 SecretId |
| `TENCENTCLOUD_SECRET_KEY` | 调用 TCB OpenAPI 的 SecretKey |

## URL 契约

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

参数含义：

| 参数 | 含义 |
| :--- | :--- |
| `c` / `campaign` | 用户本地生成的 campaign id |
| `l` / `link` | 用户本地生成的 link id |
| `rid` / `token` | 用户本地生成的随机匿名 token |
| `s` / `source` | 可选，用户或租户前缀 |
| `i` / `idx` / `event_index` | 可选，具体触发点索引，例如 `variant:12:open`、`variant:12:image:qr.png`、`variant:12:link:hero` |
| `dest` | 点击事件的最终跳转地址，仅允许 http/https |
| `target` | 二维码 PNG 中编码的目标地址，仅允许 http/https |

`event_index` 用于区分同一收件人在同一邮件中的不同触发点。AB 测试发送时会自动带上变体信息；没有 AB 变体时使用 `campaign:<id>:...`。

## 匿名事件查询

聚合统计：

```text
GET /api/stats?source=user123&campaign=456&event=open&since=2026-06-01T00:00:00Z
```

事件拉取：

```text
GET /api/events?source=user123&campaign=456&after_id=1000&limit=500
```

返回事件示例：

```json
{
  "events": [
    {
      "id": 1001,
      "source": "user123",
      "campaign": "456",
      "link": "hero",
      "token": "random-token",
      "kind": "click",
      "triggered_at": "2026-06-09 10:30:00"
    }
  ]
}
```

本地系统可以把事件 POST 到：

```text
POST /api/tracking/cloud-events/import
```

请求体：

```json
{
  "events": [
    {
      "token": "random-token",
      "kind": "open",
      "triggered_at": "2026-06-09T10:30:00Z"
    }
  ]
}
```

导入后，本地系统用 `token -> campaign_recipients.tracking_id` 或 `token -> tracking_marks.token` 做本地归因。这个映射只存在用户本地。

## 注意

- 打开事件只表示图片被请求，不等于真人阅读。
- 邮箱客户端、安全网关和图片代理可能产生预加载。
- 云端默认保存 IP/UA 作为事件环境字段；如果生产环境希望更严格匿名，可以在 `tracker/internal/handler` 中去掉这些字段或改成短期哈希。
- 写入端点是公开的，生产环境应在网关层加速率限制、预算告警和域名防滥用策略。
