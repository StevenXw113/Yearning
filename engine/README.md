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
自研规则清单见 [`SELF_RULES.md`](./SELF_RULES.md)（自动生成：`UPDATE_SELF_RULES_MANIFEST=1 go test ./internal/archguard -run TestSelfRule`）；
上游规则清单 `RULES.md` 管「上游新增的有没有被漏掉」，`SELF_RULES.md` 管「自己写的有没有被漏掉」。

## 上游规则更新

Go 是编译型语言，Bytebase 规则是 Go 代码，**无法运行时热加载**；因此"更新"= 重新同步 + 重新构建 + 重新发布。
在此基础上，引擎把「跟随上游」自动化到接近实时：

```bash
./scripts/check-upstream.sh              # 对比当前内化版本与上游最新 release（退出码 10 表示有更新）
./scripts/check-upstream.sh --latest     # 只打印最新版本
./scripts/sync-bytebase.sh <ref>         # 同步并跑全套护栏
```

检测在页面上完成：「设置 → 审核规则 → 检测上游更新」，用 GitHub 的 `releases/latest` 302 取最新 tag，
与主程序里的内化版本号（`src/handler/manage/roles/upstream.go` 的 `bytebaseRef`，由同步脚本自动维护）比较。
**页面只做检测，不改任何代码**；升级在源码侧用下面的一键脚本完成。

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
  同步脚本会自动写入新版本号，无需手工改）。

### 源码侧升级流程（一条命令）

页面只负责告诉你「上游有新版本」，更新动作在源码侧完成：

```bash
cd engine
./scripts/upgrade-engine.sh            # 自动检测上游最新版本并升级
./scripts/upgrade-engine.sh 3.23.0     # 或指定版本 / 分支 / commit
```

脚本依次做：检测目标版本 → 重放同步（稀疏拉取 → 复制子集 → 改写 import → 重放裁剪 →
`go mod tidy` → `build` / `test` → 登记新增规则 → 更新主程序版本号）→ 打印规则增删 →
构建镜像 `yearning-engine:<ref>`（有 docker 时）→ 打印对拍与重新部署的命令。

跑完你要做三件事：审阅 `git diff` 并提交 → 对拍线上引擎 → 同端口替换容器（旧 tag 留回滚）。

只想先看看改动量、不动工作区：

```bash
./scripts/upgrade-engine.sh 3.23.0 --preview   # 等价于 PREVIEW=1，内部走 preview-upgrade.sh
```

同步脚本会自动做掉三件机械活，避免升级卡在半路：

| 自动处理 | 为什么需要 |
| --- | --- |
| `go mod tidy` | 上游可能新增依赖，不补 `engine/go.mod` 就直接编译失败 |
| 更新主程序的 `bytebaseRef` | 否则「检测上游更新」会一直显示有新版本 |
| 把新增规则按「未启用」登记进 `engine/RULES.md` | 否则 archguard 清单测试会失败 → 同步被判失败并回滚，升级卡死 |

任何一步失败，脚本都会把 `internal/bytebase` 还原成同步前的内容，**仓库不会停在半成品状态**。
预演脚本用的是 HEAD（已提交）的代码与脚本；改过同步脚本请先提交再预演。

### 把上游规则接到开关上

`engine/RULES.md` 里「未启用」的规则若要启用，需要四处一起改（少一处就会出现「页面勾了但不生效」）：

1. `engine/internal/server/rules.go`：把 Yearning 开关映射到该规则类型（含 payload：数值/列表/命名规范）
2. `engine/RULES.md`：`UPDATE_RULES_MANIFEST=1 go test ./internal/archguard -run TestRulesManifest` 重新生成
3. `front/src/views/manager/rules/rules.ts`：加一条开关（`name` 必须与第 1 步里用作 key 的字段名一致）
   + `front/src/lang/{zh-cn,en-us}` 文案
4. `src/engine/engine.go` 加字段 + `src/engine/convert.go` 的映射：**缺了会被 `SuperSaveRoles` 的
   `json.Marshal` 静默丢掉**（前端选了开关但永远传不到引擎）

默认级别是 `error`（命中即拦截）。想先灰度，就在规则集页把该规则设为「观察」或「提示」，
跑一段时间看命中率与误报，确认后再收紧。

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
