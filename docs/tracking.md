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

## 注意

阅读追踪只能说明“图片被请求过”，不等于真人一定阅读。Apple Mail、Gmail 图片代理、安全网关都可能预加载或代理请求。
