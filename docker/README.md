# Yearning Docker 部署

本页说明用 **Docker / Docker Compose** 部署 Yearning 的步骤，并给出**国内网络加速**（镜像拉取、依赖源）的完整配置。

> 更完整的使用/初始化说明见 [Yearning 官方文档](https://next.yearning.io)（解决 95% 疑问）。
> 本仓库自源码构建、二次开发的加速说明见 [`docs/DEVELOPMENT.md`](../docs/DEVELOPMENT.md)。

- 镜像：`yeelabs/yearning`（各 tag 见 <https://hub.docker.com/r/yeelabs/yearning/tags>）
- **首次部署前必须先执行初始化**：`install`（建表并创建初始账号），否则服务无法正常使用。

---

## 0. 环境变量与安全提醒（务必先读）

启动前可用以下环境变量覆盖配置（`SECRET_KEY` 的优先级高于 `conf.toml`）：

| 变量 | 说明 | 默认/示例 |
|---|---|---|
| `IS_DOCKER` | 标识容器化部署 | `is_docker` |
| `SECRET_KEY` | **JWT 签名与数据源口令加密的主密钥** | 见下方"必改" |
| `MYSQL_USER` / `MYSQL_PASSWORD` / `MYSQL_ADDR` / `MYSQL_DB` | 元数据库连接 | `root` / … / `10.0.0.3:3306` / `Yearning` |
| `Y_LANG` | 界面语言 | `zh_CN` 或 `en_US` |

> **`SECRET_KEY` 必须替换为随机字符串（建议 ≥ 32 字符）**。它是 JWT 签名与数据源口令加密的主密钥，泄露等同于交出全部数据库凭据；代码已强制拒绝模板占位值启动。
> 生成：`openssl rand -base64 32`，或 `python -c "import secrets;print(secrets.token_hex(32))"`。
> 仓库示例里的 `dbcjqheupqjsuwsm` 仅是**演示占位**，生产部署请务必换成随机值，并妥善保管、勿提交/勿写入公共配置。

---

## 1. 国内 Docker 镜像拉取加速

国内访问 Docker Hub 往往超时/极慢。可按需选一种（A、B、C），可叠加使用。

### 方式 A：配置镜像加速器（最推荐，全局生效）

编辑 Docker 守护进程配置 `/etc/docker/daemon.json`（Windows Docker Desktop 在 `Settings → Docker Engine`），加入 `registry-mirrors`，然后重启 Docker：

```json
{
  "registry-mirrors": [
    "https://docker.1ms.run",
    "https://docker.m.daocloud.io",
    "https://dockerproxy.net",
    "https://docker.nju.edu.cn",
    "https://mirror.ccs.tencentyun.com"
  ]
}
```

```bash
sudo systemctl daemon-reload && sudo systemctl restart docker
docker info        # 看到 Registry Mirrors 列表即生效
```

> 加速器地址可能变动，请以各家最新公告为准（阿里云个人加速器在容器镜像服务控制台领取专属地址，形如 `https://<你的ID>.mirror.aliyuncs.com`）。

### 方式 B：为镜像加“加速前缀”（无需改 Docker 配置）

将拉取/引用的镜像名加上公共加速前缀，例如把 `yeelabs/yearning:latest`、`mysql:5.7` 换成：

```bash
# DaoCloud 公共镜像加速前缀
docker pull docker.m.daocloud.io/yeelabs/yearning:latest
docker pull docker.m.daocloud.io/library/mysql:5.7
# 拉取后打回原名，便于 compose 直接引用
docker tag docker.m.daocloud.io/yeelabs/yearning:latest yeelabs/yearning:latest
```

### 方式 C：走国内镜像仓库直连（阿里云容器镜像服务）

将发布版/镜像导入你的阿里云容器镜像服务后，用 `registry.cn-<region>.aliyuncs.com/<命名空间>/<镜像>:<tag>` 拉取与引用。

> 若 `docker pull` 仍有问题，可先临时用 `docker search`/`docker run --pull=always` 排查，或换 `方式 B` 的前缀。

---

## 2. 从源码构建镜像（含国内加速）

> 本仓库 `docker/Dockerfile` **基于源代码编译**（前端 `front/` → `go:embed` → Go 主程序，外加独立审核引擎 `engine/`），并在镜像内**一并内置引擎进程**。上下文须为仓库根目录。

构建（默认已启用国内加速，可通过 `--build-arg` 覆盖）：

```bash
# 在仓库根目录执行；NPM/GOPROXY 均默认指向国内镜像
docker build -f docker/Dockerfile -t yearning:dev .

# 明确指定国内加速源（可选，默认值即如下）
docker build \
  --build-arg NPM_REGISTRY=https://registry.npmmirror.com \
  --build-arg GOPROXY_URL=https://goproxy.cn,direct \
  -f docker/Dockerfile -t yearning:dev .
```

镜像内容与行为：
- 前端产物经 `yarn build`（npmmirror）产出，拷贝进 `src/service/dist/` 后由 `go:embed` 打入主程序。
- 引擎 gRPC 进程在容器入口脚本后台启动，监听 `127.0.0.1:13307`；主程序经 `/opt/conf.toml` 的 `[General].RpcAddr=127.0.0.1:13307` 连接它（无需外部再起引擎）。
- `/opt/conf.toml` 为内置兜底配置；**`SecretKey` 为占位值，未通过环境变量注入强随机 `SECRET_KEY` 时进程会拒绝启动**（代码强制）。
- 构建/运行所需国内依赖源（本地二次开发同）：Go `GOPROXY=https://goproxy.cn,direct`、前端 `front/.npmrc`→npmmirror。详见 [`docs/DEVELOPMENT.md`](../docs/DEVELOPMENT.md) 第 4 节。

> 高级用法：也可单独构建/部署审核引擎（`docker build -f engine/Dockerfile engine/`），再把它指向主程序 `RpcAddr`，此时请改 `/opt/conf.toml` 或自行挂载配置。

---

## 3. 首次初始化 + 启动（Docker CLI）

> 以下以官方发布镜像 `yeelabs/yearning` 为例。若使用本仓库源码构建的镜像（第 2 节，如 `yearning:dev`），把镜像名替换为你的标签即可；源码镜像已内置审核引擎，无需外部再起。

```bash
# 1) 初始化数据库（一次性；会建表并创建初始账号）
docker run --rm -it \
  -p 8000:8000 \
  -e IS_DOCKER=is_docker \
  -e SECRET_KEY=请替换为随机串 \
  -e MYSQL_USER=root \
  -e MYSQL_ADDR=10.0.0.3:3306 \
  -e MYSQL_PASSWORD=你的库密码 \
  -e MYSQL_DB=Yearning \
  -e Y_LANG=zh_CN \
  yeelabs/yearning "/opt/Yearning install"

# 2) 后台启动服务
docker run -d -it \
  -p 8000:8000 \
  -e IS_DOCKER=is_docker \
  -e SECRET_KEY=请替换为随机串 \
  -e MYSQL_USER=root \
  -e MYSQL_ADDR=10.0.0.3:3306 \
  -e MYSQL_PASSWORD=你的库密码 \
  -e MYSQL_DB=Yearning \
  -e Y_LANG=zh_CN \
  yeelabs/yearning
```

访问 `http://<服务器IP>:8000`，默认账号 `admin` / 初始密码 `Yearning_admin`（**登录后请立即修改**）。

> 方式 A/B/C 配置镜像加速后，请把上面的 `yeelabs/yearning` 换成相应可拉取的名称。

---

## 4. Docker Compose 部署

本仓库 `docker/docker-compose.yml` 提供一个 `yearning + mysql:5.7` 的组合，可直接修改使用。

### 4.1 常见运维命令

```bash
cd docker

# 首次使用：先初始化（执行后请注释/去掉 init 命令，避免重启重复初始化）
docker compose run --rm yearning /opt/Yearning install

# 后台启动
docker compose up -d

# 升级后执行结构迁移
docker compose run --rm yearning /opt/Yearning migrate

# 重置 admin 密码
docker compose run --rm yearning /opt/Yearning reset_super

# 查看日志 / 停止
docker compose logs -f
docker compose down
```

> - 若拉取镜像慢，先按第 1 节配好加速，或把 compose 中镜像名加上 `方式 B` 的前缀。
> - 数据库 `data/` 目录请做好备份；`docker compose down` 不会删除数据卷。

### 4.2 安全加固清单

- [ ] 将 `SECRET_KEY` 换为随机串（严禁使用示例值）。
- [ ] 修改默认 `admin` 密码。
- [ ] 为 `MYSQL_*` 更换强口令，避免复用示例值。
- [ ] 生产对外建议置于反向代理（Nginx/网关）之后并启用 HTTPS。
- [ ] 控制 `8000` 端口只对可信来源开放。

---

## 5. docker tag

- 官方 tag 列表：<https://hub.docker.com/r/yeelabs/yearning/tags>

---

## 附：默认信息

| 项 | 值 |
|---|---|
| 服务端口 | `8000` |
| 初始账号 | `admin` |
| 初始密码 | `Yearning_admin`（登录后请立即修改） |
