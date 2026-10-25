# 统一「数据引擎」抽象 · 设计

> **方向变更（2026-10-23，覆盖此前「两态通道」与 `CommandExecuter` 的决策）**：
> 所有数据引擎的执行与查询都进引擎 HTTP 通道；主程序只留元数据 / 补全 / 提交硬保底。
> 即 `ExecuteWorkOrder` 对所有引擎都走 `client.Exec`，查询 WS 对所有引擎都走 `client.Query`；
> `dbengine.CommandExecuter` 撤销，`dbengine` 收缩为元数据 / 补全 / 硬保底三类能力。

## 目标与边界

现在主程序里按 `db_type` 处处 `switch`（执行、元数据、查询、提交硬保底、补全，
约 10 个文件）。本设计把它们收敛为一个**主程序侧的适配器接口 + 注册表**，
keyed by `db_type`。

**要统一**：执行 / 回滚、元数据（库·表·字段·索引）、补全、提交硬保底。

**不统一（刻意保持）**：
- 规则审核：已经是统一入口（引擎 HTTP `Check` 按 `kind` 分派，`audit_role` 一份配置）。
- 规则**实现**：bytebase AST（SQL） vs `mongocheck` / `redischeck`（命令解析），永远不同。
- 执行通道：SQL（mysql/PostgreSQL）走引擎 HTTP `Exec`（binlog/OSC/前镜像），Mongo/Redis 走主程序驱动——接口里 `Execute` 对 SQL 只是"调引擎"，不强扭进同一个 DSN。

## 接口

```go
// src/lib/dbengine/engine.go
package dbengine

type Field struct{ Name, Type string }
type Index struct{ Name string; Unique bool }

// Outcome 一次执行的汇总：影响行数 + 回滚语句 + 无法自动回滚的说明。
// SQL 引擎的回滚由引擎 HTTP 返回，命令式引擎由主程序抓前镜像——这里统一成同一形状。
type Outcome struct {
	Affected int64
	Rollback []string
	Warning  string
}

type Engine interface {
	Kind() string // mysql / postgres / mongodb / redis

	// ---- 元数据 ----
	ListDatabases(ctx context.Context, src *model.CoreDataSource) ([]string, error)
	ListCollections(ctx context.Context, src *model.CoreDataSource, db string) ([]string, error)
	DescribeTable(ctx context.Context, src *model.CoreDataSource, db, table string) ([]Field, error)
	ListIndexes(ctx context.Context, src *model.CoreDataSource, db, table string) ([]Index, error)

	// ---- 补全 ----
	// 返回给前端 monaco 的建议：vl=插入文本，meta 仅作分组说明（Schema/Table/Fields）。
	Highlight(src *model.CoreDataSource, isField bool, db string) []map[string]string

	// ---- 变更 ----
	// 提交时的硬保底：不读规则集、不依赖引擎在线。SQL 引擎返回 nil（解析由引擎做）。
	ValidateChange(src *model.CoreDataSource, sql string) error
	// 执行：含回滚。SQL 走引擎 HTTP（规则复检/影响行上限在引擎内），
	// Mongo/Redis 走主程序驱动（硬保底 → 前镜像 → 执行）。返回明细与回滚，由上层落库。
	Execute(ctx context.Context, src *model.CoreDataSource, order *model.CoreSqlOrder, actor string) (*Outcome, error)

	// ---- 查询（最后迁移，可不强制） ----
	// Query 执行一条只读命令/语句，返回结果集。脱敏与 limit 仍由上层统一处理。
	// Query(ctx, src, db, sql) ([]map[string]any, error)  // 见「迁移 checklist」第 4 批
}

// For 按 db_type 取引擎；未注册的类型返回一个「拒绝一切」的占位实现。
func For(dbType int) Engine
```

## 四个实现

