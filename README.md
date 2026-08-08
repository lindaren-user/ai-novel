# AI Novel IDE

[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8.svg)](https://golang.org/)
[![Vue](https://img.shields.io/badge/Vue-3.5+-4FC08D.svg)](https://vuejs.org/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-18+-336791.svg)](https://www.postgresql.org/)
[![Redis](https://img.shields.io/badge/Redis-8+-DC382D.svg)](https://redis.io/)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED.svg)](https://www.docker.com/)

AI Novel IDE 是一个面向长篇小说创作的 AI 协作工作台。它把小说设定、卷规划、章节规划、正文生成、润色校验和上下文管理放在同一个工作流里，让作者负责方向与审美，AI 负责结构化规划、内容协作和重复性整理。

## 概览

这个项目不是普通聊天工具，而是围绕小说生产流程设计的创作 IDE：

- 小说级对话：整理世界观、人物、设定和全书结构。
- 卷级对话：规划卷主题、章节分布和阶段性冲突。
- 章节级对话：生成正文、维护草稿、进行润色和校验。
- 阅读/编辑模式：在正文阅读、草稿编辑和 AI 辅助之间切换。
- 运行记录：独立记录每次 AI 调用的模型、token、状态、开始/结束时间和错误信息。

## 功能特性

- **三级创作上下文**：小说、卷、章节拥有独立会话和消息记录，避免上下文混乱。
- **新建小说设定生成**：根据用户输入和上传资料生成小说初始化设定。
- **结构化规划卡片**：AI 输出可渲染为规划选项、卷规划、章节规划和正文草稿。
- **章节正文草稿**：支持 AI 原始草稿、可编辑草稿、当前正文和草稿切换。
- **正文润色与校验**：围绕章节正文进行润色、人味化和问题校验。
- **流式生成与恢复**：支持 SSE 流式回复、刷新恢复和主动取消。
- **模型配置管理**：支持内置模型和用户自定义兼容模型配置。
- **用户认证**：支持注册、登录、邮箱验证码、HttpOnly Cookie 和刷新会话。
- **分享与下载**：支持小说、卷、章节分享和导出下载。
- **限流与并发控制**：支持认证接口请求限流和 AI 流式任务并发限制。

## 技术栈

| 组件 | 技术 |
|------|------|
| 后端 | Go, Eino, chi |
| 前端 | Vue 3, Vite, Pinia, Tailwind CSS |
| 数据库 | PostgreSQL |
| 缓存 | Redis |
| 部署 | Docker, Docker Compose, Nginx |

## 部署

### 方式一：Docker Compose 生产部署

适合部署到 Linux 服务器。生产部署文件位于 `deploy/` 目录。

#### 前置要求

- Docker
- Docker Compose v2
- Linux 服务器

#### 快速开始

```bash
git clone <你的仓库地址>
cd ai-novel-ide/deploy
chmod +x docker-deploy.sh
./docker-deploy.sh
```

脚本会自动完成：

- 创建 `deploy/.env`
- 自动生成 PostgreSQL、Redis、JWT 随机密钥
- 创建外部命名卷 `ai_novel_postgres_data` 和 `ai_novel_redis_data`
- 生成 Redis ACL 文件
- 生成后端生产配置 `runtime/config.prod.yaml`
- 构建并启动全部容器

首次部署后，建议编辑 `.env` 补齐外部服务配置：

```bash
vi .env
```

通常需要填写：

- `TURNSTILE_*`
- `S3_*`
- `SMTP_*` 或 `RESEND_*`

修改后重新启动：

```bash
./docker-deploy.sh
```

访问：

```text
http://YOUR_SERVER_IP
```

#### 常用命令

```bash
# 查看容器状态
docker compose --env-file .env -f docker-compose.prod.yml ps

# 查看日志
docker compose --env-file .env -f docker-compose.prod.yml logs -f

# 更新并重建
docker compose --env-file .env -f docker-compose.prod.yml up -d --build

# 停止服务
docker compose --env-file .env -f docker-compose.prod.yml down
```

#### 数据安全说明

生产环境 PostgreSQL 和 Redis 使用外部命名卷：

- `ai_novel_postgres_data`
- `ai_novel_redis_data`

它们由 `docker-deploy.sh` 预先创建，并在 `docker-compose.prod.yml` 中以 `external: true` 引用。执行 `docker compose down -v` 不会删除这两个外部卷。

不要手动执行：

```bash
docker volume rm ai_novel_postgres_data ai_novel_redis_data
```

### 方式二：本地开发

本地开发只用 Docker 启动 PostgreSQL 和 Redis，后端和前端在宿主机运行，方便调试和热更新。

#### 前置要求

- Go 1.24+
- Node.js 22+
- pnpm
- Docker
- Docker Compose v2

#### 启动基础设施

```bash
docker compose up -d postgres redis
```

#### 启动后端

```bash
cd be
go run ./cmd/server
```

后端默认读取：

```text
be/config.yaml
```

#### 启动前端

```bash
cd fe
pnpm install
pnpm dev
```

访问：

```text
http://localhost:5173
```

## 数据库初始化与迁移

初始化 SQL 位于：

```text
be/migrations/init.sql
```

PostgreSQL 官方镜像只会在数据库数据目录为空时执行 `/docker-entrypoint-initdb.d` 下的 SQL。也就是说：

- 新库第一次启动会执行 `init.sql`
- 修改 `init.sql` 后重启容器不会自动执行
- 已有生产库改表结构时，需要单独执行迁移 SQL 或接入迁移工具

生产环境不要为了应用表结构变更而删除数据库 volume。

## Nginx 配置

生产前端容器实际基于 `nginx` 镜像构建，负责：

- 托管前端静态文件
- 将 `/api` 反向代理到 Go 后端
- 支持 Vue 单页应用刷新
- 支持 SSE 流式响应

配置文件：

```text
deploy/nginx/default.conf
```

该文件通过 volume 挂载进容器。修改后可以直接 reload：

```bash
docker compose --env-file .env -f docker-compose.prod.yml exec fe nginx -s reload
```

或者重启前端容器：

```bash
docker compose --env-file .env -f docker-compose.prod.yml restart fe
```

## 项目结构

```text
ai-novel-ide/
├── be/                         # Go 后端
│   ├── cmd/server/             # 服务入口
│   ├── internal/
│   │   ├── ai/                 # AI 客户端、模型适配和 Agent 编排
│   │   ├── config/             # 配置加载
│   │   ├── handler/            # HTTP Handler
│   │   ├── middleware/         # HTTP 中间件
│   │   ├── model/              # 领域模型和响应结构
│   │   ├── repo/               # 数据访问层
│   │   ├── router/             # 路由装配
│   │   └── service/            # 业务服务
│   ├── migrations/             # 数据库初始化 SQL
│   └── Dockerfile              # 后端镜像构建
├── fe/                         # Vue 前端
│   ├── src/
│   │   ├── components/         # 页面组件和业务组件
│   │   ├── pages/              # 页面
│   │   ├── services/           # API 请求
│   │   ├── stores/             # Pinia 状态
│   │   └── types/              # TypeScript 类型
│   └── Dockerfile              # 前端 Nginx 镜像构建
├── deploy/                     # 生产部署文件
│   ├── docker-compose.prod.yml
│   ├── docker-deploy.sh
│   ├── nginx/default.conf
│   └── README.md
├── docker-compose.yml          # 本地开发基础设施
└── README.md
```

## 开发命令

```bash
# 后端测试
cd be
go test ./...

# 前端构建
cd fe
pnpm build

# 本地基础设施
docker compose up -d postgres redis

# 停止本地基础设施
docker compose down
```

## 注意事项

- `be/config.yaml` 是本地私有配置，不应提交到仓库。
- `deploy/.env` 是生产私有配置，不应提交到仓库。
- 生产环境建议在外层增加 HTTPS 入口，例如 Caddy、宿主机 Nginx 或云厂商负载均衡。
- 模型 API Key、S3 Key、SMTP 密码等敏感信息只应放在本地配置或服务器 `.env` 中。
- 当前项目的 `init.sql` 用于新库初始化，不等价于生产迁移系统。
