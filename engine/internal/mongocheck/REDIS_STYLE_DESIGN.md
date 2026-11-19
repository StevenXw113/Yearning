# Mongo 规则体系 Redis 风格改写设计

> **状态：设计稿，待确认，尚未实现。** 现行实现见同目录 `README.md`（20 条静态规则）。
> 本文描述的是目标形态；确认后再按第 7 节分阶段动代码。

## 1. 目标与"Redis 风格"的判定标准

Redis 规则体系（`engine/internal/redischeck/`，77 条）不是"规则多"这么简单，抽出来是可复制的五条工程约束：

1. **表驱动规则**：一条规则 = `enabled(Config) bool` + `hit(Config, *Command) string`，`init()` 里 `Register(commandRule{...})`，规则文件不再手写 `Check` 方法。
2. **1:1 细粒度开关**：一个风险点一个开关；多参数压进同一规则（如 `MongoIndexTtlRange` 的上下限），但不把"命令族 + 阈值 + 拓扑"揉成一条。
3. **运行时事实注入**：引擎不连库；部署形态、生产标记、版本、键/集合快照由主程序探测后塞进 `Config`，形态未知时规则不生效（不误判）。
4. **安全级别夹逼**：`security` 类默认 `error`，且用户配 `observe` 时强制降为 `warn`——权限与危险命令不允许"观察=放行"。
5. **命令语义单一来源**：只读/写命令表只在 `engine/api` 存一份，引擎与主程序共用，杜绝两份实现漂移。

改写目标：Mongo 达到上述五条 + 规则覆盖面对齐 Redis 的各风险族；**不追求凑够 77 条**，按 Mongo 实际命令空间定条数。

## 2. 分层与目标形态

| 层 | 位置 | 读规则集 | 作用 |
| --- | --- | --- | --- |
| 引擎静态审核 | `internal/mongocheck` + `internal/server/check_mongo.go` | 是 | 可配开关 + 可调级别 + 运行时事实；申请页检测与查询页检测 |
| 命令语义单一来源 | `engine/api/mongo_commands.go`（新增） | 否 | 只读/写/危险/管理命令表，`MongoIsReadCommand` 等助手，两端共用 |
| 主程序硬保底 | `src/lib/mongodb/command.go`、`query.go` | 否 | 引擎离线也必须在：空 filter 改删、删库、服务端脚本、查询页只读白名单 |
| 事实探测 | `src/lib/calls/convert.go`、`src/lib/mongodb/snapshot.go`（新增） | 否 | 填 `MongoMode/MongoVersion/IsProduction`，抓集合快照，复用 `src/lib/dbengine/mongo.go` 元数据能力 |
| 执行期 | `engine/internal/mongodb/exec.go` | 是（仅上限） | 抓前镜像 + `MongoMaxAffectRows` 命中数上限 |

硬保底继续不读规则集、继续独立实现判定——Redis 侧 `src/lib/rediscmd/command.go` 也是这么做的。单一来源只统一"命令分类表"，不统一"规则判定"，两者不冲突。

## 3. 核心实现形态

### 3.1 表驱动 `commandRule`（照搬 Redis）

`mongocheck/rule.go` 改用与 `redischeck/rule.go:91-118` 完全同构的实现：

```go
type commandRule struct {
	name    string
	desc    string
	cat     Category
	modes   []Mode
	level   Level
	enabled func(Config) bool
	hit     func(Config, *Command) string // 命中说明；空串=未命中
}

func (r commandRule) Check(cfg Config, cmd *Command) []Finding {
	if r.enabled == nil || !r.enabled(cfg) {
		return nil
	}
	if msg := r.hit(cfg, cmd); msg != "" {
		return []Finding{{Rule: r.name, Level: cfg.Level(r), Message: msg}}
	}
	return nil
}
```

`Register` 现有护栏（重名/空名/非法级别/安全类非 error/无模式 → panic）原样保留；`All()` 仍是 SELF_RULES 与 archguard 的唯一出口。现有 20 条规则逐步改成 `Register(commandRule{...})`，对外名称与语义不变。

