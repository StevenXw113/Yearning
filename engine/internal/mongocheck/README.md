# internal/mongocheck

MongoDB 审核规则。上游 bytebase **完全没有** Mongo 审核能力
（`common.EngineSupportSQLReview(MONGODB)` 为 false，`plugin/advisor/` 下没有 mongodb 方言），
本包是自研的，与 `internal/customrules`（SQL 侧自研规则）并列。

`internal/bytebase` 是上游的内化副本（`scripts/sync-bytebase.sh` 整体重建），本包**不受同步影响**。

## 设计目标

让 MongoDB 成为与 MySQL 对等的「数据引擎」：同一份规则集（`core_rules.audit_role`）、
同一套级别（error / warn / observe）、同一个规则页，规则可注册、可分级、可配参。
差别只在于**审核输入**：MySQL 是语法树，Mongo 是一段 extended JSON 命令。

## 审核分层

| 层 | 位置 | 读规则集 | 作用 |
| --- | --- | --- | --- |
| 静态规则 | `engine/internal/mongocheck` | 是 | 查询、申请页检测和工单执行前复检。开关和级别由规则集控制。 |
| 硬校验 | `engine/mongoguard`、主程序及引擎的 `mongodb.Validate` | 否 | 单条 Extended JSON 命令；查询只读白名单；改删必须带非空文档条件；禁止删库和服务端脚本。 |
| 执行期限制 | `engine/internal/mongodb/exec.go` | 是（仅上限） | 写入前抓前镜像，并对快照文档数执行上限检查。 |

应用和引擎共用 `mongoguard.Parse`，保留 BSON 字段顺序和类型，拒绝重复文档字段和尾随内容。
查询审核和查询执行共用 `mongoguard.ValidateQuery`。变更执行的硬校验仍独立于可配级别。

## 审核模式（Mode）

Mongo 命令分两类入口，引擎必须知道自己在审哪一种：

| Mode | 入口 | 输入 | 写命令 |
| --- | --- | --- | --- |
| `write`（默认） | 申请页检测 `PUT /api/v2/fetch/test`、工单执行前复检 | 变更命令 | 正常审核 |
| `query` | 查询页 WebSocket（`SocketQueryResults` 的 Mongo 分支） | 只读命令 | **直接拒绝**（非规则，硬拦） |

`CheckRequest.mode` 为空时按 `write` 处理，保证旧调用方行为不变。
不靠「命令名自动推断模式」：申请页里填 `find` 必须报「不是变更命令」，
而不是悄悄按查询规则放行。

## 规则注册表

```go
type Rule interface {
	Name() string            // 规则名 = AuditRole 字段名 = RuleLevels 的 key（严格 1:1）
	Desc() string            // 规则页/清单里的一句说明
	Category() Category      // security / structure / performance
	Modes() []Mode           // 适用模式（write / query 可同时适用）
	DefaultLevel() Level     // error / warn / observe（未在规则集里配级别时用它）
	Check(cfg Config, cmd *Command) []Finding
}
```

- 规则按族分文件（`rules_<族>.go`，如安全类、结构类、性能/查询类），每条规则在 `init()` 里
  `Register(...)`：重名、名称为空、级别非法、安全类默认不拦截，都直接 panic
  （与 `customrules.Register` 一致，让配置错误立刻暴露）。
- `Command` 是共享解析层：命令名（BSON 首键）、目标集合、分类、子句数组
  （`updates[]` / `deletes[]`）、`EmptyFilter()`、`Has()`（按键名递归）、`Walk()`（按值递归，
  给 `$regex` / `$in` 这类要取值判定的规则用）。规则只读它，不各自解析。
- `All()` 返回注册表里的全部规则，供 `SELF_RULES.md` 生成与 archguard 核对。

### 配置读取（反射只用在边界上）

规则集字段多而密，逐字段映射（`mongoRulesFrom` 里一行一个）在十几条规则时必然漏。
拆成两半：

- `Config` 的字段保持**显式类型**，规则直接读 `cfg.ForbidEmptyFilter` / `cfg.IndexKeyLimit`——
  拼错名字编译期就报错，IDE 里能跳转；这层不需要反射。
- **proto → Config 这一步才用反射**（`ConfigFromProto`）：按名字对应关系自动填
  （API JSON 的 `mongo_` 前缀在 Config 里省略，见 `protoFieldKey`），
  于是 `server/rules.go` 的 `mongoRulesFrom` 只剩一行，不必为每条规则补映射。

```go
cfg.ForbidEmptyFilter            // 开关（bool）
cfg.IndexKeyLimit                // 数字参数（int）
cfg.IndexNameSpec                // 字符串参数（string）
cfg.Level(rule)                  // 级别：RuleLevels[Name] 优先，非法/缺省取 DefaultLevel()
```

