package mongocheck

import (
	"regexp"
	"strconv"
	"strings"
)

// 结构类规则：集合/索引的结构与命名。默认 warn（命名规范这类误报率较高），
// 只有不可逆的 convertToCapped 默认拦截。
const (
	// RuleCappedConvert convertToCapped 会重建集合（不可逆）
	RuleCappedConvert = "MongoForbidCappedConvert"
	// RuleCollMod collMod 改 validator / TTL / 索引可见性
	RuleCollMod = "MongoForbidCollMod"
	// RuleIndexKeyLimit 单索引键数上限（参数：0 = 不限）
	RuleIndexKeyLimit = "MongoIndexKeyLimit"
	// RuleIndexNameSpec 索引命名规范（参数：正则，空 = 不生效）
	RuleIndexNameSpec = "MongoIndexNameSpec"
	// RuleCollectionPrefix 集合名前缀规范（参数：字符串，空 = 不生效）
	RuleCollectionPrefix = "MongoCollectionPrefix"
	// RuleMaxCollectionNameLen 集合名长度上限（参数：0 = 不限）
	RuleMaxCollectionNameLen = "MongoMaxCollectionNameLen"
)

var (
	cappedConvertRule    = forbidCappedConvert{}
	collModRule          = forbidCollMod{}
	indexKeyLimitRule    = indexKeyLimit{}
	indexNameSpecRule    = indexNameSpec{}
	collectionPrefixRule = collectionPrefix{}
	maxCollectionLenRule = maxCollectionNameLen{}
)

func init() {
	Register(cappedConvertRule)
	Register(collModRule)
	Register(indexKeyLimitRule)
	Register(indexNameSpecRule)
	Register(collectionPrefixRule)
	Register(maxCollectionLenRule)
}

// ---------- convertToCapped ----------

type forbidCappedConvert struct{}

func (forbidCappedConvert) Name() string { return RuleCappedConvert }
func (forbidCappedConvert) Desc() string {
	return "禁止 convertToCapped（会重建集合，不可逆）"
}
func (forbidCappedConvert) Category() Category  { return CategoryStructure }
func (forbidCappedConvert) Modes() []Mode       { return []Mode{ModeWrite} }
func (forbidCappedConvert) DefaultLevel() Level { return LevelError }

func (r forbidCappedConvert) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.ForbidCappedConvert || cmd.Name != "converttocapped" {
		return nil
	}
	return []Finding{{
		Rule:    RuleCappedConvert,
		Level:   cfg.Level(r),
		Message: "禁止 convertToCapped：它会重建集合（索引与写入语义都会变），且不可回滚",
	}}
}

// ---------- collMod ----------

type forbidCollMod struct{}

func (forbidCollMod) Name() string { return RuleCollMod }
func (forbidCollMod) Desc() string {
	return "禁止 collMod（修改 validator / TTL / 索引可见性等集合元数据）"
}
func (forbidCollMod) Category() Category  { return CategoryStructure }
func (forbidCollMod) Modes() []Mode       { return []Mode{ModeWrite} }
func (forbidCollMod) DefaultLevel() Level { return LevelWarn }

func (r forbidCollMod) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.ForbidCollMod || cmd.Name != "collmod" {
		return nil
	}
	return []Finding{{
		Rule:    RuleCollMod,
		Level:   cfg.Level(r),
		Message: "collMod 会改变集合的校验规则/索引属性，请确认影响范围",
	}}
}

// ---------- 索引键数上限 ----------

type indexKeyLimit struct{}

func (indexKeyLimit) Name() string { return RuleIndexKeyLimit }
func (indexKeyLimit) Desc() string {
	return "createIndexes 单索引键数不超过参数上限（0 = 不限）"
}
func (indexKeyLimit) Category() Category  { return CategoryStructure }
func (indexKeyLimit) Modes() []Mode       { return []Mode{ModeWrite} }
func (indexKeyLimit) DefaultLevel() Level { return LevelWarn }