### 3.2 `Command` 共享解析层要补的 helper

现有 `Command` 提供 `Lookup/Has/Walk/ClauseDocs/EmptyFilter/Indexes`，够写现有 20 条。新增规则需要补：

| helper | 用途 | 对应规则族 |
| --- | --- | --- |
| `Required(field string) bool` | 命令必填字段齐全（updates/deletes/indexes/pipeline） | 基础 |
| `Op(name string) bool` | 顶层操作符/字段存在（bypassDocumentValidation、multi、upsert、writeConcern.w） | 写入 |
| `Pipeline() []map[string]any` | aggregate 阶段数组，按序取首 `$match`/`$sort`/`$unwind` | 查询 |
| `Regexes() []string` | 递归收集 `$regex` 的 pattern 字符串 | 操作符 |
| `WriteConcernW() (string, bool)` | 解析 `w:0/1/majority/<n>` | 写入、环境 |
| `CollGlob(pattern string) bool` | 集合名 glob 命中（自实现，不引依赖，照 Redis `globMatch`） | 基础、范围 |
| `DbName() string` | 从 schema/命令取库名 | 范围、拓扑 |

### 3.3 级别夹逼（`config.go`）

照搬 `redischeck/config.go:209-221`：

```go
func (c Config) Level(r Rule) Level {
	lv := r.DefaultLevel()
	if s, ok := c.Levels[r.Name()]; ok {
		if v := Level(strings.ToLower(strings.TrimSpace(s))); ValidLevel(v) {
			lv = v
		}
	}
	if r.Category() == CategorySecurity && lv == LevelObserve {
		return LevelWarn // 安全类不允许"观察=放行"
	}
	return lv
}
```

这是相对现行 `mongocheck` 的**行为变更**，需要在第 8 节兼容性里显式提示。

### 3.4 模式硬隔离

`check.go` 现有逻辑保留：query 模式遇非只读命令 → 硬错误；write 模式遇只读命令 → 硬错误。新增规则靠 `Modes()` 声明 `allModes/writeOnly/queryOnly`，与 Redis 一致。

## 4. 完整规则清单

**记法**：`all` = write+query，`W` = writeOnly，`Q` = queryOnly；默认级别 `error/warn`；`来源` 列中 `保留` = 现有 20 条改名不变，`新增` = 本次加。参数列 `int>0` 表示 0=规则不生效。

### 4.1 基础与范围（6 条）

| 规则名 | 开关字段 | 类 | 默认 | 模式 | 判定 / 参数 | 来源 |
| --- | --- | --- | --- | --- | --- | --- |
| `MongoCommandSyntax` | CommandSyntax | structure | error | all | JSON 可解析、首键为字符串命令名、`updates/deletes/indexes/pipeline` 必填字段齐全 | 新增 |
| `MongoUnknownCommand` | UnknownCommand | security | error | all | 命令名不在 `api.MongoSemantics` 登记表中（模块/厂商扩展）；形态未知按未知处理，不按只读 | 新增 |
| `MongoCollectionExtraction` | CollectionExtraction | security | error | all | 目标集合未能从命令识别（无法约束范围，不 panic，交规则拦） | 新增 |
| `MongoForbiddenCollectionPattern` | ForbiddenCollectionPattern | security | error | all | 集合名命中保护 glob；多前缀逗号分隔 | 新增，string |
| `MongoSensitiveCollectionAccess` | SensitiveCollectionAccess | structure | warn | all | 集合名命中敏感 glob，提示走审批 | 新增，string |
| `MongoDbScope` | DbScope | security | error | all | 库名不在允许 glob 列表（空=不生效） | 新增，string |

### 4.2 危险与管理（9 条）