「proto 有而 Config 没有」由 `TestMongoConfigCoversAllSwitches`（见护栏）挡下；
「Config 有而 proto 没有」在编译期就过不去（规则读的字段必须存在）。

### 默认级别

- `security` 类 **必须** `error`（护栏强制），不允许通过默认值把硬安全降级；
- `structure` / `performance` 类默认 `warn`（命名规范、索引形态这类误报率较高的）；
- 规则集里显式配了 `RuleLevels[Name]` 则以配置为准（含降级为 observe 灰度）。

## 规则全集

`write` 模式（变更命令）：

| 规则名（= 开关名） | 类 | 默认 | 判定 |
| --- | --- | --- | --- |
| `MongoForbidEmptyFilter` | security | error | `updates[].q` / `deletes[].q` / 顶层 `q`\|`query` 为空 |
| `MongoForbidDangerous` | security | error | `dropDatabase` / `eval` / `mapReduce` |
| `MongoForbidWhere` | security | error | `$where` / `$function` / `$accumulator`（服务端脚本） |
| `MongoForbidDropCollection` | security | error | `drop` / `renameCollection`（含 `dropTarget:true`） |
| `MongoForbidSystemCollection` | security | error | 目标集合 ∈ `system.*`（`system.users` / `system.profile` / `system.js` / `system.views`） |
| `MongoForbidAdminCommand` | security | error | `createUser` / `dropUser` / `grantRolesToUser` / `createRole` … 归 DDL 并拦 |
| `MongoForbidImmutableId` | security | error | `$set` / `$unset` / `$rename` 触及 `_id`（`$setOnInsert` 除外） |
| `MongoForbidUpsert` | security | error | `updates[].upsert` 或 `findAndModify.upsert` 为 `true` |
| `MongoForbidMultiWrite` | security | error | `updates[].multi: true` 或 `deletes[].limit: 0`，按子句判定 |
| `MongoForbidCappedConvert` | structure | error | `convertToCapped`（重建集合，不可逆） |
| `MongoForbidCollMod` | structure | warn | `collMod`（validator / TTL / 索引可见性） |
| `MongoIndexKeyLimit` | structure | warn | `createIndexes` 单索引键数 > 参数（0 = 不限） |
| `MongoIndexNameSpec` | structure | warn | 索引名不匹配参数正则（空 = 规则不生效） |
| `MongoCollectionPrefix` | structure | warn | 集合名不以参数为前缀（空 = 规则不生效） |
| `MongoMaxCollectionNameLen` | structure | warn | 集合名长度 > 参数（0 = 不限） |
| `MongoRegexUnanchored` | performance | warn | filter 里 `$regex` 未以 `^` 锚定 |
| `MongoNegationOperator` | performance | warn | `$ne` / `$nin` / `$not` / `$nor` |
| `MongoOrClause` | performance | warn | 顶层 `$or` |
| `MongoLargeInList` | performance | warn | `$in` 元素数 > 参数（0 = 不限） |

`query` 模式（只读命令）：

| 规则名 | 类 | 默认 | 判定 |
| --- | --- | --- | --- |
| `MongoQueryForbidNoFilter` | performance | warn | `find` 缺少非空 `filter`，或 `aggregate` 首阶段缺少非空 `$match` |
| `MongoQueryForbidLookup` | performance | warn | `aggregate` 顶层 `$lookup` / `$graphLookup` |
| `MongoQueryForbidNoLimit` | performance | warn | `find` 带非空 `sort`，但未提供正整数 `limit` |

上表 4 条 performance 规则同时适用于两种模式（`Modes()` 返回两个）。

执行期（非静态审核）：

| 规则名 | 默认 | 判定 |
| --- | --- | --- |
| `MongoMaxAffectRows` | error | 引擎执行 `update` / `delete` 前检查前镜像文档数。`0` 跳过规则集上限，前镜像仍受 `MaxSnapshot` 限制。 |

`MongoMaxAffectRows` 不在静态规则表里（静态审核拿不到命中数），
登记在 `internal/archguard` 的 `mongoExecOnly` 白名单。

## 查询侧审核链路

查询页校验权限和查询申请后调用引擎 `Check(mode=query)`。拦截项会阻止执行并进入查询记录，提示和观察项随成功查询保存。
引擎不可用时，应用执行同一份只读硬校验，并在成功查询的记录中标记未完成规则审核。
只读白名单拒绝未知命令、`aggregate` 顶层 `$out` / `$merge`，以及 `$where` / `$function` / `$accumulator`。
`explain` 的内层必须是白名单内的只读命令，查询规则检查内层命令。

