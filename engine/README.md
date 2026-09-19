# engine

Yearning 的独立数据库引擎适配服务，通过 **gRPC** 替代原 Juno `net/rpc`，首期支持 MySQL。

审核能力直接复用 Bytebase 原生实现：SQL 解析器与 MySQL 审核规则已从
[Bytebase 官方仓库](https://github.com/bytebase/bytebase)**脚本化内化**到 `internal/bytebase/`
（来源、布局与裁剪清单见该目录 [README](./internal/bytebase/README.md)）。引擎只做
「拆分 → 逐条审核 → 结果映射」，不重复实现任何规则，也不依赖第三方封装 module；
升级时用 `scripts/sync-bytebase.sh <ref>` 从上游重放同步，规则行为与 Bytebase 保持一致。

## 审核链路

```
SQL 文本
  → base.SplitMultiSQL(MySQL)             # Bytebase 拆分（internal/mysqlparse）
  → base.ParseStatements(Engine_MYSQL)    # Bytebase omni 解析器出 ParsedStatements
  → advisor.Check(Engine_MYSQL, ruleType) # Bytebase MySQL advisor 规则（table.*、column.*、naming.* 等 80+ 条）
  → []*storepb.Advice → enginev1.Record
```

规则开关沿用 Yearning `AuditRole` 语义，在 `internal/server/rules.go` 中映射为 Bytebase 的
`storepb.SQLReviewRule_Type` 枚举规则：

- 「强制/检查 X」类开关（如 `DDLEnablePrimaryKey`）开启即启用 `TABLE_REQUIRE_PK` 等规则；
- 「允许 X」类开关（如 `AllowCreateView`）关闭即启用 `SYSTEM_VIEW_DISALLOW_CREATE` 等禁止规则。

映射关系随 `AuditRole` 变化维护；Bytebase 规则集中没有对应实现的开关（`IsOSC`、`DMLTransaction`
等执行期能力）交由执行阶段处理。升级 Bytebase 后规则枚举若有增删，会在编译期暴露并需同步调整映射。

有两类规则**刻意不映射**：

- 依赖 `advisor.Context.FinalMetadata`（库内元数据快照）的规则——`COLUMN_NO_NULL`
  （对应 `DDLCheckColumnNullable`）与 `INDEX_TOTAL_NUMBER_LIMIT`（对应 `DDLMaxKey`）。
  引擎目前只做静态审核、不提供该快照，映射上去会在规则内 nil 解引用 panic，
  被 advisor 兜住后误报成「SQL 语法错误」（DDL 全挂）。接入元数据快照后再启用，
  回归用例见 `internal/server/check_test.go` 的 `TestCheckDDLNotPanicOnMetadataRules`。
- 需要事前/事后状态模拟的规则同理（当前审核链路不建模拟库状态）。

## 现状

- `EngineService.Check` ✅ 拆分 + 语法校验 + Bytebase MySQL 审核规则，返回逐条 `Record`
- `EngineService.Query` ✅ 查询 SQL 拆分 + 逐条校验，返回逐条 `Record`（含敏感字段词表）
- `EngineService.MergeAlterTables` ✅ 同表 `ALTER TABLE` 合并，跨表/非 ALTER 原样保留
- `EngineService.Exec` ✅ 连目标业务库拆分执行工单 SQL，统计影响行返回逐条明细（含事务与失败处理）
- `EngineService.StopDelay` ⏸ 延迟工单调度需接入 Yearning 元库连接与调度器，暂未接入

`Exec` 的逐条执行明细由 Yearning 主程序回写 `core_sql_records` 表，回滚语句回写 `core_rollbacks`。

## 回滚语句（Exec，`backup=1` 的工单）

两级方案，按可用性自动选择（`internal/server/binlog.go` / `rollback.go`）：

| 方案 | 触发条件 | 覆盖范围 |
| --- | --- | --- |
| **binlog 抓取**（默认） | 源库 `log_bin=ON` + `binlog_format=ROW` + `binlog_row_image=FULL`，且数据源账号有 `REPLICATION SLAVE, REPLICATION CLIENT` | **全部 DML**：多表 UPDATE/DELETE、无 WHERE 的全表操作、无主键表、`INSERT ... SELECT`、`REPLACE` 等 |
| 前镜像 SELECT（兜底） | 上述条件不满足 | 单表带 WHERE 的 UPDATE/DELETE、显式主键的 INSERT |

binlog 方案会在执行前记录位点，执行完成（含提交）后读取新位点，拉取这段区间的行事件，
按 `before/after image` 生成**逆序**回滚语句：

- `INSERT`/`REPLACE` 写入的行 → 按行删除（有主键用主键精确定位；无主键按全列 `<=>` 匹配并 `LIMIT 1`）
- `DELETE` 删除的行 → 按行 `INSERT` 回填
- `UPDATE` 改动的行 → 把有差异的列改回 before 值（定位用 after 镜像，主键被改也能还原）

只收集工单目标表的事件（窗口内的其他会话写入不属于本工单）；抓取超时 10s 就用已有事件。
语句失败时也会抓取已生效改动的回滚语句——主程序先落 `core_sql_records` / `core_rollbacks` 再报错，
保证部分执行的工单有据可回滚。

## 依赖说明

解析器与审核规则已内化，`go.mod` 只保留运行所需的最小依赖：`github.com/bytebase/omni`
（Bytebase 的 SQL 解析器）、`github.com/zeebo/xxh3` / `github.com/hashicorp/golang-lru/v2`
（解析器内部缓存）、`github.com/go-mysql-org/go-mysql`（回滚语句的 binlog 抓取）、
`protobuf` / `grpc`。不存在 `replace` 指令，也不再引入 tidb 的 SQL 解析器 / 各方言解析器 /
cel-go / connectrpc / LSP 等无关依赖。

> `go-mysql` 的 `client` 包间接引入 `tidb/pkg/parser` 的少量子包
> （`mysql` / `format` / `terror` / `charset` 常量，非 SQL 解析器），`go.mod` 中会以 indirect 出现，
> 这是换取成熟 binlog 协议实现的代价；自研 binlog 解码的风险（各类型 charset/精度解码出错
> 会直接写坏回滚语句）远大于这几个常量包。

内化代码的来源、目录说明、裁剪清单与升级注意事项见
[`internal/bytebase/README.md`](./internal/bytebase/README.md)；升级时执行
`./scripts/sync-bytebase.sh <bytebase-ref>` 重放同步。

为避免同步上游时被迫大面积重构，引擎把「上游 API 漂移」的爆炸半径锁死在适配层
（`internal/mysqlparse/`、`internal/server/`）内，并用 `internal/archguard/` 的测试自动拦截：
越界 import、引用未注册/已改名的规则都会让 `go test ./...` 直接失败（详见
[`internal/bytebase/README.md` 的「升级防炸（护栏）」](./internal/bytebase/README.md#升级防炸护栏)）。

## 规则分两层：上游同步 + 自研扩展

| 层 | 位置 | 同步时 |
| --- | --- | --- |
| 上游规则（Bytebase） | `internal/bytebase/` | **整体重建**（勿手改） |
| 自研规则 | `internal/customrules/` | **完全不动** |

### 规则级别：拦截 / 提示 / 观察

规则集（设置 → 审核规则）里每条规则可单独设级别，存在 `audit_role.RuleLevel[字段名]`：

| 级别 | 引擎行为 | 前端 |
| --- | --- | --- |
| `error`（默认，未配置即此） | 命中判「审核不通过」，`Record.Level=1` | 禁用提交 / 同意 |
| `warn` | 命中给提示，`Record.Level=2`、状态「警告」 | 只提示，可提交 |
| `observe` | 命中只记录，`Record.Level=3`、状态「观察」 | 只提示，可提交 |

存在的意义是**灰度**：新规则先 `observe` 观察命中率与误报 → `warn` 提醒开发 → 误报可控再收紧成 `error`，
全程只改配置、不发版。实现见 `internal/server/rules.go`（`reviewPlan` 按级别分组，
`check.go` 依次执行「错误 → 警告 → 观察」，只有错误组命中才判不通过）。

两层由 `internal/server/check.go` 编排在同一链路，命中统一映射为 `Record`；
`trim-bytebase.py` / `sync-bytebase.sh` 不触碰 `internal/customrules`，
并有 `archguard.TestSyncDoesNotTouchCustomRules` 兜底。
新增自定义规则见 [`internal/customrules/README.md`](./internal/customrules/README.md)。

## 上游规则更新

Go 是编译型语言，Bytebase 规则是 Go 代码，**无法运行时热加载**；因此"更新"= 重新同步 + 重新构建 + 重新发布。
在此基础上，引擎把「跟随上游」自动化到接近实时：

```bash
./scripts/check-upstream.sh              # 对比当前内化版本与上游最新 release（退出码 10 表示有更新）
./scripts/check-upstream.sh --latest     # 只打印最新版本
./scripts/sync-bytebase.sh <ref>         # 同步并跑全套护栏
```

CI 侧见 [`.github/workflows/check-bytebase-upstream.yml`](../../.github/workflows/check-bytebase-upstream.yml)：
每天只做检测，发现新 release 就让 job 失败作为提醒；**CI 不修改任何代码，更新由人工执行**。

### Docker 升级与回滚（无预发环境）

线上引擎是独立容器（`engine/Dockerfile`，默认监听 `:13307`，无状态、不连元数据库）。
没有预发环境，所以**发布门禁 = 离线行为 diff**：让线上正在跑的实例与候选实例，用同一批真实工单 SQL、
同一份审核规则各跑一遍，逐条比对审核结果。差异即本次升级引入的行为变化，须逐条确认是「预期收紧/放松」还是「回归」。

```bash
# 1) 构建候选镜像（标签带 git 短 sha，回滚靠它）
docker build -t yearning-engine:$(git rev-parse --short HEAD) engine/

# 2) 起候选实例到临时端口，不碰线上
docker run -d --name engine-canary -p 13308:13307 yearning-engine:<sha>

# 3) 行为 diff：线上实例 vs 候选实例（SQL 与规则都取自元数据库）
go run ./tools/checkdiff \
  -old <线上引擎>:13307 -new 127.0.0.1:13308 \
  -meta-dsn 'user:pwd@tcp(<元数据库>:3306)/Yearning_go' -limit 200
#    退出码 0=全部一致，可以发布；1=有差异，逐条确认后再决定

# 4) 发布：同端口替换，容器名/端口/网络与原来一致
docker rm -f engine-canary
docker rm -f yearning-engine
docker run -d --name yearning-engine --restart always -p 13307:13307 yearning-engine:<新 sha>

# 5) 回滚：旧镜像 tag 还在本地，重新 run 旧 tag 即可（秒级）
docker run -d --name yearning-engine --restart always -p 13307:13307 yearning-engine:<旧 sha>
```

几点要记住：

- 替换瞬间（几秒）检测会失败一次：主程序每次请求新建 gRPC 连接、`conf.toml` **不热加载**，
  因此**换端口或换地址必须重启主程序**；同端口替换只需审核人重试一次即可。
- diff 用的是「当前全局规则集」，配置本身不参与比较——门禁只看引擎行为。
- 没有元数据库权限时可先导出 SQL：`-sql-file`（每行一个工单）+ `-rule-file`（`audit_role` JSON）。
- 发布后到「设置 → 审核规则 → 检测上游更新」核对版本号（该数字来自主程序的 `bytebaseRef` 常量，
  升级引擎时要同步改，否则会一直提示有新版本）。

需要数据库元数据或执行计划才能判定的规则（`statement.dml-dry-run`、`column.no-null` 的存量判定、
`statement.affected-row-limit` 等）当前未启用：审核阶段不连业务库，待需要时通过
`CheckRequest.Source` 注入 `sql.DB` 与 `DatabaseSchemaMetadata` 即可打开。

## 目录

```
engine/
├── proto/engine/v1/engine.proto   # gRPC 契约
├── gen/engine/v1/                 # 生成的 Go 代码（enginev1）
├── cmd/engine/                    # 服务端入口
├── internal/bytebase/             # 内化的 Bytebase 解析器与 MySQL 审核规则（镜像上游 backend/）
├── internal/mysqlparse/           # 解析器封装：语句拆分 / 语法校验
├── internal/server/               # gRPC 实现（check.go 审核链路、rules.go 规则映射）
├── internal/customrules/          # 自研审核规则（同步不覆盖，可自由扩展）
├── internal/archguard/            # 架构护栏测试：耦合边界 + 规则映射一致性
├── scripts/                       # sync-bytebase.sh 同步 / trim-bytebase.py 裁剪
├── buf.gen.yaml / buf.yaml        # proto 生成配置
├── Dockerfile
└── README.md
```

## 生成代码（改 proto 后执行）

```bash
cd engine
buf generate        # 需 PATH 含 protoc-gen-go / protoc-gen-go-grpc
```

## 运行

```bash
go run ./cmd/engine -addr :13307
```

## 构建（amd64 / arm64）

```bash
GOOS=linux GOARCH=amd64 go build -o bin/engine-linux-amd64 ./cmd/engine
GOOS=linux GOARCH=arm64 go build -o bin/engine-linux-arm64 ./cmd/engine
```

## 接入 Yearning

Yearning 通过本地 `replace` 引用本 module：

```go
// Yearning go.mod
require engine v0.0.0
replace engine => ./engine
```

并将 `src/lib/calls` 与各调用点切换到 `enginev1.EngineServiceClient`。