| 引擎 | Kind | 关键差异 |
|---|---|---|
| `sqlEngine{kind, sysSchemas}` | mysql / doris | `Execute` 调 HTTP `Exec`；元数据共用 `information_schema`（差异仅系统库列表）；`ValidateChange` 恒 nil |
| `postgresqlEngine` | postgres | 同上，元数据走 `pg_indexes`/`columns` |
| `mongoEngine` | mongodb | `Execute` = `mongodb.Validate` + `ConnectMongo` + `mongodb.Execute`；`ListCollections`=`MongoCollections`；`DescribeTable`=`MongoCollectionSchema`（字段名）；`ValidateChange`=`mongodb.Validate` |
| `redisEngine` | redis | 包一层 `rediscmd` + `model/redis.go` 现有函数；回滚能力以 `Warning` 声明（如无前镜像） |

每个实现文件只薄薄包一层既有函数，不搬业务逻辑——第一步是**消除 switch，不是改写语义**。

## 迁移 checklist（分批，每批独立可回退）

### 批次 1 · 接口 + 注册表 + 元数据/补全/硬保底 — ✅ 已完成
- 新建 `src/lib/dbengine/{engine.go, registry.go, sql.go, mongo.go, redis.go, rejected.go, engine_test.go}`。
- 接口当前含 `Kind / ListDatabases / ListCollections / DescribeTable / Highlight / ValidateChange`
  （**不含 `Execute` 与 `ListIndexes`**：前者放批次 2，后者随批次 3 的索引迁移一起进）。
- 实现现状：mongo/redis 的元数据与硬保底已包到既有函数；sql 的元数据用 information_schema / `Pg*` 助手
  （mysql 的 `ExcludeDbList` 过滤留 TODO 到批次 3）；`sqlEngine.Highlight` 暂返回 nil（批次 2 迁移 `common.Highlight`）。
- 判据：`go build ./...` ✅；`go test ./src/lib/dbengine/` ✅（`TestFor` / `TestMongoValidateChange` / `TestRedisValidateChange`，离线可跑）。

### 批次 2 · 命令式引擎的执行 + 补全 — ✅ 已完成
- **实现偏差**：SQL 执行**没有**搬进 dbengine（它和 `RunOSC`/`oscUpdate`/binlog 纠缠在 audit 里，抽进去成环）。
  改为新增 `CommandExecuter` 接口（`Engine` + `Execute(src, order, actor) error`），只由 mongo/redis 实现；
  `ExecuteWorkOrder` 用 `dbengine.For(...).(dbengine.CommandExecuter)` 类型断言分流，SQL 内联路径原地不动。
- 搬移清单（逐段对照 HEAD 里的旧实现，无遗漏）：
  - `audit/mongo.go` → `dbengine/mongo.go`：`executeMongoOrder` + `recheckMongoRules` + `recordExec` + `appendOSCInfo`；
  - `audit/redis.go` → `dbengine/redis.go`：`executeRedisOrder` + `recheckRedisRules`；
  - 共享辅助收敛到 `dbengine/exec.go`：`recheckChange`（mongo/redis 复检合一，超时参数化）、`recordExec`、
    `saveRollbacks`、`writeWorkflowDetail`、`appendOSCInfo`（`oscUpdate` 语义已逐字内联）。
  - 删 `audit/mongo.go`、`audit/redis.go`；`ExecuteWorkOrder` 的两个分支退化成一行类型断言。
- 补全：`common.Highlight` 退化成 `dbengine.For(...).Highlight(...)`；`common.mongoHighlight` 删除（已并入 `mongoEngine.Highlight`）；
  mysql 的 information_schema 逻辑搬进 `sqlEngine.Highlight`（含 `ExcludeDbList` 的 NOT IN），PostgreSQL 仍返回空（与迁移前一致）。
- 判据：`go build ./...` ✅、`go vet` ✅、`go test ./src/...` ✅、引擎 `go test ./...` ✅、`vue-tsc` ✅。
  实机对拍待本地数据库就绪（本机仅 Redis，MySQL/Mongo 需部署）。