| 规则名 | 开关字段 | 类 | 默认 | 模式 | 判定 | 来源 |
| --- | --- | --- | --- | --- | --- | --- |
| `MongoForbidDangerous` | ForbidDangerous | security | error | all | `dropDatabase` / `eval` / `mapReduce`，并补 `cloneCollection` / `copydb` / `cloneCollectionAsCapped` | 保留（扩命令表） |
| `MongoForbidAdminCommand` | ForbidAdminCommand | security | error | all | `createUser` / `dropUser` / `grantRolesToUser` / `createRole` 等 13 条 | 保留 |
| `MongoForbidWhere` | ForbidWhere | security | error | all | 递归查 `$where` / `$function` / `$accumulator` | 保留 |
| `MongoForbidSystemCollection` | ForbidSystemCollection | security | error | all | 目标集合 `system.*` | 保留（扩到 all） |
| `MongoForbidShutdown` | ForbidShutdown | security | error | all | `shutdown` | 新增 |
| `MongoForbidReplicaChange` | ForbidReplicaChange | security | error | all | `replSetReconfig` / `replSetStepDown` / `replSetFreeze` | 新增 |
| `MongoForbidShardChange` | ForbidShardChange | security | error | all | `shardCollection` / `moveChunk` / `split` / `addShard` / `removeShard` / `enableSharding` / `balancer*` / `setFeatureCompatibilityVersion` | 新增 |
| `MongoForbidProfiling` | ForbidProfiling | security | error | all | `profile` / `setParameter` | 新增 |
| `MongoForbidSessionControl` | ForbidSessionControl | security | error | all | `killOp` / `killSessions` / `killAllSessions` / `killCursors` | 新增 |

### 4.3 数据变更（11 条）

| 规则名 | 开关字段 | 类 | 默认 | 模式 | 判定 / 参数 | 来源 |
| --- | --- | --- | --- | --- | --- | --- |
| `MongoForbidEmptyFilter` | ForbidEmptyFilter | security | error | W | `update[].q` / `delete[].q` / `findAndModify.q\|query` 为空 | 保留 |
| `MongoForbidImmutableId` | ForbidImmutableID | security | error | W | `$set`/`$unset`/`$rename` 触及 `_id`；`$setOnInsert` 与 insert 放行 | 保留 |
| `MongoForbidBypassDocumentValidation` | ForbidBypassDocumentValidation | security | error | W | 顶层 `bypassDocumentValidation:true` | 新增 |
| `MongoRequireAcknowledgedWrite` | RequireAcknowledgedWrite | security | error | W | `writeConcern.w:0`（火即忘，审核后无法确认落盘） | 新增 |
| `MongoForbidDropCollection` | ForbidDropCollection | security | error | W | `drop` / `renameCollection`（含 `dropTarget:true`） | 保留（补代码缺口） |
| `MongoForbidMultiUpdate` | ForbidMultiUpdate | structure | warn | W | `update`/`findAndModify` 带 `multi:true` | 新增 |
| `MongoForbidReplaceDocument` | ForbidReplaceDocument | structure | warn | W | 整体替换文档（无原子操作符），提示改用 `$set` | 新增 |
| `MongoForbidUpsert` | ForbidUpsert | structure | warn | W | `upsert:true`，提示可能意外新建文档 | 新增 |
| `MongoMaxWriteDocs` | MaxWriteDocs | performance | error | W | 单次 update/delete 命中数静态上限（`int>0`，0=不限）；执行期 `MongoMaxAffectRows` 继续兜底实际命中数 | 新增，int |
| `MongoMaxValueSize` | MaxValueSize | performance | error | W | 单文档 BSON 字节上限（`int>0`） | 新增，int |
| `MongoMaxInsertBatch` | MaxInsertBatch | performance | error | W | insert 批量文档数上限（`int>0`） | 新增，int |

### 4.4 查询与扫描（9 条）

