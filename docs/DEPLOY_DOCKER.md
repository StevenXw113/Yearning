# Yearning Docker 部署（Linux 服务器）

四容器：`frontend`（Nginx 静态 + 同源反代）、`yearning`（仅 API）、`engine`（HTTP 审核引擎）、`mysql`。
前后端分离、引擎独立；**所有镜像由本仓库源码构建**，不用官方预构建镜像。

> 二进制 / systemd 部署见 [`DEPLOY_LINUX.md`](./DEPLOY_LINUX.md)；速查版见 [`../docker/README.md`](../docker/README.md)。

---

## 0. 架构

```
浏览器 → http://<IP>:8000 ─▶ frontend 容器（Nginx :80）
                                ├─ /            → 首页
                                ├─ /front/*     → Vite 产物（vite base=/front/）
                                └─ 其余         → 反代 yearning:8000（API、/chatbot、/oidc）
                                                     └─ HTTP ▶ yearning-engine:13307
                                                     └─ MySQL ▶ mysql:3306（元数据）
```

| 项 | 说明 |
|---|---|
| 前端 | `yearning-frontend`，容器内 `:80`，宿主机 `0.0.0.0:8000` |
| 后端 | `yearning`，容器内 `:8000`，不映射宿主机 |
| 引擎 | `yearning-engine`，`:13307`，不映射宿主机（无鉴权） |
| 跨域 | 前后端同源（Nginx 反代），无 CORS |
| 镜像 | `docker/frontend.Dockerfile` / `docker/Dockerfile` / `engine/Dockerfile`，均源码构建 |

---

## 1. 服务器准备

```bash
curl -fsSL https://get.docker.com | sudo sh && sudo systemctl enable --now docker
docker version && docker compose version     # 需 Docker 20.10+ / compose v2
mkdir -p /data/yearning && cd /data/yearning
```

国内镜像加速（拉基础镜像与 mysql 用），`/etc/docker/daemon.json`：

```json
{
  "registry-mirrors": ["https://docker.1ms.run", "https://docker.m.daocloud.io", "https://docker.nju.edu.cn"],
  "log-driver": "json-file",
  "log-opts": { "max-size": "50m", "max-file": "5" }
}
```

```bash
sudo systemctl daemon-reload && sudo systemctl restart docker

# 方案一：直连 8000
sudo firewall-cmd --permanent --add-port=8000/tcp && sudo firewall-cmd --reload
# 方案二：前置 HTTPS，只放行 80/443，并把 FRONT_BIND_ADDR 改为 127.0.0.1
```

**不要**对外暴露 `3306`、`13307`。

---

## 2. 环境变量

| 变量 | 必填 | 说明 |
|---|---|---|
| `SECRET_KEY` | ✅ | JWT 签名 + 数据源口令加密。**≥32 字符随机串**，<16 或模板值进程直接退出 |
| `MYSQL_USER` / `MYSQL_PASSWORD` / `MYSQL_DB` | ✅ | 元数据库；`MYSQL_ADDR` 由 compose 写死 `mysql:3306` |
| `RPC_ADDR` | 建议 | 引擎地址，compose 内为 `yearning-engine:13307`（覆盖 conf.toml 的 `RpcAddr`） |
| `RPC_TOKEN` | 可选 | 引擎共享令牌；与引擎侧 `ENGINE_AUTH_TOKEN` 相同才生效，留空 = 不认证 |
| `Y_LANG` / `TZ` | 建议 | `zh_CN` / `Asia/Shanghai` |
| `FRONT_BIND_ADDR` / `FRONT_PORT` | 建议 | 默认 `0.0.0.0` / `8000` |
| `IS_DOCKER` | 自动 | compose 已设，跳过 `install` 的交互式确认 |

```bash
openssl rand -base64 32     # 填 SECRET_KEY
```

> **密钥投产不要更换**：换了之后全员掉线，且已保存的数据源口令无法解密。

---

## 3. 构建镜像

```bash
cd /path/to/Yearning      # 前端/后端上下文都是仓库根目录

docker build -f docker/frontend.Dockerfile -t yearning-frontend:1.0.0 .
docker build -f docker/Dockerfile          -t yearning:1.0.0 .
docker build -f engine/Dockerfile          -t yearning-engine:1.0.0 ./engine   # 上下文是 engine/
```

