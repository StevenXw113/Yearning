# 自研审核规则清单（自动生成，勿手改）

本文件由 `internal/archguard/self_rules_manifest_test.go` 生成并核对：

- 更新：`UPDATE_SELF_RULES_MANIFEST=1 go test ./internal/archguard -run TestSelfRule`
- 核对：`go test ./internal/archguard -run TestSelfRule`（CI 每次都会跑）

自研规则 = 上游 bytebase 没有、本仓库自己实现的规则，分两处存放：

- `internal/customrules`：SQL 侧（上游虽有 SQL 规则，但这几条静态规则上游没有实现）
- `internal/mongocheck`：MongoDB 侧（上游完全没有 Mongo 审核能力，
  `common.EngineSupportSQLReview(MONGODB)` 为 false，advisor 目录里也没有 mongodb 方言）

每条规则的开关都必须在 `api.AuditRole` 里存在：否则规则页配不出来、
`rule_levels` 里的级别也取不到（引擎按字段名查 map）。这条由
`TestSelfRuleSwitchesExistInAuditRole` 核对，反方向由 `TestMongoSwitchesAreImplemented` 兜住。

上游规则清单见同目录的 `RULES.md`（那份管「上游新增的有没有被漏掉」，与本文件互补）。

## SQL 侧 · internal/customrules（2 条）

- `forbid-drop` — 开关：`DDLEnableDropDatabase`、`DDLEnableDropTable`；禁止 DROP DATABASE / DROP TABLE（受「允许删除数据库/表」两个开关控制）
- `forbid-truncate` — 开关：`DDLForbidTruncate`；禁止 TRUNCATE（不可回滚，如需清空数据请改用 DELETE 并走审批）

## MongoDB 侧 · internal/mongocheck（20 条）

- `MongoCollectionPrefix` — 开关：`MongoCollectionPrefix`；集合名需以参数为前缀（空 = 不生效）
- `MongoForbidAdminCommand` — 开关：`MongoForbidAdminCommand`；禁止 createUser / dropUser / grantRolesToUser / createRole 等账号与权限管理命令
- `MongoForbidCappedConvert` — 开关：`MongoForbidCappedConvert`；禁止 convertToCapped（会重建集合，不可逆）
- `MongoForbidCollMod` — 开关：`MongoForbidCollMod`；禁止 collMod（修改 validator / TTL / 索引可见性等集合元数据）
- `MongoForbidDangerous` — 开关：`MongoForbidDangerous`；禁止 dropDatabase / eval / mapReduce 等危险命令
- `MongoForbidDropCollection` — 开关：`MongoForbidDropCollection`；禁止 drop / renameCollection 集合
- `MongoForbidEmptyFilter` — 开关：`MongoForbidEmptyFilter`；禁止无 filter 的 update / delete / findAndModify（否则作用于整个集合）
- `MongoForbidImmutableId` — 开关：`MongoForbidImmutableId`；禁止修改 _id（不可变字段：$set / $unset / $rename / 替换文档）
- `MongoForbidSystemCollection` — 开关：`MongoForbidSystemCollection`；禁止对 system.* 集合（system.users / system.profile / system.js / system.views）做变更
- `MongoForbidWhere` — 开关：`MongoForbidWhere`；禁止 $where（把脚本推到服务端执行）
- `MongoIndexKeyLimit` — 开关：`MongoIndexKeyLimit`；createIndexes 单索引键数不超过参数上限（0 = 不限）
- `MongoIndexNameSpec` — 开关：`MongoIndexNameSpec`；索引名需匹配参数里的正则（空 = 不生效）
- `MongoLargeInList` — 开关：`MongoLargeInList`；$in 元素个数不超过参数上限（0 = 不限）
- `MongoMaxCollectionNameLen` — 开关：`MongoMaxCollectionNameLen`；集合名长度不超过参数上限（0 = 不限）
- `MongoNegationOperator` — 开关：`MongoNegationOperator`；禁止 $ne / $nin / $not / $nor（选择性差，通常全集合扫描）
- `MongoOrClause` — 开关：`MongoOrClause`；禁止 $or（各分支需分别走索引，容易退化为全集合扫描）
- `MongoQueryForbidLookup` — 开关：`MongoQueryForbidLookup`；查询禁用 $lookup / $graphLookup（关联查询代价高，且难以走索引）
- `MongoQueryForbidNoFilter` — 开关：`MongoQueryForbidNoFilter`；查询条件不能为空：find 需要 filter，aggregate 需要 $match（避免全集合扫描）
- `MongoQueryForbidNoLimit` — 开关：`MongoQueryForbidNoLimit`；find 带 sort 时必须带 limit（否则服务端要对全部命中排序）
- `MongoRegexUnanchored` — 开关：`MongoRegexUnanchored`；禁止未以 ^ 锚定的 $regex（无法使用索引，退化为全集合扫描）