| 规则名 | 开关字段 | 类 | 默认 | 模式 | 判定 / 参数 | 来源 |
| --- | --- | --- | --- | --- | --- | --- |
| `MongoQueryForbidNoFilter` | QueryForbidNoFilter | performance | warn | Q | `find` 无/空 filter；`aggregate` 管道无 `$match` | 保留 |
| `MongoQueryForbidLookup` | QueryForbidLookup | performance | warn | Q | 聚合含 `$lookup` / `$graphLookup` | 保留 |
| `MongoQueryForbidNoLimit` | QueryForbidNoLimit | performance | warn | Q | `find` 带 `sort` 或 `skip` 但无正数 `limit` | 保留（扩 skip） |
| `MongoQuerySortBeforeMatch` | QuerySortBeforeMatch | performance | warn | Q | 管道首个 `$match` 之前出现 `$sort` | 新增 |
| `MongoQueryUnwindBeforeMatch` | QueryUnwindBeforeMatch | performance | warn | Q | 管道首个 `$match` 之前出现 `$unwind` | 新增 |
| `MongoQueryForbidOutMerge` | QueryForbidOutMerge | security | error | Q | 聚合含 `$out` / `$merge`（写回集合，查询页不应出现） | 新增 |
| `MongoQueryMaxReturnDocs` | QueryMaxReturnDocs | performance | warn | Q | `limit` 超过上限（`int>0`） | 新增，int |
| `MongoQueryMaxPipelineStages` | QueryMaxPipelineStages | performance | warn | Q | 管道阶段数超过上限（`int>0`） | 新增，int |
| `MongoQueryForbidFullCount` | QueryForbidFullCount | performance | warn | Q | `count` / `distinct` 无 query（全集合统计） | 新增 |

### 4.5 操作符形态（5 条）

| 规则名 | 开关字段 | 类 | 默认 | 模式 | 判定 / 参数 | 来源 |
| --- | --- | --- | --- | --- | --- | --- |
| `MongoRegexUnanchored` | RegexUnanchored | performance | warn | all | `$regex` 未以 `^` 锚定 | 保留 |
| `MongoNegationOperator` | NegationOperator | performance | warn | all | 含 `$ne` / `$nin` / `$not` / `$nor` | 保留 |
| `MongoOrClause` | OrClause | performance | warn | all | 含 `$or` | 保留 |
| `MongoLargeInList` | LargeInList | performance | warn | all | `$in` 元素数超过上限（`int>0`） | 保留，int |
| `MongoMaxRegexPatternLen` | MaxRegexPatternLen | performance | warn | all | 正则 pattern 长度上限（`int>0`），抑制 catastrophic backtracking | 新增，int |

### 4.6 索引与结构（12 条）

| 规则名 | 开关字段 | 类 | 默认 | 模式 | 判定 / 参数 | 来源 |
| --- | --- | --- | --- | --- | --- | --- |
| `MongoForbidCappedConvert` | ForbidCappedConvert | structure | error | W | `convertToCapped`（不可逆重建） | 保留 |
| `MongoForbidCollMod` | ForbidCollMod | structure | warn | W | `collMod`（validator / TTL / 索引可见性） | 保留 |
| `MongoIndexKeyLimit` | IndexKeyLimit | structure | warn | W | 单索引键数超过上限（`int>0`） | 保留，int |
| `MongoIndexNameSpec` | IndexNameSpec | structure | warn | W | 索引名不匹配正则（空=不生效） | 保留，string |
| `MongoRequireIndexName` | RequireIndexName | structure | warn | W | `createIndexes` 未显式给 `name` | 新增 |
| `MongoMaxIndexNameLen` | MaxIndexNameLen | structure | warn | W | 索引名长度上限（`int>0`） | 新增，int |
| `MongoForbidTextIndex` | ForbidTextIndex | structure | warn | W | 建 `text` 索引 | 新增 |
| `MongoForbidWildcardIndex` | ForbidWildcardIndex | structure | warn | W | 建 `$**` 通配符索引 | 新增 |
| `MongoIndexTtlRange` | IndexTtlRange | performance | warn | W | `expireAfterSeconds` 超出下限/上限（多参数压一个开关） | 新增，int×2 |
| `MongoForbidDropAllIndexes` | ForbidDropAllIndexes | security | error | W | `dropIndexes` 的 `index:"*"` | 新增 |
| `MongoCollectionPrefix` | CollectionPrefix | security | error | all | 集合名不以参数前缀开头（空=不生效） | 保留，string |
| `MongoMaxCollectionNameLen` | MaxCollectionNameLen | structure | warn | all | 集合名长度上限（`int>0`） | 保留，int |

