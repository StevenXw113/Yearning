# 本地开发指南（Yearning Go 后端）

本文说明在 **dev 分支** 上进行本地开发的完整流程。Yearning 仓库只含 Go 后端，前端源码在独立工程中；本仓库通过 `go:embed` 把前端构建产物打进可执行文件。

> 技术栈：Go 1.21+（工具链 1.22 亦可）、GORM、自研 Web 框架 `github.com/cookieY/yee`、MySQL。
> SQL 解析 / 审核 / 执行由独立的「审核引擎」进程提供（见下文「审核引擎」），**不在本仓库内**。

---

## 1. 环境准备

| 依赖 | 版本 | 用途 |
|---|---|---|
| Go | >= 1.21（推荐 1.22.x） | 编译后端 |
| MySQL | 5.7+ / 8.0，字符集 **utf8mb4** | 元数据存储（工单、用户、权限等） |
| 前端产物 | 见第 3 节 | 随二进制内嵌的前端静态文件 |
| 审核引擎 | 独立进程 | SQL 检测 / 执行 / 查询预检（RPC） |

> Windows / PowerShell 与 macOS / Linux / bash 命令在下方分别给出。

### 1.1 Go 版本检查

```powershell
go version   # 期望 >= go1.21
```

如未安装，请从官方安装包安装（请勿使用已损坏的临时目录工具链，见「常见问题」）。

### 1.2 启动 MySQL（无本地库时的最快方式）

```powershell
docker run -d --name yearning-mysql ^
  -e MYSQL_ROOT_PASSWORD=root ^
  -e MYSQL_DATABASE=Yearning_go ^
  -p 3306:3306 ^
  mysql:5.7 --character-set-server=utf8mb4 --collation-server=utf8mb4_general_ci
```

若用系统 MySQL，请自行确认库与字符集。

---

## 2. 配置文件

配置文件不提交仓库（`.gitignore` 已忽略 `conf.toml`），首次需从模板复制：

```powershell
cd c:/hub/Yearning
Copy-Item conf.toml.template conf.toml
```

按模板修改 `conf.toml`：

- `[Mysql]`：本机数据库连接。
- `[General].SecretKey`：**必须替换为随机字符串（建议 ≥ 32 字符）**。代码已强制拒绝使用模板占位值启动，否则进程直接退出。
  - 生成：PowerShell 下
    ```powershell
    openssl rand -base64 32
    ```
  - 或用 Python：`python -c "import secrets;print(secrets.token_hex(32))"`
  - 也可通过环境变量 `SECRET_KEY` 注入（优先级高于配置文件）。
- `[General].RpcAddr`：审核引擎地址，默认 `127.0.0.1:50001`。

> 同一主密钥同时用于 JWT 签名与数据源口令加密，泄露等同交出全部数据库凭据，请妥善保管、勿提交仓库。

---

## 3. 前端 embed 产物（启动前必做）

`src/service/yearning.go` 通过 `go:embed` 内嵌：

- `src/service/dist/` —— 主前端页面
- `src/service/chat/server/app/index.html` —— AI 助手（SSE）页面

这两个目录被 `.gitignore` 忽略，clone 后**不存在**。缺少它们时 `go build ./...` 会报：

```
src/service/yearning.go:28:12: pattern chat/*: no matching files found
```

### 3.1 有前端构建产物

把 `dist` 输出放到 `src/service/dist/`，`chat` 工程输出放到 `src/service/chat/`，保持
`index.html` 的路径正确即可。

### 3.2 仅开发后端（最小占位）

为让编译通过，可放入最小占位文件（见下方命令）。此时页面为空壳，但 HTTP API / 路由 / JWT 等后端逻辑可正常开发与自测。

```powershell
New-Item -ItemType Directory -Force -Path src/service/dist, src/service/chat/server/app | Out-Null
@'
<!DOCTYPE html><html><head><title>Yearning</title></head><body><div id="app"></div></body></html>
'@ | Set-Content -Encoding UTF8 src/service/dist/index.html
@'
<!DOCTYPE html><html><head><title>Yearning Chat</title></head><body><div id="app"></div></body></html>
'@ | Set-Content -Encoding UTF8 src/service/chat/server/app/index.html
```