func (r indexKeyLimit) Check(cfg Config, cmd *Command) []Finding {
	if cfg.IndexKeyLimit <= 0 || cmd.Name != "createindexes" {
		return nil
	}
	for _, idx := range cmd.Indexes() {
		key, ok := idx["key"]
		if !ok {
			continue
		}
		if n := KeyCount(key); n > cfg.IndexKeyLimit {
			return []Finding{{
				Rule:    RuleIndexKeyLimit,
				Level:   cfg.Level(r),
				Message: "索引键数 " + strconv.Itoa(n) + " 超过上限 " + strconv.Itoa(cfg.IndexKeyLimit) + "（复合索引键越多，写入与更新成本越高）",
			}}
		}
	}
	return nil
}

// ---------- 索引命名规范 ----------

type indexNameSpec struct{}

func (indexNameSpec) Name() string { return RuleIndexNameSpec }
func (indexNameSpec) Desc() string {
	return "索引名需匹配参数里的正则（空 = 不生效）"
}
func (indexNameSpec) Category() Category  { return CategoryStructure }
func (indexNameSpec) Modes() []Mode       { return []Mode{ModeWrite} }
func (indexNameSpec) DefaultLevel() Level { return LevelWarn }

func (r indexNameSpec) Check(cfg Config, cmd *Command) []Finding {
	// 空参数 = 规则不生效（与 MySQL 侧字符串参数一致）；非法正则同样视为不生效，
	// 宁可不检查，也不要把「配置写错」变成拦不住的误报
	spec := strings.TrimSpace(cfg.IndexNameSpec)
	if spec == "" || cmd.Name != "createindexes" {
		return nil
	}
	re, err := regexp.Compile(spec)
	if err != nil {
		return nil
	}
	for _, idx := range cmd.Indexes() {
		name, _ := idx["name"].(string)
		if name == "" {
			continue // 未命名索引由服务端按 key 生成，不是命名问题
		}
		if !re.MatchString(name) {
			return []Finding{{
				Rule:    RuleIndexNameSpec,
				Level:   cfg.Level(r),
				Message: "索引名 " + name + " 不符合规范 " + spec,
			}}
		}
	}
	return nil
}

// ---------- 集合命名规范 ----------

type collectionPrefix struct{}

func (collectionPrefix) Name() string        { return RuleCollectionPrefix }
func (collectionPrefix) Desc() string        { return "集合名需以参数为前缀（空 = 不生效）" }
func (collectionPrefix) Category() Category  { return CategoryStructure }
func (collectionPrefix) Modes() []Mode       { return []Mode{ModeWrite} }
func (collectionPrefix) DefaultLevel() Level { return LevelWarn }

func (r collectionPrefix) Check(cfg Config, cmd *Command) []Finding {
	prefix := strings.TrimSpace(cfg.CollectionPrefix)
	if prefix == "" || cmd.Coll == "" {
		return nil
	}
	if strings.HasPrefix(cmd.Coll, prefix) {
		return nil
	}
	return []Finding{{
		Rule:    RuleCollectionPrefix,
		Level:   cfg.Level(r),
		Message: "集合名 " + cmd.Coll + " 未以 " + prefix + " 开头（命名规范）",
	}}
}

type maxCollectionNameLen struct{}

func (maxCollectionNameLen) Name() string { return RuleMaxCollectionNameLen }
func (maxCollectionNameLen) Desc() string {
	return "集合名长度不超过参数上限（0 = 不限）"
}
func (maxCollectionNameLen) Category() Category  { return CategoryStructure }
func (maxCollectionNameLen) Modes() []Mode       { return []Mode{ModeWrite} }
func (maxCollectionNameLen) DefaultLevel() Level { return LevelWarn }

func (r maxCollectionNameLen) Check(cfg Config, cmd *Command) []Finding {
	if cfg.MaxCollectionNameLen <= 0 || cmd.Coll == "" {
		return nil
	}
	if len(cmd.Coll) <= cfg.MaxCollectionNameLen {
		return nil
	}
	return []Finding{{
		Rule:    RuleMaxCollectionNameLen,
		Level:   cfg.Level(r),
		Message: "集合名长度 " + strconv.Itoa(len(cmd.Coll)) + " 超过上限 " + strconv.Itoa(cfg.MaxCollectionNameLen),
	}}
}
