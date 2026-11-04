# Yearning Linux 部署（二进制 + systemd）

CentOS / Rocky / Ubuntu，x86_64 或 ARM64。主程序 `Yearning` + 独立审核引擎 `engine`（HTTP）+ MySQL 元数据库，前端已 `go:embed` 进主程序。

> Docker 四容器部署见 [`DEPLOY_DOCKER.md`](./DEPLOY_DOCKER.md)；本地开发见 [`DEVELOPMENT.md`](./DEVELOPMENT.md)。

```
浏览器 → :8000 Yearning（systemd: yearning）──HTTP 127.0.0.1:13307──▶ engine（systemd: yearning-engine）
                     └── MySQL :3306（元数据库 utf8mb4）
```

| 组件 | 端口 | 对外 |
|---|---|---|
| Yearning | 8000 | 经 Nginx |
| engine | 13307 | 否，仅本机 |
| MySQL | 3306 | 否，内网/本机 |

---

## 1. 准备

```bash
sudo useradd -r -s /sbin/nologin yearning
sudo mkdir -p /opt/yearning && sudo chown -R yearning:yearning /opt/yearning
sudo timedatectl set-timezone Asia/Shanghai      # 时区错会导致定时执行/工单时间偏移

# 只放行 80/443；必须直连 8000 时再加 8000/tcp
sudo firewall-cmd --permanent --add-service=http --add-service=https && sudo firewall-cmd --reload
```

## 2. MySQL 元数据库

MySQL 8.0（5.7 亦可），字符集 **utf8mb4**。8.0 下若不想改 DSN，请用 `CREATE USER ... IDENTIFIED WITH mysql_native_password BY '...'` 建账号（应用未开 `allowPublicKeyRetrieval`，caching_sha2_password 在非 TLS 下会认证失败）：

```sql
CREATE DATABASE `Yearning_go` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci;
CREATE USER 'yearning'@'%' IDENTIFIED WITH mysql_native_password BY '强口令';
GRANT ALL PRIVILEGES ON `Yearning_go`.* TO 'yearning'@'%';
FLUSH PRIVILEGES;
```

业务库账号另建（查询用只读号、执行用带 DDL/DML 的号），不要复用 root。

## 3. 编译

前端产物必须先生成并拷进 `src/service/dist/`（`go:embed` 只能引用同级子目录）：

```bash
cd front && pnpm install && pnpm build && cd ..
rm -rf src/service/dist/* && cp -r front/dist/* src/service/dist/
```

**Windows 交叉编译后上传**（无需服务器装 Go）：

```powershell
cd c:\hub\Yearning
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -trimpath -ldflags "-s -w" -o dist\Yearning .
cd engine
go build -trimpath -ldflags "-s -w" -o ..\dist\engine ./cmd\engine
```

```powershell
scp dist\Yearning dist\engine deploy@<服务器IP>:/tmp/
```

**Linux 上直接编译**：同上，去掉 `$env:`，改为 `CGO_ENABLED=0 go build ...`。ARM64 用 `GOARCH=arm64`。

```bash
sudo mv /tmp/Yearning /tmp/engine /opt/yearning/
sudo chmod 755 /opt/yearning/Yearning /opt/yearning/engine
sudo chown yearning:yearning /opt/yearning/Yearning /opt/yearning/engine
```

## 4. 配置 `conf.toml`

```bash
sudo cp /path/to/repo/conf.toml.template /opt/yearning/conf.toml
sudo chmod 600 /opt/yearning/conf.toml && sudo chown yearning:yearning /opt/yearning/conf.toml
```

```toml
[Mysql]
Db = "Yearning_go"
Host = "127.0.0.1"
Port = "3306"
User = "yearning"
Password = "强口令"

[General]
SecretKey = "openssl rand -base64 32 的输出"
RpcAddr = "127.0.0.1:13307"   # 必须与 engine -addr 一致
LogLevel = "info"
Lang = "zh_CN"

[Oidc]
Enable = false
```

- `SecretKey` 长度 <16 或为模板值（`CHANGE_ME_PLEASE_USE_RANDOM_32_CHARS`、`dbcjqheupqjsuwsm`）→ 进程直接退出。
- **投产后不要更换**：换了全员掉线，且已保存的数据源口令无法解密。

敏感项走环境变量更安全（优先级高于配置文件）：`SECRET_KEY`、`Y_LANG`、`MYSQL_USER` / `MYSQL_PASSWORD` / `MYSQL_ADDR`（`host:port`）/ `MYSQL_DB`、`RPC_ADDR` / `RPC_TOKEN`（引擎地址与共享令牌）、`IS_DOCKER`（非空时跳过 install 的交互确认）。

```bash
sudo install -m 600 -o yearning -g yearning /dev/null /opt/yearning/yearning.env
# 写入 SECRET_KEY=... 与 Y_LANG=zh_CN
```

## 5. 初始化（一次性）

```bash
sudo -u yearning /opt/yearning/Yearning install -c /opt/yearning/conf.toml
# 会问「是否已将数据库字符集设置为UTF8/UTF8MB4?」，确认后输入 y
# 无人值守：sudo -u yearning IS_DOCKER=is_docker /opt/yearning/Yearning install -c ...
```

