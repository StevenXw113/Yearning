# Yearning Docker 部署（速查）

四容器：`frontend`（Nginx 静态 + 反代）、`yearning`（仅 API）、`engine`（审核引擎）、`mysql`。
镜像全部由本仓库源码构建，不用官方镜像。完整说明见 [`../docs/DEPLOY_DOCKER.md`](../docs/DEPLOY_DOCKER.md)。

> ⚠️ `docker-compose.yml` 的 `build.context` 相对 compose 文件解析，请在仓库 `docker/` 目录执行，或改成源码绝对路径。

```bash
cd docker
cp .env.example .env && chmod 600 .env    # 必改 SECRET_KEY（openssl rand -base64 32）与 MYSQL_* 口令
docker compose build                      # frontend + yearning + engine
docker compose run --rm yearning /opt/Yearning install   # 仅一次
docker compose up -d
```

访问 `http://<服务器IP>:8000`，默认 `admin` / `Yearning_admin`（登录后改）。

## 常用命令

```bash
docker compose logs -f frontend yearning engine
docker compose down                                    # 保留 ./data/mysql
docker compose run --rm yearning /opt/Yearning reset_super
docker compose run --rm yearning /opt/Yearning migrate # 跨大版本升级，先备份

# 备份元数据库
docker compose exec -T mysql sh -c 'exec mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" --single-transaction "$MYSQL_DB"' > backup.sql
```

只发前端（后端不重启）：

```bash
docker build -f docker/frontend.Dockerfile -t yearning-frontend:1.0.1 .
sed -i 's/^FRONTEND_IMAGE=.*/FRONTEND_IMAGE=yearning-frontend:1.0.1/' .env
docker compose up -d frontend
```

全量升级同理：三个镜像用新 tag 构建 → 改 `.env` 的 `*_IMAGE` → `docker compose up -d frontend yearning engine`。旧 tag 保留即可回滚。

## 要点

- 入口只有 `frontend` 的 `8000`（容器内 80）；后端 `8000` 与引擎 `13307` 都不映射宿主机。
- 前端容器启动时把 `yearning` 解析成 IP，后端重建换 IP 后要 `docker compose restart frontend`，否则 502。
- 引擎与后端同网络，地址由 `RPC_ADDR` 环境变量给（默认 `yearning-engine:13307`），容器名必须一致。
- 引擎默认不认证；跨不信任网络时设 `RPC_TOKEN`（后端）与 `ENGINE_AUTH_TOKEN`（引擎）为同一随机串。
- 后端镜像已内置 `gh-ost`（OSC 在线表变更，`OscMinRows > 0` 时启用）。
- 前端构建用 pnpm（`front/pnpm-lock.yaml`）；镜像加速走 `/etc/docker/daemon.json` 的 `registry-mirrors`。
- 生产建议前置 Nginx + HTTPS 指向 `127.0.0.1:8000`，并把 `FRONT_BIND_ADDR` 改为 `127.0.0.1`。

## 默认值

| 项 | 值 |
|---|---|
| 前端入口 | `http://<IP>:8000`（Nginx 80） |
| 后端 API | `yearning:8000`（容器内） |
| 引擎 | `yearning-engine:13307`（容器内） |
| 元数据库 | MySQL 8.0，`Yearning_go`，utf8mb4 |
| 初始账号 | `admin` / `Yearning_admin` |
| 后端配置 | `/opt/conf.toml`（可挂载覆盖） |