### 4.7 环境与拓扑（6 条，依赖运行时事实）

| 规则名 | 开关字段 | 类 | 默认 | 模式 | 生效条件 / 判定 | 来源 |
| --- | --- | --- | --- | --- | --- | --- |
| `MongoProductionWrite` | ProductionWrite | structure | warn | W | `IsProduction=true` 时的写命令提示 | 新增 |
| `MongoCommandCompatibility` | CommandCompatibility | security | error | all | 命令/操作符最低版本 > `MongoVersion`（空版本=不生效） | 新增 |
| `MongoRequireMajorityWriteConcern` | RequireMajorityWriteConcern | security | error | W | `Deployment=replica` 且写命令 `w != majority` | 新增 |
| `MongoForbidCrossShardRename` | ForbidCrossShardRename | security | error | W | `Deployment=sharded` 且 `renameCollection` 跨库/跨分片 | 新增 |
| `MongoMaxCommandCount` | MaxCommandCount | performance | error | all | 提交命令数超过 `CommandCount` 上限 | 新增，int |
| `MongoForbidChangeStream` | ForbidChangeStream | performance | warn | all | 聚合含 `$changeStream`（长连接游标） | 新增 |

### 4.8 集合快照（4 条，Phase 2，依赖 `CollectionSnapshot`）

| 规则名 | 开关字段 | 类 | 默认 | 模式 | 判定 | 来源 |
| --- | --- | --- | --- | --- | --- | --- |
| `MongoUniqueIndexOnNonEmpty` | UniqueIndexOnNonEmpty | security | error | W | 非空集合上建 `unique` 索引（大概率因重复值失败） | 新增 |
| `MongoIndexNameConflict` | IndexNameConflict | structure | warn | W | 索引名与现有索引冲突 | 新增 |
| `MongoLargeCollectionMultiWrite` | LargeCollectionMultiWrite | performance | warn | W | 文档数超阈值的集合上执行 `multi:true` 写 | 新增 |
| `MongoExistingDocumentOverwrite` | ExistingDocumentOverwrite | structure | warn | W | 整体替换已存在文档（快照命中） | 新增 |

**合计**：4.1–4.7 静态 58 条 + 4.8 快照 4 条 = **62 条**（其中保留现有 20 条、新增 42 条；再加执行期 `MongoMaxAffectRows` 共 63 个开关）。

## 5. 运行时事实注入

照 Redis 的四件套（`DataSource` 字段 → `CheckRequest` 字段 → `calls` 探测 → `check*` 注入）：

**`engine/api/api.go` 新增**
```go
type DataSource struct {
	// ... 现有字段
	MongoMode    string `json:"mongo_mode"`    // standalone/replica/sharded，空=未知，拓扑规则不生效
	MongoVersion string `json:"mongo_version"` // buildInfo.version，空=未知，兼容规则不生效
}
type CheckRequest struct {
	// ... 现有字段
	CollectionSnapshot map[string]CollectionState `json:"collection_snapshot"` // key=schema.collection
	CommandCount       int32                      `json:"command_count"`
}
type CollectionState struct {
	DocCount   int64        `json:"doc_count"`
	AvgDocSize int64        `json:"avg_doc_size"`
	Indexes    []IndexState `json:"indexes"`
}
```

**主程序探测**（`src/lib/calls/convert.go`，与 `RedisAudit` 并列）
```go
func MongoAudit(ds *api.DataSource, s *model.CoreDataSource, sql, schema string) map[string]api.CollectionState {
	if ds == nil || s == nil || s.DBType != model.DBTypeMongoDB { return nil }
	FillMongoFacts(ds, s) // hello/buildInfo → MongoMode/MongoVersion；IsProduction 已有
	return mongodb.CollectionSnapshot(s, schema, sql) // 复用 dbengine/mongo.go
}
```
`MongoMode` 判定：`hello.isWritablePrimary && setName != ""` → replica；`msg == "isdbgrid"` → sharded；其余 standalone。探测失败一律空串，**规则不生效而非误判**。

