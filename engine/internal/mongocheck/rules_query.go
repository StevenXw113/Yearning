package mongocheck

import (
	"strconv"
	"strings"
)

// 性能类规则：默认 warn，只提示不拦。
//
// 这里判的是「操作符形态」——静态审核拿不到索引信息，也没法看执行计划，
// 所以只标出「大概率用不上索引」的写法，由各环境按自己的索引与数据量决定要不要收紧。
const (
	// RuleRegexUnanchored $regex 未以 ^ 锚定（无法用索引做前缀匹配）
	RuleRegexUnanchored = "MongoRegexUnanchored"
	// RuleNegationOperator $ne / $nin / $not / $nor（选择性差，通常退化为全集合扫描）
	RuleNegationOperator = "MongoNegationOperator"
	// RuleOrClause 出现 $or（每个分支各自决定能否用索引）
	RuleOrClause = "MongoOrClause"
	// RuleLargeInList $in 元素过多（参数：0 = 不限）
	RuleLargeInList = "MongoLargeInList"
	// RuleQueryNoFilter 查询无 filter / aggregate 无 $match（全集合扫描）
	RuleQueryNoFilter = "MongoQueryForbidNoFilter"
	// RuleQueryForbidLookup aggregate 使用 $lookup / $graphLookup
	RuleQueryForbidLookup = "MongoQueryForbidLookup"
	// RuleQueryNoLimit 带 sort 但无 limit
	RuleQueryNoLimit = "MongoQueryForbidNoLimit"
)

var (
	regexUnanchoredRule  = regexUnanchored{}
	negationOperatorRule = negationOperator{}
	orClauseRule         = orClause{}
	largeInListRule      = largeInList{}
	queryNoFilterRule    = queryNoFilter{}
	queryLookupRule      = queryLookup{}
	queryNoLimitRule     = queryNoLimit{}
)

func init() {
	Register(regexUnanchoredRule)
	Register(negationOperatorRule)
	Register(orClauseRule)
	Register(largeInListRule)
	Register(queryNoFilterRule)
	Register(queryLookupRule)
	Register(queryNoLimitRule)
}

// ---------- 变更与查询共用（Modes 两种都适用）----------

type regexUnanchored struct{}

func (regexUnanchored) Name() string { return RuleRegexUnanchored }
func (regexUnanchored) Desc() string {
	return "禁止未以 ^ 锚定的 $regex（无法使用索引，退化为全集合扫描）"
}
func (regexUnanchored) Category() Category  { return CategoryPerformance }
func (regexUnanchored) Modes() []Mode       { return []Mode{ModeWrite, ModeQuery} }
func (regexUnanchored) DefaultLevel() Level { return LevelWarn }

func (r regexUnanchored) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.RegexUnanchored {
		return nil
	}
	hit := cmd.Walk(func(key string, val interface{}) bool {
		if !strings.EqualFold(key, "$regex") {
			return false
		}
		switch v := val.(type) {
		case string:
			return !strings.HasPrefix(v, "^")
		case map[string]interface{}: // {"$regex": {"pattern": "..."}}
			p, ok := v["pattern"].(string)
			return ok && !strings.HasPrefix(p, "^")
		}
		return false
	})
	if !hit {
		return nil
	}
	return []Finding{{
		Rule:    RuleRegexUnanchored,
		Level:   cfg.Level(r),
		Message: "$regex 未以 ^ 锚定，无法使用索引；前缀匹配请写成 ^开头",
	}}
}

type negationOperator struct{}

func (negationOperator) Name() string { return RuleNegationOperator }
func (negationOperator) Desc() string {
	return "禁止 $ne / $nin / $not / $nor（选择性差，通常全集合扫描）"
}
func (negationOperator) Category() Category  { return CategoryPerformance }
func (negationOperator) Modes() []Mode       { return []Mode{ModeWrite, ModeQuery} }
func (negationOperator) DefaultLevel() Level { return LevelWarn }

var negationOperators = []string{"$ne", "$nin", "$not", "$nor"}

func (r negationOperator) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.NegationOperator {
		return nil
	}
	for _, op := range negationOperators {
		if cmd.Has(op) {
			return []Finding{{
				Rule:    RuleNegationOperator,
				Level:   cfg.Level(r),
				Message: "使用了 " + op + "：否定条件通常无法用索引，容易全集合扫描",
			}}
		}
	}
	return nil
}

type orClause struct{}

func (orClause) Name() string { return RuleOrClause }
func (orClause) Desc() string {
	return "禁止 $or（各分支需分别走索引，容易退化为全集合扫描）"
}
func (orClause) Category() Category  { return CategoryPerformance }
func (orClause) Modes() []Mode       { return []Mode{ModeWrite, ModeQuery} }
func (orClause) DefaultLevel() Level { return LevelWarn }

