# Nous Mail

面向行政人员的批量邮件发送、阅读追踪和 CSV 导出平台。

## 技术栈

- 后端：Go + Gin
- 数据库：SQLite
- 前端：Vue 3 + Vite + ECharts
- 部署：Go 单体服务嵌入前端构建产物

## 目录

```text
cmd/server          Gin 服务入口
internal/app        API、追踪、导出、静态资源服务
internal/db         SQLite 连接与迁移
internal/mailer     SMTP 发送与邮件变量渲染
internal/models     核心模型
internal/webui      前端 dist 嵌入目录
frontend            标准 Vite Vue 应用
data                本地 SQLite 数据
```

## 开发启动

```bash
go run ./cmd/server
cd frontend
npm install
npm run dev
```

Vite 默认运行在 `http://localhost:5173`，API 代理到 `http://localhost:8080`。

## 生产构建

```bash
cd frontend
npm run build
cd ..
go build -o nous-mail ./cmd/server
./nous-mail
```

访问 `http://localhost:8080`。