### 批次 3 · 元数据 + 提交硬保底 — ✅ 已完成
- 提交硬保底：`personal/post.go`、`personal/batch.go` 的 `switch src.DBType` → `dbengine.For(...).ValidateChange(...)`；
  删除 `checkMongoOrder` / `checkRedisOrder`（逻辑即 `mongoEngine.ValidateChange` / `redisEngine.ValidateChange`）。
- 元数据：`fetch.go` 的 `FetchBase` / `FetchTable` 里 mongo/redis/PostgreSQL 三个 early-return 分支 → `dbengine.For(...).ListDatabases/ListCollections(...)`。
- **刻意保留内联**：MySQL 的 `SHOW DATABASES` + `ExcludeDbList(Hide)` / `SHOW TABLES` 语义与 info_schema 不同（权限视图/系统库），
  硬统一会改行为，故 mysql 路径暂不动；字段/索引（`FetchTableFieldsOrIndexes`）因 `Field/Index` 契约与 `common.FieldInfo/IndexInfo`
  未对齐、且 `ListIndexes` 还没进接口，也暂不动。
- 判据：`go build ./...` ✅、`go test ./src/...` ✅、引擎 `go test ./...` ✅。

### 批次 4 · 查询 WS — 暂缓（有意识地留分支）
- 三个引擎的查询执行早已收敛到同一 `*Query{Field,Data}` 契约（`MultiSQLRunner.Run` / `runMongoCommand` / `runRedisCommand`），
  但 `Query` 类型与三条 WS 循环（`SocketQueryResults` / `socketMongoResults` / `socketRedisResults`）都写在 `personal` 包里，
  且与 msgpack、limit、脱敏、查询审计 `saveQueryRecord` 纠缠。
- 抽成 `Engine.Query` 需要把 `Query` 类型下移 + 重写三处 WS 循环，收益（消除 ~2 处重复）低于回归风险。
- 结论：本批不做，留作独立任务；接口里也**不加** `Query`（YAGNI，等真要做时再定义）。
- `src/handler/fetch/{fetch.go,impl.go}` 的 `FetchBase`/`FetchTable`/库表树 → 接口。
- `src/handler/personal/{post.go,batch.go}` 的 `checkMongoOrder`/`checkRedisOrder` → `ValidateChange`。
- 判据：四引擎的库/表/字段/索引接口返回一致；Mongo/Redis 提交非法命令仍被拒、SQL 不受影响。

### 批次 4 · 查询 WS（最重，最后）
- `src/handler/personal/{query.go,mongo.go,redis.go}` 的执行分支 → `Engine.Query`（返回行集），
  脱敏/limit/audit 仍在上层共用。
- 判据：四引擎查询页回归；此批若发现 SQL 与命令式的执行/脱敏语义纠缠过深，允许**只抽 Query 签名、留部分分支**并说明原因。

## 对拍方案（每一批的行为门禁）

1. **单测**：`dbengine` 包对每个引擎的 `ValidateChange` / `Highlight`（用假源或真实函数）加最小用例。
2. **引擎审核对拍**（升级/重构后）：`tools/checkdiff` 拿「迁移前 build 的引擎」与「迁移后 build 的引擎」对同一批语料（含四引擎工单）diff 输出，差异应为空。
3. **主程序行为矩阵**：四引擎 × {提交硬保底, 执行, 回滚, 元数据, 补全} 各打一枪，对照迁移前的记录（`core_sql_records`/`core_rollbacks` 落库一致）。
4. 每批**先跑一遍迁移前的基线**（记录输出），迁移后再跑，diff 为空才算过。

## 全引擎走统一通道(HTTP) · 迁移计划（2026-10-23 新方向，覆盖上文批次 2/4 与「不做」条目）

> 上文「批次 2」「批次 4」以及「不做」里的两态通道描述已被新方向取代，仅保留历史记录。