> Linux/macOS 请对应改用 `mkdir -p` 与 `cat > ... <<'EOF'`。

---

## 4. 安装依赖

依赖已由 `go.sum` 管理（dev 分支已恢复其版本控制）。

```powershell
cd c:/hub/Yearning
go mod tidy
```

若访问 `proxy.golang.org` 超时（本项目此前即遇到），切换到国内代理：

```powershell
go env -w GOPROXY=https://goproxy.cn,direct
go env -w GOSUMDB=sum.golang.google.cn
go mod tidy
```

---

## 5. 编译校验（可选但推荐）

```powershell
go build ./...     # 全量编译
go vet ./...       # 静态检查（可选）
```

> 纯语法级检查可用 `gofmt -e -l ./src ./cmd`。

---

## 6. 初始化数据库

仅首次（或库结构变化）时执行，会在目标库建表并创建初始账号：

```powershell
go run . install -c conf.toml
```

默认账号：`admin` / `Yearning_admin`（登录后请立即修改）。

---

## 7. 启动开发服务

```powershell
go run . run -c conf.toml -p 8000
```

- 访问 `http://127.0.0.1:8000`（有前端产物时打开完整 UI）。
- 常用调试端口：`-p 8000`。
- 实时重载：热重启依赖你在编辑器配置运行 `go run . run`（项目未内置 air/watcher，可自装 `github.com/air-verse/air`）。

启动成功标志（日志）：
```
Yearning is running on port:  8000
```

---

## 8. 审核引擎（可选但功能必需）

`[General].RpcAddr` 指向的「审核引擎」**独立于本仓库**，负责：

| RPC 方法 | 用途 |
|---|---|
| `Engine.Check` | 提交前 SQL 语法 / 规则检查 |
| `Engine.Query` | 查询语句预检、拆条、LIMIT 注入、敏感列识别 |
| `Engine.Exec` | 工单执行 |
| `Engine.MergeAlterTables` | 合并多条 ALTER |
| `Engine.StopDelay` | 停止定时执行 |

缺引擎时：登录、用户、数据源 CRUD、设置、权限等不依赖引擎的接口可正常自测；凡发起 `Engine.*` 的调用（SQL 检测、执行、查询）会报连接错误。若需完整联调，请先在 `RpcAddr` 指定的地址部署引擎进程。

---

## 9. CLI 命令速查

```text
Yearning install        # 初始化数据库并建初始账号
Yearning run            # 启动服务（-p 端口）
Yearning migrate        # 破坏性版本升级修复
Yearning reset_super    # 重置 admin 密码
Yearning --help         # 帮助
```

---

## 10. 常见问题

| 现象 | 原因与解决 |
|---|---|
| `go build` 报 `chat/*: no matching files found` | 前端 embed 产物缺失，见第 3 节补 `dist/` 与 `chat/` |
| 进程秒退，日志含 `SecretKey 强度不足或仍是模板示例值` | `conf.toml` 的 SecretKey 仍是占位值，替换为随机串（第 2 节） |
| `go mod tidy` / `go get` 网络超时 | 切 `GOPROXY=https://goproxy.cn,direct` |
| 提示工具链损坏、`textflag.h:1: expected identifier` | 使用了损坏的 Go（含此前临时目录 `%USERPROFILE%\sdk`）。请用官方安装包重装到默认路径 |
| 页面空白 | 使用了第 3.2 节的占位产物；接上真实前端构建产物即可 |
| 端口被占用 | `run` 命令加 `-p <端口>` |
| `go.sum` 未被 git 跟踪 | dev 分支已恢复跟踪；勿再从 `.gitignore` 删除/忽略它 |
| 修改配置文件后需重启 | 配置在启动时读取，改动后重启进程 |

---

## 11. 提交与分支约定（本次安全修复分支）

```powershell
git checkout dev
git add -A
git commit -m "scope: 简述改动"
git push origin dev
```

- `dev` 分支承载开发/安全修复，`main`/`next` 为稳定线。
- 合入稳定线前请在正常 Go 环境跑一次 `go build ./...` 与 `go test ./...`。
- `SecretKey`、数据库口令等敏感信息**严禁**进入提交。
