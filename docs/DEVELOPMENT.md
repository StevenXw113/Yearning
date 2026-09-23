# 本地开发指南（Yearning Go 后端）

本文说明在 **dev 分支** 上进行本地开发的完整流程。仓库结构为：

- `front/` —— 前端（Vue3 + Vite）源码，前端构建产物由此目录编译产出（见第 3 节）。
- `src/` —— Go 后端主程序，通过 `go:embed` 把前端构建产物打进可执行文件。
- `engine/` —— SQL 解析 / 审核 / 执行引擎（独立 Go module，见第 8 节），由 `go.mod` 的 `replace engine => ./engine` 引用。

> 技术栈：Go ≥ 1.26、GORM、自研 Web 框架 `github.com/cookieY/yee`、MySQL。
> 前端为 Vue3 + Vite + TypeScript，技术栈详见 `front/README.md`。
> SQL 解析 / 审核 / 执行由独立的「审核引擎」进程提供（源码在本仓库 `engine/`，见下文「审核引擎」）。

---

## 1. 环境准备

| 依赖 | 版本 | 用途 |
|---|---|---|
| Go | >= 1.26（`go.mod` 已声明 `go 1.26.0`） | 编译后端 |
| MySQL | 5.7+ / 8.0，字符集 **utf8mb4** | 元数据存储（工单、用户、权限等） |
| Node.js + yarn | 前端 `front/` 使用 | 前端依赖安装与构建（Vue3 + Vite） |
| 审核引擎 | 见第 8 节（源码在 `engine/`） | SQL 检测 / 执行 / 查询预检（RPC） |

> Windows / PowerShell 与 macOS / Linux / bash 命令在下方分别给出。

### 1.1 Go 版本检查

```powershell
go version   # 期望 >= go1.26
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

前端源码在本仓库 **`front/`**，构建产物由 `front` 编译产出。`src/service/yearning.go` 通过 `go:embed` 把下列目录内嵌进可执行文件：

- `src/service/dist/` —— 主前端页面（`/`、`/front`）
- `src/service/chat/server/app/index.html` —— AI 助手（SSE）页面（`/chatbot`）

> **为什么产物要放到 `src/service/dist/`？** Go 的 `//go:embed` 只能引用声明它的 `.go` 文件所在目录的子目录，**不能引用 `../front/dist`**。因此前端在 `front/` 编译产出后，还需拷贝进 `src/service/dist/` 才能被后端内嵌。产物路径由 `.gitignore` 忽略，不入库。

### 3.1 构建完整前端并接入后端

在仓库根目录下，先在 `front/` 中安装依赖并构建，再把产物拷贝到 embed 目录：

```powershell
cd front
yarn install          # 首次或依赖变更后
yarn build            # 产出到 front/dist
cd ..
Remove-Item src/service/dist/* -Recurse -Force -ErrorAction SilentlyContinue
Copy-Item front/dist/* src/service/dist/ -Recurse -Force
```

> Linux/macOS 将 `Remove-Item`/`Copy-Item` 对应改为 `rm -rf src/service/dist/*` 与 `cp -r front/dist/* src/service/dist/`。
>
> AI 助手（SSE）页面 `src/service/chat/` 若属独立前端工程，同样按其构建产物放入 `src/service/chat/server/app/index.html` 即可；当前仓库如无该工程可跳过（需相应去掉 `yearning.go` 中的 embed 声明，否则见下报错）。

### 3.2 仅开发后端（最小占位）

无前端产物时，为让 `go build` 通过，可放入最小占位文件（见下方命令）。此时页面为空壳，但 HTTP API / 路由 / JWT 等后端逻辑可正常开发与自测。

```powershell
New-Item -ItemType Directory -Force -Path src/service/dist, src/service/chat/server/app | Out-Null
@'
<!DOCTYPE html><html><head><title>Yearning</title></head><body><div id="app"></div></body></html>
'@ | Set-Content -Encoding UTF8 src/service/dist/index.html
@'
<!DOCTYPE html><html><head><title>Yearning Chat</title></head><body><div id="app"></div></body></html>
'@ | Set-Content -Encoding UTF8 src/service/chat/server/app/index.html
```

> Linux/macOS 直接用仓库自带脚本（已存在的文件不会被覆盖，可安全重复执行）：
>
> ```bash
> ./scripts/prepare-embed.sh
> ```
>
> 干净 clone 后 `go build ./...` 报 `pattern dist/*: no matching files found` 或
> `pattern chat/*: no matching files found`，就是缺这一步。

---

## 4. 安装依赖

### 4.1 Go 依赖（统一国内加速源）

