# Deploy Directory

这个目录存放生产环境部署相关文件，用来把 AI Novel IDE 直接部署到 Linux 服务器。

## 文件说明

`docker-compose.prod.yml`

生产环境的 Docker Compose 编排文件，负责启动：

- `postgres`
- `redis`
- `be`
- `fe`

其中：

- `be` 是 Go 后端服务
- `fe` 实际上是一个 `nginx` 容器，用来托管前端静态文件，并把 `/api` 反向代理到后端
- PostgreSQL 和 Redis 只映射到服务器 `127.0.0.1`，方便本机或 SSH 隧道排查，不直接暴露公网
- PostgreSQL 和 Redis 数据使用外部命名卷，由 `docker-deploy.sh` 预先创建
- 后端容器配置了 `stop_grace_period: 120s`，停止服务时会给 AI 流式任务留出收口时间

`docker-deploy.sh`

一键部署脚本。主要作用：

- 创建运行时目录
- 初始化 `deploy/.env`
- 自动生成 PostgreSQL / Redis / JWT 随机密钥
- 创建 PostgreSQL / Redis 外部命名卷
- 生成 Redis ACL 文件
- 生成后端生产配置文件
- 执行 `docker compose up -d --build`

`nginx/default.conf`

前端容器内使用的 `nginx` 配置文件，作用：

- 提供前端静态页面
- 把 `/api` 转发到 `be:8080`
- 兼容单页应用路由刷新
- 支持 SSE 长连接

这个文件通过 volume 挂载到容器中，所以修改后不需要重新 build 前端镜像。

`.env.example`

生产环境变量模板。首次部署时，脚本会基于它生成 `deploy/.env`。

其中数据库密码、Redis 密码、JWT 密钥会自动生成，但以下配置通常需要手工填写：

- `TURNSTILE_*`
- `S3_*`
- `SMTP_*`
- `RESEND_*`
- `PPROF_ENABLED` 和 `PPROF_ADDR`（需要排查 Go 性能时再开启）

## 运行时生成文件

以下内容不是手写维护的，而是部署脚本自动生成：

`deploy/.env`

部署实际使用的环境变量文件。

`deploy/runtime/config.prod.yaml`

后端容器读取的生产配置文件。

`deploy/runtime/redis/users.acl.conf`

Redis ACL 配置文件。

Docker 外部命名卷

用于持久化数据库和 Redis 数据，由脚本自动创建：

- `ai_novel_postgres_data`
- `ai_novel_redis_data`

`deploy/data/logs/`

后端日志目录。

## 部署流程

在服务器上执行：

```bash
cd deploy
chmod +x docker-deploy.sh
./docker-deploy.sh
```

首次执行后，建议检查：

```bash
vi .env
```

确认外部依赖配置已经补齐，然后重新执行：

```bash
docker compose --env-file .env -f docker-compose.prod.yml up -d --build
```

## 常用命令

查看状态：

```bash
docker compose --env-file .env -f docker-compose.prod.yml ps
```

查看日志：

```bash
docker compose --env-file .env -f docker-compose.prod.yml logs -f
```

进入 PostgreSQL：

```bash
docker compose --env-file .env -f docker-compose.prod.yml exec postgres psql -U ai_novel -d ai_novel_ide
```

宿主机本地连接 PostgreSQL：

```bash
psql -h 127.0.0.1 -p 5432 -U ai_novel -d ai_novel_ide
```

重启更新：

```bash
docker compose --env-file .env -f docker-compose.prod.yml up -d --build
```

停止服务：

```bash
docker compose --env-file .env -f docker-compose.prod.yml down
```

注意：PostgreSQL 和 Redis 使用外部命名卷。不要手动执行 `docker volume rm ai_novel_postgres_data ai_novel_redis_data`，否则会删除生产数据。

停止或重启后端时，Go 服务会先停止接收新请求，然后取消并等待正在运行的 AI 流式任务，尽量写完 `t_model_runs` 的结束状态。

修改 `nginx/default.conf` 后刷新配置：

```bash
docker compose --env-file .env -f docker-compose.prod.yml exec fe nginx -s reload
```

如果 reload 不方便，也可以直接重启前端容器：

```bash
docker compose --env-file .env -f docker-compose.prod.yml restart fe
```

## 查看 Go pprof

生产环境默认关闭 pprof。需要排查后端性能时，在 `.env` 中设置：

```bash
PPROF_ENABLED=true
PPROF_ADDR=0.0.0.0:6060
```

然后重新执行部署脚本，使生产配置文件更新：

```bash
./docker-deploy.sh
```

pprof 端口只绑定到服务器的 `127.0.0.1:6060`，不会通过 Nginx 或公网暴露。开发电脑通过 SSH 隧道访问：

```bash
ssh -N -L 6060:127.0.0.1:6060 root@服务器IP
```

随后打开：

```text
http://127.0.0.1:6060/debug/pprof/
```

排查完成后将 `PPROF_ENABLED` 改回 `false`，再执行一次部署脚本。
