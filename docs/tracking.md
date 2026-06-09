# 匿名埋点云设计

本项目采用分离架构：

- 用户本地系统：运行 `local-backend/cmd/server`，保存联系人、邮箱、模板、发送记录、AB 测试分组等敏感数据。
- 埋点云服务：运行 `tracking-server/cmd/server`，只接收匿名 open/click/qrcode 事件并做聚合。

云端不保存联系人、邮箱、公司名、模板正文等 PII。邮件里只植入本地生成的随机 `rid` / `token`，云端只能看到某个匿名 token 在某个时间触发了某类事件。

## 本地系统

启动：

```bash
TRACKING_BASE_URL=https://track.example.com go run ./local-backend/cmd/server
```

如果不设置 `TRACKING_BASE_URL`，开发环境默认使用：

```text
http://localhost:8081
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

启动：

```bash
TRACKING_ADDR=:8081 TRACKING_DB_PATH=data/tracking.db go run ./tracking-server/cmd/server
```

埋点云负责：

- `GET /p`：记录打开事件，返回 1x1 GIF。
- `GET /r`：记录点击事件，然后 302 到 `dest`。
- `GET /qrcode.png`：记录二维码图片加载事件，返回二维码 PNG。
- `GET /api/stats`：返回匿名聚合统计。
- `GET /api/events`：返回匿名事件列表，供本地系统拉取导入。

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

二维码图片：

```html
<img src="https://track.example.com/qrcode.png?token=random-token&target=https%3A%2F%2Fexample.com%2Fsurvey" />
```

参数含义：

| 参数 | 含义 |
| :--- | :--- |
| `c` / `campaign` | 用户本地生成的 campaign id |
| `l` / `link` | 用户本地生成的 link id |
| `rid` / `token` | 用户本地生成的随机匿名 token |
| `s` / `source` | 可选，用户或租户前缀 |
| `dest` | 点击事件的最终跳转地址，仅允许 http/https |
| `target` | 二维码 PNG 中编码的目标地址，仅允许 http/https |

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
- 云端默认保存 IP/UA 作为事件环境字段；如果生产环境希望更严格匿名，可以在 `tracking-server/internal/trackingcloud` 中去掉这些字段或改成短期哈希。
- 写入端点是公开的，生产环境应在网关层加速率限制、预算告警和域名防滥用策略。