Go 依赖已由 `go.sum` 管理（dev 分支已恢复其版本控制）。仓库**统一使用国内加速源**，请先配置一次（会写入本机 go env，全局生效；`.\scripts\dev.ps1 prepare` 也会自动执行）：

```powershell
go env -w GOPROXY=https://goproxy.cn,direct
go env -w GOSUMDB=sum.golang.google.cn
```

再拉取/校验依赖：

```powershell
cd c:/hub/Yearning
go mod tidy
```

### 4.2 前端依赖（统一国内镜像）

`front/` 下已内置 `front/.npmrc`，将 registry 固定为国内镜像
`https://registry.npmmirror.com`，`npm`/`yarn` 安装时会自动生效，无需额外配置：

```powershell
cd front
yarn install     # 或 npm install
```

> 如需临时切回官方源，可删除 `front/.npmrc` 或执行 `npm config set registry https://registry.npmjs.org`。

---

## 5. 编译校验（可选但推荐）

```powershell
go build ./...     # 全量编译（dev 分支已在此版本下验证通过）
go vet ./...       # 静态检查（可选；有两个升级前已存在的存量告警，与本次升级无关）
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

SQL 解析 / 审核 / 执行引擎**源码在本仓库 `engine/`**（独立 Go module，`go.mod` 通过 `replace engine => ./engine` 引用，grpc 定义见 `engine/proto/engine/v1/engine.proto`）。它是**独立进程**，单独编译部署后，后端经 `[General].RpcAddr` 指明的地址（默认 `127.0.0.1:50001`）调用其方法：

| RPC 方法 | 用途 |
|---|---|
| `Engine.Check` | 提交前 SQL 语法 / 规则检查 |
| `Engine.Query` | 查询语句预检、拆条、LIMIT 注入、敏感列识别 |
| `Engine.Exec` | 工单执行 |
| `Engine.MergeAlterTables` | 合并多条 ALTER |
| `Engine.StopDelay` | 停止定时执行 |

### 8.1 编译并启动引擎

```powershell
cd engine
go build -o ../bin/engine ./cmd/engine   # 产出引擎可执行文件
../bin/engine                             # 默认监听 127.0.0.1:50001
```

> `engine/.gitignore` 已忽略引擎自身的编译产物（`/bin/`、`*.exe`、`*.test`）。

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
| `go build` 报 `chat/*: no matching files found` | 前端 embed 产物缺失，见第 3 节在 `src/service/dist/` 放入产物或使用最小占位 |
| 进程秒退，日志含 `SecretKey 强度不足或仍是模板示例值` | `conf.toml` 的 SecretKey 仍是占位值，替换为随机串（第 2 节） |
| `go mod tidy` / `go get` 网络超时 | 未启用国内源；先按第 4.1 节 `go env -w GOPROXY=https://goproxy.cn,direct` |
| 提示工具链损坏、`textflag.h:1: expected identifier` | 使用了损坏的 Go（含此前临时目录 `%USERPROFILE%\sdk`）。请用官方安装包重装到默认路径 |
| 页面空白 | 使用了第 3.2 节的占位产物；接上真实前端构建产物即可 |
| 端口被占用 | `run` 命令加 `-p <端口>` |
| `go.sum` 未被 git 跟踪 | dev 分支已恢复跟踪；勿再从 `.gitignore` 删除/忽略它 |
| 修改配置文件后需重启 | 配置在启动时读取，改动后重启进程 |

---

## 11. 功能测试（对已部署环境跑一遍）

`tools/ftest` 是端到端功能测试：走真实链路（主程序 HTTP/WS → 审核引擎 gRPC → 目标库），
断言直接读元数据库与目标库核对，不只看接口返回码。适合每次发版后对目标环境跑一遍。

```bash
go run ./tools/ftest \
  -base http://127.0.0.1:8000 \
  -meta-dsn 'root:pwd@tcp(<元数据库>:3306)/Yearning_go?charset=utf8mb4&parseTime=true' \
  -target-dsn 'demo:pwd@tcp(<目标库>:3306)/demo?charset=utf8mb4&parseTime=true' \
  -source-id <数据源 source_id> -schema demo -table users -id-column id
```

- 47 个用例：认证与越权（含**超级管理员拥有全部权限**：可审/可执行他人工单、数据源权限不依赖权限组）、
  工单全流程（立即/定时/人工执行、驳回、撤销、回滚语句、**编号 = 自增 id**）、
  项目级工单（批量提交多条 SQL、批次查询、逐条审核（审一条只通过该条）、批量执行、失败即停、提交原子性、
  **子工单严格按 -1、-2、-3… 排列**）、
  规则（级别灰度、自研规则、空载荷护栏、规则集增删与回滚）、查询（结果、脱敏、审计记录、导出开关、编号）、
  权限读写、上游版本检测、**工单编号规则不变量（含历史数据）**
- 工单列表（我的工单 / 工单审核 / 记录）统一**按项目聚合**：同批次的子工单在前端折叠成一行项目，
  展开后逐条查看/审核；**分页单位也是项目**，同一批的子工单不会跨页（见 `common.GroupedOrderPage`）
- 需要四个测试账号（默认 admin / dev1 / dba1 / readonly1）与至少两级的流程模板（`-approve-flag` 指定审批级下标）
- **测试数据会保留**（工单文本前缀 `FT-`，可在界面上核对现场）；过程中改过的配置（规则集、
  数据源脱敏字段、导出开关）会自动还原
- 退出码：0=全部通过，1=有用例失败，2=参数/环境问题

---

## 12. 数据源 ID 收敛（UUID → 4 位短 ID）

新建数据源已经是 4 位短 ID（`factory.NextSourceId()` 查重分配，撞车会自动加长）。
历史库里遗留的 UUID 形态 ID 用专用工具收敛（**幂等**，默认只预演、加 `-apply` 才写入）：

```bash
go run ./tools/shorten-source-id -meta-dsn 'root:pwd@tcp(<元数据库>:3306)/Yearning_go?charset=utf8mb4'
go run ./tools/shorten-source-id -meta-dsn '...' -apply
```

它会同步改写所有引用：`core_sql_orders` / `core_query_orders` / `core_auto_tasks` 的 `source_id`，
以及 `core_role_groups.permissions` 里 `ddl_source` / `dml_source` / `query_source` 列表中的旧 ID
—— **权限就是按 source_id 授权的，漏改会让这些数据源对所有人变成「没有权限」**（表现为接口报
`没有该数据源权限`、检测/查询全部失败）。改完重启主程序再核对。

> ⚠️ 不要用 `migration` 那个工具做这件事：它是给「≤ v3.0.0 老库」做破坏性升级的
> （会重建权限组 group_id、重写权限列表、删列），在当前版本的库上重复执行会把权限配置洗成空。

---

## 13. 提交与分支约定（本次安全修复分支）

```powershell
git checkout dev
git add -A
git commit -m "scope: 简述改动"
git push origin dev
```

- `dev` 分支承载开发/安全修复，`main`/`next` 为稳定线。
- 合入稳定线前请在正常 Go 环境跑一次 `go build ./...` 与 `go test ./...`。
- `SecretKey`、数据库口令等敏感信息**严禁**进入提交。

---

## 14. 工单编号规则

SQL 工单与查询工单的编号统一为「自增 id」形态（`factory.GenWorkId` 已删除）：

- **普通工单**：`work_id` = 该行的自增 `id`（如 `144`）。做法是先落库拿到 id，再回填 `work_id`。
- **项目级工单**：项目号 = 项目内**首条子工单**的自增 id；各子工单编号为 `项目号-1`、`项目号-2`…
  （按提交顺序），并共享 `batch_id` = 项目号。
- 生成入口统一为 `factory.OrderNo(id, seq)`：`seq=0` 为普通工单/项目号，`seq>0` 追加 `-序号`。
- **列表里的顺序**：列表本身按「待审批优先 + 时间」排序，会把同一项目的子工单打散（-2 排到 -1 前）。
  因此 `common.GroupedOrderPage` 出来后会做一次 `groupBatchRows`：把同一批次收拢到该批次首次出现的
  位置并按 id 升序排列，保证展开后严格是 -1、-2、-3…（普通工单位置与排序不变）。
- 编号搜索按「精确命中 or 该项目全部子工单」（`common.AccordingToWorkId`）；数字编号用子串
  匹配会命中大量无关工单（搜 `1` 命中所有含 1 的编号）。

### 14.1 历史数据收敛

历史工单（随机 8 位编号）用专用工具一次性改写（**幂等**，默认只预演、加 `-apply` 才写入）：

```bash
go run ./tools/renumber-orders -meta-dsn 'root:pwd@tcp(<元数据库>:3306)/Yearning_go?charset=utf8mb4'
go run ./tools/renumber-orders -meta-dsn '...' -apply
```

它会同步改写引用旧编号的表：SQL 工单的 `core_sql_records` / `core_rollbacks` /
`core_workflow_details` / `core_order_comments`，查询工单的 `core_query_records`，
以及项目级工单的 `batch_id`（旧批次号是随机的，一并收敛成项目号）。改完重启主程序再核对。
