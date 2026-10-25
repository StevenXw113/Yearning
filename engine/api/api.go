// Package api 是主服务(Yearning)与审核引擎(engine)之间的线缆契约。
//
// 历史说明:契约此前以 proto3 定义、经 gRPC 传输;现已改为 HTTP+JSON 传输,
// 本包即契约的唯一权威定义(手写 struct,字段名与原 proto 完全一致)。
// 两条铁律:
//  1. 字段只增不改不删(JSON 缺省零值,旧客户端向后兼容);
//  2. 两端必须同仓库同版本发布(跨版本协议不兼容,与 gRPC 时代语义一致)。
package api

// DataSource 数据源连接信息。密码仅用于建立到目标库的连接,绝不落日志。
type DataSource struct {
	Ip       string `json:"ip"`
	Port     int32  `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Ca       string `json:"ca"`
	Cert     string `json:"cert"`
	Key      string `json:"key"`
	Kind     string `json:"kind"` // "mysql" / "mongodb" / "redis" / "postgres" / "doris"
	// AuthDb MongoDB 认证库(连接串 authSource)。空 = 默认 admin;
	// 账号建在业务库上的部署(authSource=业务库名)必须显式指定,否则连不上。
	AuthDb string `json:"auth_db"`
}

// AuditRole 审核规则,与 Yearning engine.AuditRole 对齐。
// 字段按方言分段增长(MySQL 上游 bytebase 规则 + Mongo/Redis/Doris 自研规则);
// rule_levels 为每条规则的级别覆盖(key=字段名,value=error/warn/observe,缺省 error)。
type AuditRole struct {
	DmlTransaction                 bool   `json:"dml_transaction"`
	DmlAllowLimitStmt              bool   `json:"dml_allow_limit_stmt"`
	DmlInsertColumns               bool   `json:"dml_insert_columns"`
	DmlMaxInsertRows               int32  `json:"dml_max_insert_rows"`
	DmlWhere                       bool   `json:"dml_where"`
	DmlWhereExprValueIsNull        bool   `json:"dml_where_expr_value_is_null"`
	DmlOrder                       bool   `json:"dml_order"`
	DmlSelect                      bool   `json:"dml_select"`
	DmlAllowInsertNull             bool   `json:"dml_allow_insert_null"`
	DmlInsertMustExplicitly        bool   `json:"dml_insert_must_explicitly"`
	DdlEnablePrimaryKey            bool   `json:"ddl_enable_primary_key"`
	DdlCheckTableComment           bool   `json:"ddl_check_table_comment"`
	DdlCheckColumnComment          bool   `json:"ddl_check_column_comment"`
	DdlCheckColumnNullable         bool   `json:"ddl_check_column_nullable"`
	DdlCheckColumnDefault          bool   `json:"ddl_check_column_default"`
	DdlEnableAcrossDbRename        bool   `json:"ddl_enable_across_db_rename"`
	DdlEnableAutoincrementInit     bool   `json:"ddl_enable_autoincrement_init"`
	DdlEnableAutoIncrement         bool   `json:"ddl_enable_auto_increment"`
	DdlEnableAutoincrementUnsigned bool   `json:"ddl_enable_autoincrement_unsigned"`
	DdlEnableDropTable             bool   `json:"ddl_enable_drop_table"`
	DdlEnableDropDatabase          bool   `json:"ddl_enable_drop_database"`
	DdlEnableNullIndexName         bool   `json:"ddl_enable_null_index_name"`
	DdlIndexNameSpec               bool   `json:"ddl_index_name_spec"`
	DdlMaxKeyParts                 uint32 `json:"ddl_max_key_parts"`
	DdlMaxKey                      uint32 `json:"ddl_max_key"`
	DdlMaxCharLength               uint32 `json:"ddl_max_char_length"`
	MaxTableNameLen                int32  `json:"max_table_name_len"`
	MaxAffectRows                  uint32 `json:"max_affect_rows"`
	MaxDdlAffectRows               uint32 `json:"max_ddl_affect_rows"`
	SupportCharset                 string `json:"support_charset"`
	SupportCollation               string `json:"support_collation"`
	CheckIdentifier                bool   `json:"check_identifier"`
	MustHaveColumns                string `json:"must_have_columns"`
	DdlMultiToCommit               bool   `json:"ddl_multi_to_commit"`
	DdlPrimaryKeyMust              bool   `json:"ddl_primary_key_must"`
	DdlAllowColumnType             bool   `json:"ddl_allow_column_type"`
	DdlImplicitTypeConversion      bool   `json:"ddl_implicit_type_conversion"`
	DdlAllowPriNotInt              bool   `json:"ddl_allow_pri_not_int"`
	DdlAllowMultiAlter             bool   `json:"ddl_allow_multi_alter"`
	DdlEnableForeignKey            bool   `json:"ddl_enable_foreign_key"`
	DdlTablePrefix                 string `json:"ddl_table_prefix"`
	DdlColumnsMustHaveIndex        string `json:"ddl_columns_must_have_index"`
	DdlAllowChangeColumnPosition   bool   `json:"ddl_allow_change_column_position"`
	DdlCheckFloatDouble            bool   `json:"ddl_check_float_double"`
	IsOsc                          bool   `json:"is_osc"`
	OscExpr                        string `json:"osc_expr"`
	OscSize                        uint32 `json:"osc_size"`
	AllowCreateView                bool   `json:"allow_create_view"`
	AllowCreateViewWithSelectStar  bool   `json:"allow_create_view_with_select_star"`
	AllowCreatePartition           bool   `json:"allow_create_partition"`
	AllowSpecialType               bool   `json:"allow_special_type"`
	PriRollBack                    bool   `json:"pri_roll_back"`
	// RuleLevels 规则级别:key 为规则集里的字段名(与前端规则开关同名),
	// value 取 error/warn/observe。缺省 error(命中即拦截);warn 提示不拦、
	// observe 只记录不拦,用于新规则灰度上线。
	RuleLevels map[string]string `json:"rule_levels"`
	// DdlForbidTruncate 自研规则开关(internal/customrules):true = 禁止 TRUNCATE。
	DdlForbidTruncate bool `json:"ddl_forbid_truncate"`

	// ------- MongoDB 审核规则(internal/mongocheck,自研;上游 bytebase 无此方言)-------
	// 语义与 SQL 侧一致:开关可关,级别走 RuleLevels(key=字段名)。
	MongoForbidEmptyFilter    bool `json:"mongo_forbid_empty_filter"`
	MongoForbidDangerous      bool `json:"mongo_forbid_dangerous"`
	MongoForbidWhere          bool `json:"mongo_forbid_where"`
	MongoForbidDropCollection bool `json:"mongo_forbid_drop_collection"`
	// MongoMaxAffectRows 单次 update/delete 允许命中的文档数上限,0 = 不限制。
	// 注意:这是**执行期**限制(静态审核拿不到命中数),引擎侧只透传配置,
	// 真正的判定在主程序抓取前镜像时进行。
	MongoMaxAffectRows          int32 `json:"mongo_max_affect_rows"`
	MongoForbidSystemCollection bool  `json:"mongo_forbid_system_collection"`
	MongoForbidAdminCommand     bool  `json:"mongo_forbid_admin_command"`
	// MongoForbidImmutableId 禁止修改 _id(不可变字段:$set/$unset/$rename/替换文档)。
	MongoForbidImmutableId bool `json:"mongo_forbid_immutable_id"`

	// ------- MongoDB 结构类规则(默认 warn,避免误报影响提交)-------
	// MongoForbidCappedConvert:convertToCapped 会重建集合(不可逆),默认拦截。
	MongoForbidCappedConvert bool `json:"mongo_forbid_capped_convert"`
	MongoForbidCollMod       bool `json:"mongo_forbid_coll_mod"`
	// MongoIndexKeyLimit 单索引键数上限,0 = 不限制(Mongo 自身上限 32)。
	MongoIndexKeyLimit int32 `json:"mongo_index_key_limit"`
	// MongoIndexNameSpec 索引命名规范(正则,空 = 规则不生效)。
	MongoIndexNameSpec string `json:"mongo_index_name_spec"`
	// MongoCollectionPrefix 集合名前缀规范(空 = 规则不生效)。
	MongoCollectionPrefix string `json:"mongo_collection_prefix"`
	// MongoMaxCollectionNameLen 集合名长度上限,0 = 不限制。
	MongoMaxCollectionNameLen int32 `json:"mongo_max_collection_name_len"`

	// ------- MongoDB 性能类规则(默认 warn,只提示不拦;前 4 条变更与查询两模式通用)-------
	MongoRegexUnanchored  bool `json:"mongo_regex_unanchored"`
	MongoNegationOperator bool `json:"mongo_negation_operator"`
	MongoOrClause         bool `json:"mongo_or_clause"`
	// MongoLargeInList $in 元素个数上限,0 = 不限制。
	MongoLargeInList int32 `json:"mongo_large_in_list"`
	// 以下 3 条只适用于查询模式。
	MongoQueryForbidNoFilter bool `json:"mongo_query_forbid_no_filter"`
	MongoQueryForbidLookup   bool `json:"mongo_query_forbid_lookup"`
	MongoQueryForbidNoLimit  bool `json:"mongo_query_forbid_no_limit"`

	// ------- Redis 审核规则(internal/redischeck,自研;上游 bytebase 无此方言)-------
	// --- 安全类:默认 error(Register 时强制校验)---
	RedisForbidDangerous   bool `json:"redis_forbid_dangerous"`
	RedisForbidAdmin       bool `json:"redis_forbid_admin"`
	RedisForbidScript      bool `json:"redis_forbid_script"`
	RedisForbidReplica     bool `json:"redis_forbid_replica"`
	RedisForbidKeysCommand bool `json:"redis_forbid_keys_command"`
	// --- 结构类 ---
	RedisForbidRename bool `json:"redis_forbid_rename"`
	// RedisKeyPrefix key 命名前缀规范,空 = 不检查(如填 "app:")。
	RedisKeyPrefix string `json:"redis_key_prefix"`
	// RedisMaxKeyLen key 名长度上限,0 = 不限制。
	RedisMaxKeyLen  int32 `json:"redis_max_key_len"`
	RedisForbidMove bool  `json:"redis_forbid_move"`
	// --- 性能 / 风险类:默认只提示不拦 ---
	// RedisMaxAffectKeys 单条命令允许影响的键数上限,0 = 不限制。
	RedisMaxAffectKeys int32 `json:"redis_max_affect_keys"`
	// RedisMaxValueLen 单次写入 value 的字节数上限,0 = 不限制。
	RedisMaxValueLen int32 `json:"redis_max_value_len"`
	// RedisRequireTtl SET 必须带过期时间(防内存无上限增长)。
	RedisRequireTtl    bool `json:"redis_require_ttl"`
	RedisForbidPersist bool `json:"redis_forbid_persist"`
	// RedisScanRequireMatch SCAN 必须带 MATCH(否则等同于全量遍历)。
	RedisScanRequireMatch bool `json:"redis_scan_require_match"`
	// RedisForbidBlocking 禁止阻塞命令(BLPOP/BRPOP/BLMOVE/WAIT 等)。
	RedisForbidBlocking bool `json:"redis_forbid_blocking"`

	// ------- Doris 建表审核规则(internal/dorischeck,自研)-------
	// Doris 兼容 MySQL 协议与查询/DML 语法,但建表 DDL 是自有语法
	// (DISTRIBUTED BY / PROPERTIES / KEY 模型),MySQL 解析器解析不了。
	// DorisRequireDistribution 建表必须有 DISTRIBUTED BY(否则默认随机分桶,不可控),默认拦截。
	DorisRequireDistribution bool `json:"doris_require_distribution"`
	// DorisBucketLimit BUCKETS 数量上限,0 = 不限制。
	DorisBucketLimit int32 `json:"doris_bucket_limit"`
	// DorisReplicationNum 期望副本数,0 = 不校验。
	DorisReplicationNum int32 `json:"doris_replication_num"`
	// DorisKeyModel 是否提示 KEY 数据模型(聚合模型写入有约束)。
	DorisKeyModel bool `json:"doris_key_model"`
}

// Order 工单(用于 Exec),字段与 Yearning CoreSqlOrder 对齐。
type Order struct {
	WorkId      string `json:"work_id"`
	Username    string `json:"username"`
	Status      uint32 `json:"status"`
	Type        int32  `json:"type"` // 1 dml, 0 ddl
	Backup      uint32 `json:"backup"`
	Idc         string `json:"idc"`
	Source      string `json:"source"`
	SourceId    string `json:"source_id"`
	DataBase    string `json:"data_base"`
	Table       string `json:"table"`
	Date        string `json:"date"`
	Sql         string `json:"sql"`
	Text        string `json:"text"`
	Assigned    string `json:"assigned"`
	Delay       string `json:"delay"`
	RealName    string `json:"real_name"`
	ExecuteTime string `json:"execute_time"`
	CurrentStep int32  `json:"current_step"`
	OscInfo     string `json:"osc_info"`
	File        string `json:"file"`
}

// Record 单条语句审核/执行记录,与 Yearning engine.Record 对齐。
type Record struct {
	Sql              string   `json:"sql"`
	AffectRows       uint32   `json:"affect_rows"`
	Status           string   `json:"status"`
	Error            string   `json:"error"`
	Level            uint32   `json:"level"` // critical=0 error=1 warning=2 info=3 debug=4
	ExecTime         string   `json:"exec_time"`
	Table            string   `json:"table"`
	Schema           string   `json:"schema"`
	IsOsc            bool     `json:"is_osc"`
	InsulateWordList []string `json:"insulate_word_list"`
	// Rollback 该语句的回滚语句(执行前抓取前镜像生成,可能为空)。
	Rollback string `json:"rollback"`
}

type CheckRequest struct {
	Sql    string      `json:"sql"`
	Schema string      `json:"schema"`
	Source *DataSource `json:"source"`
	Lang   string      `json:"lang"`
	Rule   *AuditRole  `json:"rule"`
	// Mode 审核模式:空 / "write" = 变更命令审核(申请页检测、工单执行前复检),
	// "query" = 只读命令审核(查询页)。
	// 空值按 write 处理,保证旧调用方行为不变;不靠命令名自动推断模式——
	// 申请页里填 find 必须报「不是变更命令」,而不是悄悄按查询规则放行。
	Mode string `json:"mode"`
}

type CheckReply struct {
	Ok      bool      `json:"ok"`
	Error   string    `json:"error"`
	Records []*Record `json:"records"`
}

type MergeAlterTablesRequest struct {
	Sqls string `json:"sqls"`
}

type MergeAlterTablesReply struct {
	Ok    bool   `json:"ok"`
	Error string `json:"error"`
	Sql   string `json:"sql"`
}

type ExecRequest struct {
	Order         *Order      `json:"order"`
	Rules         *AuditRole  `json:"rules"`
	Source        *DataSource `json:"source"`
	MaxAffectRows uint32      `json:"max_affect_rows"`
}

type ExecReply struct {
	Ok      bool      `json:"ok"`
	Error   string    `json:"error"`
	Records []*Record `json:"records"` // 逐条执行明细(影响行/错误/回滚)。
	// Rollback 整单回滚语句(binlog 抓取,按事件逆序,每条语句一个元素)。
	Rollback []string `json:"rollback"`
}

type QueryRequest struct {
	Sql              string `json:"sql"`
	Limit            uint64 `json:"limit"`
	InsulateWordList string `json:"insulate_word_list"`
	// Kind 数据源方言(mysql / postgres / mongodb / redis)。查询预检据此选解析器;
	// 空值按 mysql 处理,保证旧调用方行为不变。
	Kind string `json:"kind"`
}

type QueryReply struct {
	Ok      bool      `json:"ok"`
	Error   string    `json:"error"`
	Records []*Record `json:"records"`
}