初始账号 `admin` / `Yearning_admin`，**登录后立即修改**。已初始化的库不要再执行 install。

## 6. systemd

```bash
sudo tee /etc/systemd/system/yearning-engine.service >/dev/null <<'EOF'
[Unit]
Description=Yearning SQL Audit Engine
After=network.target

[Service]
Type=simple
User=yearning
Group=yearning
WorkingDirectory=/opt/yearning
ExecStart=/opt/yearning/engine -addr 127.0.0.1:13307
Restart=always
RestartSec=3
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

sudo tee /etc/systemd/system/yearning.service >/dev/null <<'EOF'
[Unit]
Description=Yearning
After=network.target yearning-engine.service
Wants=yearning-engine.service

[Service]
Type=simple
User=yearning
Group=yearning
WorkingDirectory=/opt/yearning
EnvironmentFile=-/opt/yearning/yearning.env
ExecStart=/opt/yearning/Yearning run -c /opt/yearning/conf.toml -p 8000
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now yearning-engine yearning
journalctl -u yearning -f          # 期望：Yearning is running on port:  8000
journalctl -u yearning-engine -f   # 期望：engine ... listening on 127.0.0.1:13307
```

冒烟：`curl -I http://127.0.0.1:8000/` 返回 200；`ss -lntp | grep -E '8000|13307'` 确认 13307 只监听 127.0.0.1。应用没有专用健康接口，探活用 `GET /`。

## 7. Nginx + HTTPS

```nginx
upstream yearning_backend { server 127.0.0.1:8000; keepalive 32; }

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
        proxy_pass http://yearning_backend;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header Connection "";
        proxy_buffering off;
        proxy_cache off;
        gzip off;
        proxy_read_timeout 3600s;
    }

    location / {
        proxy_pass http://yearning_backend;
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

启用 OIDC 时把 `[Oidc].RedirectUrL` 改成对外地址，如 `https://sql.example.com/oidc/_token-login`。若想前后端分离（Nginx 托管 `front/dist`、后端只做 API），参考 `docker/nginx/frontend.conf`，并在 `src/service/dist/` 放占位页以通过编译。

## 8. 上线后

改 `admin` 密码 → 添加数据源（区分只读/执行账号）→ 配角色权限与审核规则、审批流 →（可选）配置 AI 模型地址与 Key，需能出网 → 跑一条测试工单验证：检测 → 审批 → 执行 → 查询记录。

## 9. 运维

| 操作 | 命令 |
|---|---|
| 重启 | `sudo systemctl restart yearning yearning-engine` |
| 日志 | `journalctl -u yearning -n 200 --no-pager` |
| 重置 admin | `sudo -u yearning /opt/yearning/Yearning reset_super -c /opt/yearning/conf.toml` |
| 结构修复 | `sudo -u yearning /opt/yearning/Yearning migrate -c /opt/yearning/conf.toml` |
| 备份 | `mysqldump -uyearning -p --single-transaction --default-character-set=utf8mb4 Yearning_go > y_$(date +%F).sql` |

升级：备份库 → `systemctl stop yearning yearning-engine` → 替换两个二进制（保留 `conf.toml` 与 `yearning.env`）→ 跨大版本才 `migrate` → 启服务看日志。**不要换 `SecretKey`**。

安全清单：`conf.toml` / `yearning.env` 600 且不入仓；改默认密码 + RBAC 最小权限；3306 与 13307 不对外；对外只开 443；定期备份并单独保管密钥。

## 10. 故障排查

| 现象 | 处理 |
|---|---|
| 秒退，日志 `SecretKey 强度不足或仍是模板示例值` | 换成 `openssl rand -base64 32` 的结果，或注入 `SECRET_KEY` |
| `dial tcp 127.0.0.1:3306: connection refused` | MySQL 未起 / `bind-address` 只监听本机 / 防火墙 |
| `Access denied for user` | 账号口令或库权限不对 |
| install 后中文乱码 | 库不是 utf8mb4，重建库后重新 install |
| 页面白屏 | 编译时用的是占位 `src/service/dist/index.html`，重新构建前端并重编译 |
| 检测/执行/查询报引擎连接错误 | engine 未起，或 `RpcAddr` 与 `engine -addr` 不一致 |
| Nginx 502 | 主程序没监听 8000；先 `curl -I http://127.0.0.1:8000/` |
| 上传 413 | `client_max_body_size` 太小 |
| AI 助手无回复 | 模型配置/出网问题，或未对 `/api/v2/chat` 关缓冲 |
| 定时执行时间不对 | 服务器时区非 `Asia/Shanghai` |
| 登录后 `/api/v2` 全 401 | `SecretKey` 被更换，旧 token 失效，重新登录 |

## 附录

| CLI | 作用 |
|---|---|
| `Yearning install -c conf.toml` | 初始化建表 + 初始账号（仅首次） |
| `Yearning run -c conf.toml -p 8000` | 启动 |
| `Yearning migrate -c conf.toml` | 破坏性结构修复（先备份） |
| `Yearning reset_super -c conf.toml` | 重置 admin 为 `Yearning_admin` |

| 默认值 | |
|---|---|
| Web 端口 / 引擎端口 | `8000` / `13307`（仅本机） |
| 元数据库 | `Yearning_go` |
| 初始账号 | `admin` / `Yearning_admin` |