- 前端：`node:20-alpine` 用 pnpm 构建（`pnpm-lock.yaml`）→ `nginx:1.27-alpine` 托管，`EXPOSE 80`。
- 后端：`golang:1.26` 编译 → `alpine:3.20`；`go:embed` 目录只放占位页，页面由前端镜像提供；镜像内附带 `gh-ost`（OSC 用）。
- 引擎：`golang:1.26` 编译 → distroless，`EXPOSE 13307`，**无 shell，只能看日志**。
- 构建加速：`--build-arg NPM_REGISTRY=https://registry.npmmirror.com`、`--build-arg GOPROXY_URL=https://goproxy.cn,direct`（默认已开启）。
- ARM64：三个镜像都加 `--platform linux/arm64`（用 `docker buildx build`）。
- 离线：`docker save yearning-frontend:1.0.0 yearning:1.0.0 yearning-engine:1.0.0 | gzip > app.tgz`，目标机 `gunzip -c app.tgz | docker load`。

---

## 4. 部署

```bash
cd /path/to/Yearning/docker     # context: .. 必须相对此目录
cp .env.example .env && chmod 600 .env && vi .env
docker compose build
docker compose run --rm yearning /opt/Yearning install    # 仅一次
docker compose up -d
docker compose ps && docker compose logs -f yearning frontend
```

访问 `http://<服务器IP>:8000`，初始账号 `admin` / `Yearning_admin`（**登录后立即改**）。

- 不要把 `install` 写进常驻 `command`，否则每次重启重复初始化。
- 部署目录若不在仓库内，`build.context` 要改成源码绝对路径（或删掉 `build:` 只用已构建镜像）。
- 不用 Compose 时：建 `yearning` 网络，依次起 mysql、engine（`-addr :13307`）、执行 `install`、起后端（不映射端口）、起前端（`-p 8000:80`）。

---

## 5. 自定义配置（可选）

引擎地址走 `RPC_ADDR` 环境变量（compose 已设），不必挂载配置。要改日志级别、OIDC、OSC、归档等再挂载：

```bash
cp /path/to/Yearning/docker/conf.toml ./conf.toml && chmod 644 ./conf.toml
```

放开 `docker-compose.yml` 中 `yearning` 的挂载后重启：

```yaml
        volumes:
            - ./conf.toml:/opt/conf.toml:ro
```

常用项（完整字段见仓库 `conf.toml.template`）：

```toml
[General]
LogLevel = "info"
RpcToken = ""            # 与 ENGINE_AUTH_TOKEN 一致才启用引擎认证
OscMinRows = 0           # >0 时大表 ALTER 走 gh-ost（镜像已内置）
ArchiveDays = 0          # >0 时归档终态工单与查询日志
```

---

## 6. HTTPS（宿主机 Nginx，可选）

前端容器已处理静态与 API，宿主机只做 TLS 终止，整站转发到 `127.0.0.1:8000`。

```nginx
upstream yearning_frontend { server 127.0.0.1:8000; keepalive 32; }

server {
    listen 80;
    server_name sql.example.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name sql.example.com;
    ssl_certificate     /etc/nginx/ssl/sql.example.com.pem;
    ssl_certificate_key /etc/nginx/ssl/sql.example.com.key;
    client_max_body_size 50m;

    # AI 助手 SSE：不能缓冲
    location = /api/v2/chat {
        proxy_pass http://yearning_frontend;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header Connection "";
        proxy_buffering off;
        proxy_cache off;
        gzip off;
        proxy_read_timeout 3600s;
    }

    location / {
        proxy_pass http://yearning_frontend;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 300s;
    }
}
```

```bash
sudo nginx -t && sudo systemctl reload nginx
```

- 前端是 hash 路由，无需 `try_files`；启用 OIDC 时把 `[Oidc].RedirectUrL` 改为对外地址。

---

## 7. 运维