**引擎注入**（`check_mongo.go`）：与 `check_redis.go:23-34` 同形，`cfg.Deployment/MongoVersion/Production/CollectionSnapshot/CommandCount` 从 `req` 与 `req.Source` 填入。

**入口护栏**：新增 `src/archguard/mongo_audit_entry_test.go`，仿 `redis_audit_entry_test.go:10-50`，检查每个构造 `CheckRequest`/`ExecRequest` 的入口都调了 `MongoAudit`/`FillMongoFacts`——漏一个就有规则静默失效。

## 6. 命令语义单一来源（新增改造）

把命令分类表从两处副本：
- `engine/internal/mongocheck/command.go:21-53`
- `src/lib/mongodb/command.go:26-51`

收敛到 `engine/api/mongo_commands.go`：

```go
var MongoSemantics = map[string]MongoCommandSemantic{ /* name → {scope, dangerous, admin, ...} */ }
func MongoIsReadCommand(name string) bool
func MongoIsWriteCommand(name string) bool
func MongoClassify(name string) int // KindQuery=-1 / KindDDL=0 / KindDML=1
```

两端表格与 `Parse` 各自保留（Mongo 是 JSON token 流 vs bson 驱动，解析不共用，与 Redis 的 `api.SplitRedisCommand` 不同），**只共用语义表**。硬保底的"不读规则集、引擎离线也在"不变——它引用的是编译期共享的常量表，不是运行时规则配置。

加 `engine/internal/archguard/mongo_command_tables_test.go` + `src/lib/mongodb/command_test.go` 护栏，仿 Redis 的 `TestRedisCommandTablesSingleSource`。

## 7. 全链路接线清单

新增一条规则必须同时改到以下 9 处；每处都有护栏，漏了要么编译失败要么测试失败：

| # | 层 | 文件 | 改动 | 护栏 |
| --- | --- | --- | --- | --- |
| 1 | 规则 | `engine/internal/mongocheck/rules_<族>.go` | `Register(commandRule{...})` | `TestRegisteredRules` |
| 2 | 引擎配置 | `engine/internal/mongocheck/config.go` | `Config` 加同名字段 | `TestMongoConfigCoversAllSwitches` |
| 3 | 引擎契约 | `engine/api/api.go` | `AuditRole` 加 `Mongo*` 字段（snake_case） | `TestMongoSwitchesAreImplemented` |
| 4 | 主程序契约 | `src/engine/engine.go` | `AuditRole` 加 `Mongo*` 字段（PascalCase） | 编译期 |
| 5 | 主程序转换 | `src/engine/convert.go` | `AuditRoleToAPI` 加一行赋值 | `TestAuditRoleMongoFieldsAreMapped` |
| 6 | 前端条目 | `front/src/views/manager/rules/rules.ts` | `mongoRule(name, category, tp, level)` 工厂生成 | `TestMongoRulesHaveFrontendEntries` |
| 7 | 前端文案 | `front/src/lang/{zh-cn,en-us}/rule/index.ts` | 规则名 → 说明 | 同上 |
| 8 | 清单 | `engine/SELF_RULES.md` | 重新生成（勿手改） | `TestSelfRulesManifest` |
| 9 | 测试 | `engine/internal/mongocheck/*_test.go` | 命中 / 开关关闭不命中 / 边界不误伤 | `go test ./...` |

**新增护栏**（对齐 Redis 已有而 Mongo 缺的）：
- `TestMongoSecurityRuleCannotBeObserve`：安全类配 observe 必须夹逼成 warn（仿 `TestRedisSecurityRuleCannotBeObserve`）
- `TestMongoFrontendRuleLevels`：前端默认级别必须与 `DefaultLevel()` 一致（仿 `TestRedisFrontendRuleLevels`）
- `TestMongoFactInjectionAtEveryEntry`：所有 `CheckRequest`/`ExecRequest` 入口必须填运行时事实
- `TestMongoCommandTablesSingleSource`：命令语义表只能有一份