## 生产规则集启用

`MongoForbidUpsert` 与 `MongoForbidMultiWrite` 在已有规则集里默认为关闭。开启后默认级别为 `error`。
可以先设为 `warn` 收集命中情况，再按数据源切换为 `error`。新的查询硬校验随应用和引擎部署生效。
多文档规则按子句限制，批次仍可包含多个单文档子句。整批写入范围需要结合同一规则集的 `MongoMaxAffectRows` 评估。

## 执行期复检

MongoDB 工单连接目标库前，会按执行请求携带的规则集以 `write` 模式复检。命中 `error` 则终止，提示随执行明细记录。
`MongoMaxAffectRows` 统计执行前读到的文档。并发写入可以改变后续命中范围，静态规则与前镜像计数都无法给出事务级写入上限。

## 护栏（internal/archguard）

| 测试 | 挡什么 |
| --- | --- |
| `TestSelfRulesManifest`（已有） | 规则改了没更新 `engine/SELF_RULES.md` |
| `TestSelfRuleSwitchesExistInAuditRole`（已有） | 开关名与 API 字段名不一致（页面配不出来、级别无处安放） |
| `TestMongoSwitchesAreImplemented`（已有） | API 有开关但引擎没实现 |
| `TestMongoConfigCoversAllSwitches`（新） | API 有开关但 `Config` 取不到（加了规则忘接线 → 勾了不生效） |
| `TestMongoRulesHaveFrontendEntries`（新） | 规则没进 `front/src/views/manager/rules/rules.ts`，或 zh-cn/en-us 少文案 |
| `TestMongoRuleDefaultLevels`（新） | 级别取值非法；或 `security` 类默认级别被写成非 error |
| `TestAuditRoleSelfRuleFieldsAreMapped`（新，主程序侧 `src/engine/convert_test.go`） | `api.AuditRole` 的 Mongo 字段都能被 `AuditRoleToAPI` 填上（漏接线） |

## 新增一条规则

1. `internal/mongocheck/<规则>.go`（新文件）：实现 `Rule`，`init()` 里 `Register`；
   同时按规则声明 `Category` / `Modes` / `DefaultLevel`，规则名常量写在文件顶部。
2. `internal/mongocheck/config.go`：`Config` 加同名字段（API JSON 的 `mongo_` 前缀在 Config 里省略）。
3. `engine/api/api.go` 的 `AuditRole` 加字段，JSON 键使用 `mongo_` 前缀和蛇形命名。
4. `src/engine/engine.go` 的 `AuditRole` 保存前端规则集字段，`src/engine/convert.go` 的 `AuditRoleToAPI` 将字段传给引擎。
5. `front/src/views/manager/rules/rules.ts` 加一行（`tp:0` 开关 / `1` 数字 / `2` 文本）
   + `front/src/lang/{zh-cn,en-us}/rule/index.ts` 加文案。
6. `UPDATE_SELF_RULES_MANIFEST=1 go test ./internal/archguard -run TestSelfRule` 重生成清单。
7. 补测试（命中 / 开关关闭不命中 / 边界不误伤），`go test ./...`。

第 2、3、4、5 步都是「漏了不报错」的类型，所以每条都有护栏：
`TestMongoConfigCoversAllSwitches`（步骤 2）、`TestSelfRuleSwitchesExistInAuditRole`（步骤 3）、
`TestAuditRoleSelfRuleFieldsAreMapped`（步骤 4）、`TestMongoRulesHaveFrontendEntries`（步骤 5）。

## 明确不做（YAGNI）

- **不写 Mongo 命令解析器**：不做 AST，规则只做「命令名 + 关键字段 + 操作符形态」判定。
- **不接入 schema 元数据**：引擎目前不提供 `FinalMetadata` 快照，
  「filter 是否走索引」这类需要库内信息的规则一律不做——宁可少一条规则，不造误报。
- **不改 `core_sql_records` 表**：变更侧审核结论已由执行明细承载（`Error` 列）。
- **不给 SQL 查询侧加规则审核**：那是另一批工作，不与本次混在一起。

## 已知取舍

- 性能类规则只能看「操作符形态」（`$regex` 未锚定、`$ne`/`$nin`、`$or`），
  无法判断真实执行计划；因此默认 `warn`，靠规则集灰度后再由各环境决定是否收紧。
- 硬保底与引擎规则存在**刻意的语义重复**（空 filter、`$where`、危险命令各两份实现），
  两边都要改时容易只改一份：`mongocheck` 与 `src/lib/mongodb/command.go` 的判定语义
  必须在同一个提交里同步。
