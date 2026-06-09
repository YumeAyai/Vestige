# 邮件阅读埋点设计

当前代码把追踪逻辑拆在 `internal/tracker`，主 Gin 应用只是挂载一个兼容接口：

```text
GET /api/track/open.gif?tid=<tracking_id>
```

邮件发送时，每个收件人都会有独立的 `tracking_id`，并在 HTML 里追加一个 1x1 透明 GIF：

```html
<img src="https://track.example.com/api/track/open.gif?tid=..." width="1" height="1" alt="" style="display:none;width:1px;height:1px;border:0" />
```

## 为什么要拆

业务后台负责：

- 联系人导入
- 模板渲染
- 单封单收件人发送
- 统计和导出

tracker 负责：

- 返回透明图片
- 记录 `tracking_id / ip / user_agent / opened_at`
- 更新收件人的首次阅读、最近阅读、阅读次数

这样以后可以把 tracker 单独部署到 Lambda、Cloudflare Workers、API Gateway 或独立 Go 服务，而不影响后台业务代码。

## 当前本地模式

默认情况下，图片地址使用当前后台地址：

```text
http://localhost:8080/api/track/open.gif?tid=...
```

适合本地测试，但真实邮件里必须使用公网 HTTPS 地址，否则对方邮箱客户端无法加载图片。

## 独立 tracker 模式

生产环境建议设置：

```bash
TRACKING_BASE_URL=https://track.your-company.com
```

这样邮件里的图片地址会变成：

```text
https://track.your-company.com/api/track/open.gif?tid=...
```

后续如果拆到 Lambda，Lambda 只需要实现同样的 GET 接口，并把事件写回数据库、Webhook 或队列。

## 二维码图片触发点

除默认打开像素外，模板里的二维码变量也会进入埋点体系。当前支持：

```text
{{.QRCode}}
```

发送时，本地会为每个收件人创建一条 `tracking_marks`：

```text
campaign_recipient_id
token
kind = qrcode
label = QRCode
target_url
```

模板变量会渲染为：

```html
<img src="https://track.example.com/api/track/qrcode.png?token=..." width="132" height="132" />
```

图片被邮件客户端加载时，tracker 记录一条 `tracking_mark_events`。如果图片服务部署在云端，云端只需要保存触发列表，例如：

```json
{
  "token": "mark-token",
  "kind": "qrcode",
  "triggered_at": "2026-06-09T10:00:00Z",
  "ip": "203.0.113.1",
  "user_agent": "...",
  "referer": "",
  "accept_language": "zh-CN,zh;q=0.9"
}
```

本地通过下面接口导入云端事件，再用 token 和本地 `tracking_marks` / `campaign_recipients` 做比对分析：

```text
POST /api/tracking/cloud-events/import
```

这样云端不需要保存公司名单、邮箱、联系人等敏感信息，只保存不可读的 token 触发记录。

## 现在能收集到的信息

二维码图片请求发生时，本地后端或云端 tracker 可以收集：

- `token`：不可读的二维码埋点 token，用来回连本地 `tracking_marks`
- `kind`：当前为 `qrcode`
- `triggered_at`：二维码图片被请求的时间
- `ip`：请求来源 IP，可能是邮件客户端代理或安全网关 IP
- `user_agent`：请求 UA，可能是邮件客户端、图片代理或安全扫描器
- `referer`：通常为空，但保留字段
- `accept_language`：请求语言，可辅助判断环境
- `is_prefetch`：基于 UA 的弱判断，当前识别 Google image proxy 和 Apple Mail 类请求
- `raw_payload`：云端或本地保留的原始事件 JSON，方便后续补充解析

不建议把这个事件直接命名为“真实查看”。产品侧更准确的口径是：

```text
二维码加载时间 / 正文图片加载时间 / 内容加载推测
```

## 统计服务器和后台交互

推荐生产部署采用“两层”模式：

```text
邮件客户端
  -> 公网 tracker: GET /api/track/qrcode.png?token=...
  -> tracker 立即返回二维码 PNG，并异步记录事件
  -> 后台定时或手动调用 POST /api/tracking/cloud-events/import 导入事件
  -> 后台用 token 关联 campaign_recipient，生成任务统计和收件人明细
```

交互契约：

1. 后台发送邮件前，为每个收件人的二维码生成 `tracking_marks.token`。
2. 邮件正文里的 `{{.QRCode}}` 渲染为 tracker 的二维码图片 URL。
3. tracker 不保存邮箱、联系人、公司名，只保存 token 和请求环境。
4. tracker 需要尽快返回图片，事件写入可以同步写本地表、队列或日志。
5. 后台导入事件后，通过 `token -> tracking_marks -> campaign_recipients` 做归因。
6. 后台 API `GET /api/campaigns/:id/stats` 返回普通像素趋势和二维码加载趋势。
7. 后台 API `GET /api/campaigns/:id/recipients` 返回每个收件人的二维码加载次数、首次加载、最近加载和最近 IP/UA。

## 注意

阅读追踪只能说明“图片被请求过”，不等于真人一定阅读。Apple Mail、Gmail 图片代理、安全网关都可能预加载或代理请求。