func (r orClause) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.OrClause || !cmd.Has("$or") {
		return nil
	}
	return []Finding{{
		Rule:    RuleOrClause,
		Level:   cfg.Level(r),
		Message: "使用了 $or：请确认每个分支都能命中索引（否则会全集合扫描）",
	}}
}

type largeInList struct{}

func (largeInList) Name() string        { return RuleLargeInList }
func (largeInList) Desc() string        { return "$in 元素个数不超过参数上限（0 = 不限）" }
func (largeInList) Category() Category  { return CategoryPerformance }
func (largeInList) Modes() []Mode       { return []Mode{ModeWrite, ModeQuery} }
func (largeInList) DefaultLevel() Level { return LevelWarn }

func (r largeInList) Check(cfg Config, cmd *Command) []Finding {
	if cfg.LargeInList <= 0 {
		return nil
	}
	n := 0
	hit := cmd.Walk(func(key string, val interface{}) bool {
		if !strings.EqualFold(key, "$in") {
			return false
		}
		if arr, ok := val.([]interface{}); ok && len(arr) > cfg.LargeInList {
			n = len(arr)
			return true
		}
		return false
	})
	if !hit {
		return nil
	}
	return []Finding{{
		Rule:    RuleLargeInList,
		Level:   cfg.Level(r),
		Message: "$in 有 " + strconv.Itoa(n) + " 个元素，超过上限 " + strconv.Itoa(cfg.LargeInList) + "；请分批或改用其他条件",
	}}
}

// ---------- 只在查询模式生效 ----------

type queryNoFilter struct{}

func (queryNoFilter) Name() string { return RuleQueryNoFilter }
func (queryNoFilter) Desc() string {
	return "查询条件不能为空：find 需要 filter，aggregate 需要 $match（避免全集合扫描）"
}
func (queryNoFilter) Category() Category  { return CategoryPerformance }
func (queryNoFilter) Modes() []Mode       { return []Mode{ModeQuery} }
func (queryNoFilter) DefaultLevel() Level { return LevelWarn }

func (r queryNoFilter) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.QueryForbidNoFilter {
		return nil
	}
	switch cmd.Name {
	case "find":
		if v, ok := cmd.Lookup("filter"); ok && !IsEmptyDoc(v) {
			return nil
		}
	case "aggregate":
		// 聚合没有 filter 字段，判据是管道里有没有 $match
		if cmd.Has("$match") {
			return nil
		}
	default:
		return nil
	}
	return []Finding{{
		Rule:    RuleQueryNoFilter,
		Level:   cfg.Level(r),
		Message: "查询没有过滤条件（find 的 filter / aggregate 的 $match），会扫描整个集合",
	}}
}

type queryLookup struct{}

func (queryLookup) Name() string { return RuleQueryForbidLookup }
func (queryLookup) Desc() string {
	return "查询禁用 $lookup / $graphLookup（关联查询代价高，且难以走索引）"
}
func (queryLookup) Category() Category  { return CategoryPerformance }
func (queryLookup) Modes() []Mode       { return []Mode{ModeQuery} }
func (queryLookup) DefaultLevel() Level { return LevelWarn }

func (r queryLookup) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.QueryForbidLookup {
		return nil
	}
	for _, stage := range []string{"$lookup", "$graphLookup"} {
		if cmd.Has(stage) {
			return []Finding{{
				Rule:    RuleQueryForbidLookup,
				Level:   cfg.Level(r),
				Message: "聚合使用了 " + stage + "：关联查询代价高，请确认数据量",
			}}
		}
	}
	return nil
}

type queryNoLimit struct{}

func (queryNoLimit) Name() string { return RuleQueryNoLimit }
func (queryNoLimit) Desc() string {
	return "find 带 sort 时必须带 limit（否则服务端要对全部命中排序）"
}
func (queryNoLimit) Category() Category  { return CategoryPerformance }
func (queryNoLimit) Modes() []Mode       { return []Mode{ModeQuery} }
func (queryNoLimit) DefaultLevel() Level { return LevelWarn }

func (r queryNoLimit) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.QueryForbidNoLimit || cmd.Name != "find" || !cmd.Has("sort") {
		return nil
	}
	if v, ok := cmd.Lookup("limit"); ok {
		if n, ok := AsInt(v); ok && n > 0 {
			return nil // 有上限
		}
	}
	return []Finding{{
		Rule:    RuleQueryNoLimit,
		Level:   cfg.Level(r),
		Message: "find 带 sort 但没写 limit：服务端需要对全部命中排序，请加上 limit",
	}}
}
