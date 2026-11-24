package ruleset

import "encoding/json"

// ParamType describes how a rule parameter is represented in Definition.Rule.Params.
// Parameters are always encoded as a JSON object; scalar params are rejected by validators.
type ParamType string

const (
	ParamTypeNone   ParamType = "none"
	ParamTypeInt    ParamType = "int"
	ParamTypeString ParamType = "string"
)

// ManifestRule is the externally exportable metadata for one stable rule ID.
type ManifestRule struct {
	ID                    string          `json:"id"`
	LegacyID              string          `json:"legacy_id"`
	Name                  string          `json:"name"`
	Desc                  string          `json:"desc"`
	Category              string          `json:"category"`
	Modes                 []string        `json:"modes"`
	ParamType             ParamType       `json:"param_type"`
	ParamKey              string          `json:"param_key,omitempty"`
	DefaultParams         json.RawMessage `json:"default_params,omitempty"`
	DefaultLevel          Level           `json:"default_level"`
	SafetyBaseline        bool            `json:"safety_baseline"`
	ProductionRecommended bool            `json:"production_recommended"`
	RequiresDBAApproval   bool            `json:"requires_dba_approval"`
}

// Manifest is a versioned catalog used by rule-set import, export and UI rendering.
type Manifest struct {
	SchemaVersion int            `json:"schema_version"`
	Dialect       Dialect        `json:"dialect"`
	Scope         Scope          `json:"scope"`
	Rules         []ManifestRule `json:"rules"`
}