**目标形态**：审核 `Check`（已统一）、执行 `Exec`、查询 `Query` 三种 RPC 全部按 `kind` 分派；
主程序只留元数据 / 补全 / 提交硬保底（`dbengine`），`ExecuteWorkOrder` 与查询 WS 都只剩一条 RPC 调用。

**分阶段**：

1. **引擎引入驱动与执行器**：`engine/go.mod` 加 `mongo-driver` + `go-redis`；
   把 `src/lib/mongodb`（Execute / 前镜像 / 回滚 / WatchIndexBuild）与 `src/lib/rediscmd`（Execute / ExecuteQuery / 前镜像）搬进
   `engine/internal/{mongodb,rediscmd}`，连接逻辑（`model.ConnectMongo` / `ConnectRedisDB`）在引擎侧用 `enginev1.DataSource` 重建一份。
   主程序里的 `Validate` / `ValidateQuery` 留在原处（提交硬保底仍本地）。
2. **引擎 Exec/Query 命令式实现**：`internal/server/exec.go` / `query.go` 按 `kind` 分派到 mongodb/rediscmd 实现；
   `Record.rollback` / `ExecReply.rollback` 已有字段承载回滚语句。
3. **主程序翻面**：`ExecuteWorkOrder` 删 `CommandExecuter` 分支，四引擎统一 `client.Exec`；
   查询 WS 的 `socketMongoResults` / `socketRedisResults` 收敛到 `client.Query`；删 `dbengine` 里的 `Execute`/`CommandExecuter` 与 `exec.go` 共享辅助。
4. **语义核对**：引擎离线时命令式执行/查询的 fail-closed（对齐 SQL）；`WatchIndexBuild` 进度回传与 `osc_info` 的跨进程写法；
   查询侧只读白名单 `ValidateQuery` 移入引擎 Query 路径。

**关键风险**：引擎依赖变重；`go get` 需网络（本机 Go proxy 此前有超时）；连接/口令语义要逐一核对；
查询从本地 fail-open 变为引擎 fail-closed。

**状态（2026-10-23，已实施）**：

- 阶段 1 ✅：`engine/internal/{mongodb,rediscmd}` 搬入执行包（含测试）；引擎 `go.mod` 引入 `mongo-driver v1.17.10` + `go-redis v9.22.0`（与主模块同版本）。
- 阶段 2 ✅：`engine/internal/server/dsconn.go` 从 `enginev1.DataSource` 重建 Mongo/Redis 连接（口令已明文，不再解密）。
- 阶段 3 ✅：`internal/server/exec_command.go` 实现 `execMongo`/`execRedis`，`Exec` 按 kind 分派；硬保底、前镜像、回滚都在引擎侧，映射回 `ExecReply{Records, Rollback}`。
- 阶段 4 ✅：`ExecuteWorkOrder` 删 `CommandExecuter` 分支、四引擎统一 `client.Exec`；`dbengine` 收缩为元数据/补全/硬保底；
  主模块删 `mongodb/{exec,indexprogress}.go`、`rediscmd/{exec,snapshot}.go`（执行已搬走）。
- **查询侧最终结论**：规则审核本就在引擎（Check mode=query）；白名单硬保底（`ValidateQuery`/`rediscmd.Validate` 的只读判定）**留在本地**——它是 fail-safe，
  搬进引擎会让「查询页不能写」依赖引擎在线；查询执行与 MySQL 一致留在本地。故查询无需再搬，方案 B 的「预检进引擎」实质已满足。

**两个行为回归（有意，已在代码注释标注）**：

1. Mongo `createIndexes` 的实时构建进度（`WatchIndexBuild` → `osc_info`）：引擎没有主程序元数据库，进度回传被跳过；
   索引是否构建完成仍由执行结果体现，详情页「在线变更」面板不再实时刷新进度。
2. 执行前规则复检（`recheckChange`, mode=write）：随 `CommandExecuter` 撤销；SQL 执行本来也不复检，现在 Mongo/Redis 与 SQL 一致。