## 8. 兼容性与风险

1. **现有 20 条规则名与语义全部保留**，不重命名、不拆分——避免库里 `core_rules.audit_role` JSON 里的既有开关失效。新增规则默认关闭的通过 `RuleLevels` 灰度；新增开关默认 `false` 与现行行为一致。
2. **安全级别夹逼是行为变更**：现行允许安全类配 `observe`（等于放行），改写后降为 `warn`。需在发布说明中写明，避免有人靠 observe 绕过硬安全。
3. **硬保底与引擎规则的刻意重复继续保留**：本次只统一命令语义表，不统一判定；任何新增/修改的安全判定仍须同一个提交同步 `mongocheck` 与 `src/lib/mongodb/{command,query}.go`。
4. **文档漂移顺手修**：`README.md:97` 声称 `renameCollection` 判 `dropTarget:true`（代码无）；`README.md:154-159` 声称执行前 `Check(mode=write)` 复检，`design.md:167-171` 记录已撤销。改写时让 README 与代码一致。
5. **规则数量是目标不是承诺**：Phase 3 的 4 条快照规则若探测成本过高（需要 `explain` 或全集合计数），实现时降级为"配置显式提供快照，不自动探测"，不为了凑数造误报。

## 9. 迁移计划

| 阶段 | 内容 | 验证 |
| --- | --- | --- |
| P0 | 修 README 两处漂移；把命令语义表提到 `engine/api`；加两个 single-source 护栏 | `go test ./internal/archguard/... ./src/lib/mongodb/...` |
| P1 | `commandRule` 表驱动改造；`Level()` 夹逼；模式隔离；20 条现有规则迁移到表驱动（语义不变） | `go test ./internal/mongocheck/...`，对比迁移前后对同一批命令的 findings |
| P2 | 新增 38 条静态规则（4.1–4.7 中除保留的 20 条）+ 全链路接线（第 7 节 9 处）+ 4 个新护栏 + 前端文案 | `UPDATE_SELF_RULES_MANIFEST=1 go test ./internal/archguard -run TestSelfRule`；前端 `vitest run` |
| P3 | 运行时事实注入：`api` 字段、`calls.MongoAudit`、快照探测、`check_mongo.go` 注入 + 入口护栏 | 单测注入快照覆盖 4 条快照规则；集成探针因元数据库不可达时跳过并标注 |
| P4 | 硬保底同步核对；清理旧 `MongoForbidDangerous` 命令表重复；发布说明 | 全量 `go test ./...` + 人工过一遍 SELF_RULES.md |

## 10. 明确不做（YAGNI）

- **不写 Mongo AST**：继续"命令名 + 关键字段 + 操作符形态"判定，不做完整语法树。
- **不做"filter 是否走索引"的规则**：需要 `explain` 与库内元数据，误报率高；快照规则只做文档数/索引名这类廉价事实。
- **不改查询页只读白名单的实现位置**：继续留在主程序 `src/lib/mongodb/query.go` 作 fail-safe。
- **不给 SQL 查询侧加规则**：与本次无关。
- **不引入新依赖**：glob / 版本比较 / 阈值判定全部标准库实现，与 Redis 侧一致。

## 11. 验收标准

- `engine/internal/mongocheck` 规则数 = `engine/SELF_RULES.md` 条数 = `api.AuditRole.Mongo*` 开关数（执行期 `MongoMaxAffectRows` 按 `mongoExecOnly` 白名单例外）。
- 每条规则至少有 3 个单测：命中、开关关闭不命中、边界不误伤。
- 6 道原有 archguard + 4 道新增 archguard 全绿。
- 前端规则页能逐条勾/配级别/配参数，且默认级别与 `DefaultLevel()` 一致。
- 元数据库不可达时，所有依赖运行时事实的规则**静默不生效而非误拦**——这是 P3 的硬验收项。