## Redis 侧 · internal/redischeck（73 条）

- `RedisBigKeySyncDelete` — 开关：`RedisBigKeySyncDelete`；同步删除大 key 会阻塞服务端，建议改用 UNLINK
- `RedisClusterCrossSlot` — 开关：`RedisClusterCrossSlot`；Cluster 中多键命令必须落在同一 slot，否则拦截
- `RedisClusterDbZero` — 开关：`RedisClusterDbZero`；原生 Cluster 仅允许 DB 0；选择其它 DB 时拦截
- `RedisClusterScanScope` — 开关：`RedisClusterScanScope`；Cluster 下 SCAN 只覆盖单个节点，不能当作全局遍历
- `RedisClusterScriptSlot` — 开关：`RedisClusterScriptSlot`；Cluster 脚本/函数必须显式声明同 slot 的键；未验证时拦截
- `RedisClusterTransactionSlot` — 开关：`RedisClusterTransactionSlot`；Cluster 事务要求所有键同一 slot；单命令入口无法保证，拦截
- `RedisCommandCompatibility` — 开关：`RedisCommandCompatibility`；命令需要更高的 Redis 版本，当前实例不支持
- `RedisCommandSyntax` — 开关：`RedisCommandSyntax`；检查命令、子命令、参数数量及选项组合；语法非法或参数不完整时拦截
- `RedisCopyOverwrite` — 开关：`RedisCopyOverwrite`；COPY 使用 REPLACE 等覆盖行为时提示，并检查源键、目标键及目标 DB 权限
- `RedisDbScope` — 开关：`RedisDbScope`；命令只能访问指定 DB 范围；超出授权范围时拦截；格式如 0-15
- `RedisDeleteKey` — 开关：`RedisDeleteKey`；执行 DEL、UNLINK 时提示数据删除风险
- `RedisExpensiveRead` — 开关：`RedisExpensiveRead`；SORT、集合交并差及其他复杂读取，输入规模未知或超过预算时提示
- `RedisForbidAclChange` — 开关：`RedisForbidAclChange`；禁止修改用户及权限的 ACL 子命令，如 SETUSER、DELUSER、LOAD
- `RedisForbidAdmin` — 开关：`RedisForbidAdmin`；禁止修改实例配置或连接状态的管理子命令，如 CONFIG SET/REWRITE/RESETSTAT、CLIENT KILL/PAUSE；只读子命令单独审核
- `RedisForbidBlockingCommand` — 开关：`RedisForbidBlockingCommand`；普通命令入口禁止列表阻塞、Stream 阻塞等等待型操作；专项场景另行配置
- `RedisForbidClusterChange` — 开关：`RedisForbidClusterChange`；禁止修改集群拓扑和 Slot 状态的 CLUSTER 子命令；只读查询子命令单独审核
- `RedisForbidConnectionControl` — 开关：`RedisForbidConnectionControl`；禁止提交 AUTH、HELLO 等认证或连接状态控制命令，连接状态由审核入口统一管理
- `RedisForbidDangerous` — 开关：`RedisForbidDangerous`；禁止 FLUSHALL、FLUSHDB、SHUTDOWN、SWAPDB 等清库、关闭实例或交换数据库的命令
- `RedisForbidDebug` — 开关：`RedisForbidDebug`；禁止 DEBUG 命令
- `RedisForbidKeysCommand` — 开关：`RedisForbidKeysCommand`；禁止 KEYS，包括带业务前缀的 KEYS；建议改用受限 SCAN
- `RedisForbidMigration` — 开关：`RedisForbidMigration`；禁止 MIGRATE、RESTORE 等迁移或恢复操作，转专项审核
- `RedisForbidModuleChange` — 开关：`RedisForbidModuleChange`；禁止 MODULE LOAD、LOADEX、UNLOAD 等模块变更命令
- `RedisForbidMonitor` — 开关：`RedisForbidMonitor`；禁止 MONITOR，避免持续监听带来的性能及数据暴露风险
- `RedisForbidMove` — 开关：`RedisForbidMove`；禁止 MOVE，避免跨 DB 迁移数据；SELECT 另按 DB 权限及部署模式检查
- `RedisForbidPersist` — 开关：`RedisForbidPersist`；禁止 PERSIST，避免将有过期时间的缓存转为永久保存；允许永久化的业务可单独豁免
- `RedisForbidPersistence` — 开关：`RedisForbidPersistence`；禁止普通业务入口执行 SAVE、BGSAVE、BGREWRITEAOF 等持久化管理命令
- `RedisForbidRandomKey` — 开关：`RedisForbidRandomKey`；禁止 RANDOMKEY，避免读取范围超出业务键授权范围
- `RedisForbidRename` — 开关：`RedisForbidRename`；禁止 RENAME、RENAMENX；RENAME 可能覆盖目标键，RENAMENX 虽不覆盖但仍会改变键名与业务归属
- `RedisForbidReplica` — 开关：`RedisForbidReplica`；禁止 REPLICAOF、SLAVEOF，避免改变复制关系
- `RedisForbidScript` — 开关：`RedisForbidScript`；禁止未登记的 EVAL、EVALSHA、EVAL_RO、EVALSHA_RO、FCALL、FCALL_RO 等脚本或函数执行命令
- `RedisForbidScriptManagement` — 开关：`RedisForbidScriptManagement`；禁止 SCRIPT 及 FUNCTION 中修改、加载、清理脚本或函数状态的子命令；只读子命令单独审核
- `RedisForbidSubscription` — 开关：`RedisForbidSubscription`；禁止 SUBSCRIBE、PSUBSCRIBE、SSUBSCRIBE 等持续订阅操作
- `RedisForbidWatch` — 开关：`RedisForbidWatch`；普通命令入口禁止 WATCH、UNWATCH；仅在明确支持完整会话语义时开放
- `RedisForbiddenKeyPattern` — 开关：`RedisForbiddenKeyPattern`；禁止访问匹配保护规则的键，如系统键、核心锁及内部状态键；多个规则用逗号分隔
- `RedisFullCollectionRead` — 开关：`RedisFullCollectionRead`；HGETALL、SMEMBERS、全量 LRANGE/ZRANGE 等操作，在集合规模未知或超过阈值时提示
- `RedisImmediateExpire` — 开关：`RedisImmediateExpire`；指定立即过期或已过去的过期时间时，按删除操作处理并提示
- `RedisKeyExtraction` — 开关：`RedisKeyExtraction`；必须完整识别命令访问的源键和目标键；无法识别时拦截
- `RedisKeyPrefix` — 开关：`RedisKeyPrefix`；键名必须位于授权前缀范围内，所有源键和目标键均需检查；留空表示不检查前缀
- `RedisLargeOffsetWrite` — 开关：`RedisLargeOffsetWrite`；SETRANGE、SETBIT 等按偏移写入的操作，预计对象最终大小超过阈值时拦截，0 表示不限制
- `RedisLargeRangeDelete` — 开关：`RedisLargeRangeDelete`；集合范围删除、列表裁剪及 Stream 裁剪等操作，影响元素数量未知或超过阈值时提示
- `RedisLargeValueRead` — 开关：`RedisLargeValueRead`；读取超大 key（> 1 MB）会占用查询连接与带宽
- `RedisMaxCommandPayload` — 开关：`RedisMaxCommandPayload`；单条命令参数总字节数上限，0 表示不限制；防止多个小值绕过单值大小限制
- `RedisMaxDeleteKeys` — 开关：`RedisMaxDeleteKeys`；单条删除命令键数上限，0 表示不限制；超过上限转批量删除审核
- `RedisMaxKeyLen` — 开关：`RedisMaxKeyLen`；键名长度上限（字节），0 表示不限制
- `RedisMaxReadElements` — 开关：`RedisMaxReadElements`；单次范围查询或随机读取请求的元素数上限，0 表示不限制；无法确定上限时提示
- `RedisMaxReadKeys` — 开关：`RedisMaxReadKeys`；MGET 等单条批量读取命令的键数上限，0 表示不限制
- `RedisMaxScanCount` — 开关：`RedisMaxScanCount`；SCAN、HSCAN、SSCAN、ZSCAN 的 COUNT 建议不超过配置值；COUNT 仅是工作量提示，不代表结果条数上限
- `RedisMaxTtl` — 开关：`RedisMaxTtl`；过期时间超过业务最大值时提示；最大值按业务配置，0 表示不检查
- `RedisMaxValueSize` — 开关：`RedisMaxValueSize`；单个新写入值的字节数上限，0 表示不限制；超过上限拦截
- `RedisMaxWriteElements` — 开关：`RedisMaxWriteElements`；单条命令新增或修改的字段、成员、列表元素数上限，0 表示不限制
- `RedisMaxWriteKeys` — 开关：`RedisMaxWriteKeys`；单条写入命令涉及的键数上限，0 表示不限制；超过上限转批量审核
- `RedisMinTtl` — 开关：`RedisMinTtl`；过期时间低于业务最小值时提示；最小值按业务配置，0 表示不检查
- `RedisNonIdempotentWrite` — 开关：`RedisNonIdempotentWrite`；INCR、DECR、计数增量、列表弹出等重复执行会改变结果的操作，提示重复执行风险
- `RedisOverwriteExisting` — 开关：`RedisOverwriteExisting`；覆盖写入已存在的键，原值会丢失
- `RedisProductionWrite` — 开关：`RedisProductionWrite`；生产数据源上的写命令，需要确认影响面与回滚方式
- `RedisProtectedKeyDelete` — 开关：`RedisProtectedKeyDelete`；禁止删除匹配保护规则的锁、幂等键、核心状态及其他关键业务键；多个规则用逗号分隔
- `RedisProxyAtomicity` — 开关：`RedisProxyAtomicity`；源→目的类命令跨分片无法保证原子性（SMOVE/RENAME/RPOPLPUSH/BITOP/MSETNX 等要求同 slot）
- `RedisProxyCommandSupport` — 开关：`RedisProxyCommandSupport`；Proxy 集群未实现或受限的命令（如 Redis 4.0 代理不支持 BZPOPMAX/BZPOPMIN）
- `RedisProxyCrossShard` — 开关：`RedisProxyCrossShard`；多键命令的键跨 slot 时拦截（代理按 slot 路由，跨分片命令无法保证结果）
- `RedisProxyDbSupport` — 开关：`RedisProxyDbSupport`；Proxy 未验证支持非 db0 逻辑库时拦截
- `RedisProxyScanScope` — 开关：`RedisProxyScanScope`；Proxy 下扫描覆盖范围未知，不能作为全局扫描或批量变更依据
- `RedisProxyScriptSupport` — 开关：`RedisProxyScriptSupport`；Proxy 集群要求脚本显式声明 key（numkeys ≥ 1）且所有 key 同 slot
- `RedisProxyTransactionSupport` — 开关：`RedisProxyTransactionSupport`；Proxy 集群只支持单分片内事务，MULTI/EXEC 与 WATCH 跨分片无法保证
- `RedisReadConsistency` — 开关：`RedisReadConsistency`；分片架构下多键读不是同一时刻快照，可能一部分读到旧值
- `RedisRequireTtl` — 开关：`RedisRequireTtl`；缓存类键新建后必须有过期时间；明确登记为永久数据的业务键可豁免
- `RedisScanMatchRequired` — 开关：`RedisScanMatchRequired`；SCAN 必须指定允许的业务匹配范围；MATCH 规则无法确认属于授权范围时拦截
- `RedisSensitiveKeyAccess` — 开关：`RedisSensitiveKeyAccess`；访问凭据、Token、个人信息等敏感业务键时提示；敏感范围由业务配置
- `RedisStoreOperation` — 开关：`RedisStoreOperation`；集合计算或排序结果写入目标键时，检查输入规模、目标覆盖及所有源键和目标键权限
- `RedisStreamStateChange` — 开关：`RedisStreamStateChange`；消费组变更、消息确认、认领等操作会改变消费状态，执行时提示
- `RedisTransactionIntegrity` — 开关：`RedisTransactionIntegrity`；MULTI/EXEC 必须组成完整审核单元；单命令入口无法保证，直接拦截
- `RedisTtlLossOnOverwrite` — 开关：`RedisTtlLossOnOverwrite`；覆盖写入可能清除原 TTL，且未明确指定过期策略或保留 TTL 时提示
- `RedisTypeMismatch` — 开关：`RedisTypeMismatch`；写命令的键类型与命令期望不符，执行会直接报 WRONGTYPE
- `RedisUnknownCommand` — 开关：`RedisUnknownCommand`；禁止尚未登记语义的命令、模块命令及厂商扩展命令

## Doris 侧 · internal/dorischeck（4 条）

- `DorisBucketLimit` — 开关：`DorisBucketLimit`；建表 BUCKETS 数量超过上限（0 = 不限制）
- `DorisKeyModel` — 开关：`DorisKeyModel`；提示 AGGREGATE KEY 聚合模型（写入有约束，聚合类型选错会导致结果错误）
- `DorisReplicationNum` — 开关：`DorisReplicationNum`；建表副本数与期望值不一致（0 = 不校验）
- `DorisRequireDistribution` — 开关：`DorisRequireDistribution`；建表必须指定 DISTRIBUTED BY（否则默认随机分桶，不可控）

## 执行期限制 · 非静态审核（1 条）

- `MongoMaxAffectRows` — 开关：`MongoMaxAffectRows`；单次 update / delete 允许命中的文档数上限（0 = 不限），由主程序抓取前镜像时判定