| 操作 | 命令 |
|---|---|
| 重建镜像 | `docker compose build` |
| 只发前端 | 构建新 tag → 改 `FRONTEND_IMAGE` → `docker compose up -d frontend` |
| 重启 | `docker compose restart frontend yearning engine` |
| 日志 | `docker compose logs -f yearning`（或 `frontend` / `engine`） |
| 重置 admin | `docker compose run --rm yearning /opt/Yearning reset_super` |
| 结构修复 | `docker compose run --rm yearning /opt/Yearning migrate`（先备份） |
| 进容器 | `docker compose exec frontend sh` / `... exec yearning sh`（engine 无 shell） |

```bash
# 备份 / 恢复元数据库
docker compose exec -T mysql sh -c 'exec mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" --single-transaction "$MYSQL_DB"' > y_$(date +%F).sql
docker compose exec -T mysql sh -c 'exec mysql -uroot -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DB"' < y_2026-11-03.sql
```

升级：`git pull` → 三个镜像用新 tag 构建 → 改 `.env` 的 `*_IMAGE` → `docker compose up -d frontend yearning engine` → 跨大版本再 `migrate`。旧 tag 保留即可回滚。`.env` 与 `conf.toml` 单独备份（缺 `SECRET_KEY` 则数据源口令报废）。

---

## 8. 引擎独立部署

| 项 | 说明 |
|---|---|
| 地址 | 后端 `RPC_ADDR=yearning-engine:13307`（或 conf.toml 的 `RpcAddr`），容器名必须一致 |
| 协议 | HTTP+JSON；健康检查 `/healthz`，Prometheus 指标用 `-metrics-addr` 另开端口 |
| 认证 | 默认不认证（仅容器内网）；跨网络时后端 `RPC_TOKEN` + 引擎 `ENGINE_AUTH_TOKEN` 用同一随机串 |
| TLS | 引擎 `-tls-cert/-tls-key`（可加 `-tls-client-ca` 做 mTLS），后端配 `RpcTLSCA`/`RpcTLSCert`/`RpcTLSKey` |
| 网络 | 只在内网，勿映射宿主机 |
| 依赖 | 需能直连被审计的业务库；不连元数据库 |
| 升级 | 改 `ENGINE_IMAGE` → `docker compose up -d engine`，不影响前后端 |
| 扩容 | 无状态，可多副本 + 负载均衡，`RPC_ADDR` 指向 LB |

---

## 9. 故障排查

| 现象 | 处理 |
|---|---|
| 日志 `SecretKey 强度不足或仍是模板示例值` | `SECRET_KEY` 未注入 / 太短 / 用了示例值 |
| `dial tcp ...:3306: connection refused` | MySQL 未就绪或地址错；确认 `MYSQL_ADDR=mysql:3306`、同 network |
| `Access denied for user` | 口令与 MySQL 首次初始化时创建的账号不一致（改口令需进库 `ALTER USER`） |
| 中文乱码 / 建表失败 | 元库不是 utf8mb4 |
| 首页只有一句"API 服务运行中" | 访问到了后端占位页：应访问前端容器 `8000` |
| 页面能开但 js/css 全 404 | 静态资源走 `/front/`，检查镜像内 `dist` 与 Nginx alias |
| 前端 502 | 后端未起，或后端换 IP 后 upstream 失效：`docker compose restart frontend` |
| 工单检测/执行报引擎连接错误 | 引擎未起、`RPC_ADDR` 与容器名不符、或不同 network |
| 上传 413 | Nginx `client_max_body_size` 太小（前端容器 + 宿主机两层都要设） |
| AI 助手无输出 | 未对 `/api/v2/chat` 关缓冲，或容器无法出网访问模型 API |
| 时间差 8 小时 | 容器 `TZ`、MySQL `--default-time-zone` 未设 |
| 磁盘被日志占满 | 未配 `log-opts max-size` |
| `install` 报已存在表 | 重复初始化，不要再执行 |

---

## 附录

| 后端容器内命令 | 作用 |
|---|---|
| `/opt/Yearning install` / `run` / `migrate` / `reset_super` | 初始化 / 启动 / 结构修复 / 重置 admin |

| 默认值 | |
|---|---|
| 前端入口 | `http://<IP>:8000` |
| 引擎地址 | `yearning-engine:13307` |
| 元数据库 | `Yearning_go`（utf8mb4） |
| 初始账号 | `admin` / `Yearning_admin` |