// MongoManifest returns the complete MongoDB data-source rule catalog.
// IDs are stable public identifiers (`mongo.*`); LegacyID is only a compatibility key.
func MongoManifest() Manifest {
	return Manifest{
		SchemaVersion: SchemaVersion,
		Dialect:       DialectMongoDB,
		Scope:         ScopeDataSource,
		Rules: []ManifestRule{
			{
				ID: "mongo.empty_filter", LegacyID: "MongoForbidEmptyFilter", Name: "禁止空过滤条件",
				Desc: "update/delete 不能使用空过滤条件", Category: "security", Modes: []string{"write"},
				ParamType: ParamTypeNone, DefaultLevel: LevelError, SafetyBaseline: true, ProductionRecommended: true,
			},
			{
				ID: "mongo.dangerous_command", LegacyID: "MongoForbidDangerous", Name: "禁止危险命令",
				Desc: "禁止 dropDatabase / eval / mapReduce 等危险命令", Category: "security", Modes: []string{"write"},
				ParamType: ParamTypeNone, DefaultLevel: LevelError, SafetyBaseline: true, ProductionRecommended: true,
			},
			{
				ID: "mongo.where", LegacyID: "MongoForbidWhere", Name: "禁止 $where",
				Desc: "禁止 $where（把脚本推到服务端执行）", Category: "security", Modes: []string{"write"},
				ParamType: ParamTypeNone, DefaultLevel: LevelError, SafetyBaseline: true, ProductionRecommended: true,
			},
			{
				ID: "mongo.collection.drop", LegacyID: "MongoForbidDropCollection", Name: "dropCollection 特批",
				Desc: "dropCollection 是破坏性操作，必须进入 DBA 特批流", Category: "security", Modes: []string{"write"},
				ParamType: ParamTypeNone, DefaultLevel: LevelError, SafetyBaseline: true, ProductionRecommended: true,
				RequiresDBAApproval: true,
			},
			{
				ID: "mongo.collection.rename", LegacyID: "MongoForbidDropCollection", Name: "renameCollection 特批",
				Desc: "renameCollection 会改变集合访问路径，必须进入 DBA 特批流", Category: "security", Modes: []string{"write"},
				ParamType: ParamTypeNone, DefaultLevel: LevelError, SafetyBaseline: true, ProductionRecommended: true,
				RequiresDBAApproval: true,
			},
			{
				ID: "mongo.system_collection", LegacyID: "MongoForbidSystemCollection", Name: "禁止系统集合",
				Desc: "禁止操作系统集合或以 system. 开头的集合", Category: "security", Modes: []string{"write"},
				ParamType: ParamTypeNone, DefaultLevel: LevelError, SafetyBaseline: true, ProductionRecommended: true,
			},
			{
				ID: "mongo.account.privilege", LegacyID: "MongoForbidAdminCommand", Name: "账号权限特批",
				Desc: "createUser / dropUser / grantRolesToUser 等账号权限命令必须进入 DBA 特批流", Category: "security", Modes: []string{"write"},
				ParamType: ParamTypeNone, DefaultLevel: LevelError, SafetyBaseline: true, ProductionRecommended: true,
				RequiresDBAApproval: true,
			},
			{
				ID: "mongo.immutable_id", LegacyID: "MongoForbidImmutableId", Name: "禁止修改 _id",
				Desc: "禁止通过 $set / $unset / $rename 或文档替换修改 _id", Category: "security", Modes: []string{"write"},
				ParamType: ParamTypeNone, DefaultLevel: LevelError, SafetyBaseline: true, ProductionRecommended: true,
			},
			{
				ID: "mongo.upsert", LegacyID: "MongoForbidUpsert", Name: "禁止 upsert",
				Desc: "禁止 update 带 upsert 的写入", Category: "security", Modes: []string{"write"},
				ParamType: ParamTypeNone, DefaultLevel: LevelError, SafetyBaseline: true,
			},
			{
				ID: "mongo.multi_write", LegacyID: "MongoForbidMultiWrite", Name: "禁止多文档写入",
				Desc: "update/delete 默认作用于多文档，必须显式声明并控制影响范围", Category: "security", Modes: []string{"write"},
				ParamType: ParamTypeNone, DefaultLevel: LevelError, SafetyBaseline: true, ProductionRecommended: true,
			},
			{
				ID: "mongo.capped_convert", LegacyID: "MongoForbidCappedConvert", Name: "禁止 convertToCapped",
				Desc: "convertToCapped 会重建集合且不可逆", Category: "structure", Modes: []string{"write"},
				ParamType: ParamTypeNone, DefaultLevel: LevelError, SafetyBaseline: true, ProductionRecommended: true,
			},
			{
				ID: "mongo.coll_mod", LegacyID: "MongoForbidCollMod", Name: "限制 collMod",
				Desc: "collMod 会修改 validator / TTL / 索引可见性等集合元数据", Category: "structure", Modes: []string{"write"},
				ParamType: ParamTypeNone, DefaultLevel: LevelWarn,
			},
			{
				ID: "mongo.index_key_limit", LegacyID: "MongoIndexKeyLimit", Name: "索引键数上限",
				Desc: "createIndexes 单索引键数不超过 max_keys", Category: "structure", Modes: []string{"write"},
				ParamType: ParamTypeInt, ParamKey: "max_keys", DefaultParams: json.RawMessage(`{"max_keys":0}`), DefaultLevel: LevelWarn,
			},
			{
				ID: "mongo.index_name_spec", LegacyID: "MongoIndexNameSpec", Name: "索引命名规范",
				Desc: "索引名需匹配 pattern 正则", Category: "structure", Modes: []string{"write"},
				ParamType: ParamTypeString, ParamKey: "pattern", DefaultParams: json.RawMessage(`{"pattern":""}`), DefaultLevel: LevelWarn,
			},
			{
				ID: "mongo.collection_prefix", LegacyID: "MongoCollectionPrefix", Name: "集合名前缀",
				Desc: "集合名需以 prefix 开头", Category: "structure", Modes: []string{"write"},
				ParamType: ParamTypeString, ParamKey: "prefix", DefaultParams: json.RawMessage(`{"prefix":""}`), DefaultLevel: LevelWarn,
			},
			{
				ID: "mongo.max_collection_name_len", LegacyID: "MongoMaxCollectionNameLen", Name: "集合名长度上限",
				Desc: "集合名长度不超过 max_length", Category: "structure", Modes: []string{"write"},
				ParamType: ParamTypeInt, ParamKey: "max_length", DefaultParams: json.RawMessage(`{"max_length":0}`), DefaultLevel: LevelWarn,
			},
			{
				ID: "mongo.regex_unanchored", LegacyID: "MongoRegexUnanchored", Name: "限制非锚定正则",
				Desc: "非锚定正则容易全集合扫描", Category: "performance", Modes: []string{"write", "query"},
				ParamType: ParamTypeNone, DefaultLevel: LevelWarn,
			},
			{
				ID: "mongo.negation_operator", LegacyID: "MongoNegationOperator", Name: "限制否定操作符",
				Desc: "$nin / $ne 通常无法有效使用索引", Category: "performance", Modes: []string{"write", "query"},
				ParamType: ParamTypeNone, DefaultLevel: LevelWarn,
			},
			{
				ID: "mongo.or_clause", LegacyID: "MongoOrClause", Name: "限制 $or",
				Desc: "$or 各分支需分别命中索引，否则会全集合扫描", Category: "performance", Modes: []string{"write", "query"},
				ParamType: ParamTypeNone, DefaultLevel: LevelWarn,
			},
			{
				ID: "mongo.large_in_list", LegacyID: "MongoLargeInList", Name: "$in 数量上限",
				Desc: "$in 元素个数不超过 max_items", Category: "performance", Modes: []string{"write", "query"},
				ParamType: ParamTypeInt, ParamKey: "max_items", DefaultParams: json.RawMessage(`{"max_items":0}`), DefaultLevel: LevelWarn,
			},
			{
				ID: "mongo.query.no_filter", LegacyID: "MongoQueryForbidNoFilter", Name: "查询必须有过滤条件",
				Desc: "find 需要非空 filter，aggregate 首阶段需要非空 $match", Category: "performance", Modes: []string{"query"},
				ParamType: ParamTypeNone, DefaultLevel: LevelWarn,
			},
			{
				ID: "mongo.query.no_lookup", LegacyID: "MongoQueryForbidLookup", Name: "查询禁用关联阶段",
				Desc: "禁止 $lookup / $graphLookup 等高成本关联阶段", Category: "performance", Modes: []string{"query"},
				ParamType: ParamTypeNone, DefaultLevel: LevelWarn,
			},
			{
				ID: "mongo.query.no_limit", LegacyID: "MongoQueryForbidNoLimit", Name: "排序查询必须限制返回数",
				Desc: "find 带 sort 时必须带正整数 limit", Category: "performance", Modes: []string{"query"},
				ParamType: ParamTypeNone, DefaultLevel: LevelWarn,
			},
		},
	}
}

// MongoKnownRules returns the set of stable MongoDB rule IDs accepted by import and validation.
func MongoKnownRules() map[string]struct{} {
	manifest := MongoManifest()
	out := make(map[string]struct{}, len(manifest.Rules))
	for _, rule := range manifest.Rules {
		out[rule.ID] = struct{}{}
	}
	return out
}
