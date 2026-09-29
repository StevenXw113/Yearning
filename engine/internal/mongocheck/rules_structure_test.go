package mongocheck

import (
	"strings"
	"testing"
)

// structureCfg 打开批次 4 的全部开关与参数。
// 集合名前缀不在默认集里：其它用例的集合名都叫 users，开了前缀规范会互相污染。
func structureCfg() Config {
	return Config{
		ForbidCappedConvert:  true,
		ForbidCollMod:        true,
		IndexKeyLimit:        5,
		IndexNameSpec:        "^idx_",
		MaxCollectionNameLen: 30,
	}
}

func TestCheckCappedConvert(t *testing.T) {
	q := `{"convertToCapped":"users","size":1024}`
	_, findings, err := Check(q, structureCfg())
	if err != nil {
		t.Fatalf("不该报用法错误: %v", err)
	}
	if hit := Blocked(findings); hit == nil || hit.Rule != RuleCappedConvert {
		t.Errorf("convertToCapped 应被拦截, got %+v", findings)
	}
	if _, findings, _ := Check(q, Config{}); Blocked(findings) != nil {
		t.Error("开关关闭时不该命中")
	}
}

// TestCheckCollMod 结构类默认 warn：命中只提示，不拦提交
func TestCheckCollMod(t *testing.T) {
	_, findings, err := Check(`{"collMod":"users","validator":{"a":{"$exists":true}}}`, structureCfg())
	if err != nil {
		t.Fatalf("不该报用法错误: %v", err)
	}
	if len(findings) != 1 || findings[0].Rule != RuleCollMod || findings[0].Level != LevelWarn {
		t.Errorf("collMod 应命中一条 warn, got %+v", findings)
	}
	if Blocked(findings) != nil {
		t.Error("warn 级不该拦提交")
	}
}

func TestCheckIndexKeyLimit(t *testing.T) {
	six := `{"createIndexes":"users","indexes":[{"key":{"a":1,"b":1,"c":1,"d":1,"e":1,"f":1},"name":"idx_six"}]}`
	if _, findings, _ := Check(six, structureCfg()); len(findings) == 0 {
		t.Error("6 个键超过上限 5，应命中")
	}
	five := `{"createIndexes":"users","indexes":[{"key":{"a":1,"b":1,"c":1,"d":1,"e":1},"name":"idx_five"}]}`
	if _, findings, _ := Check(five, structureCfg()); len(findings) != 0 {
		t.Errorf("5 个键等于上限，不该命中: %+v", findings)
	}
	// 0 = 不限制
	cfg := structureCfg()
	cfg.IndexKeyLimit = 0
	if _, findings, _ := Check(six, cfg); len(findings) != 0 {
		t.Errorf("上限为 0 时不该命中: %+v", findings)
	}
}

func TestCheckIndexNameSpec(t *testing.T) {
	bad := `{"createIndexes":"users","indexes":[{"key":{"a":1},"name":"a_1"}]}`
	_, findings, _ := Check(bad, structureCfg())
	if len(findings) != 1 || findings[0].Rule != RuleIndexNameSpec {
		t.Errorf("索引名不符合规范应命中, got %+v", findings)
	}
	good := `{"createIndexes":"users","indexes":[{"key":{"a":1},"name":"idx_a"}]}`
	if _, findings, _ := Check(good, structureCfg()); len(findings) != 0 {
		t.Errorf("符合规范的索引名不该命中: %+v", findings)
	}
	// 空参数 = 规则不生效（与 MySQL 侧字符串参数语义一致）
	cfg := structureCfg()
	cfg.IndexNameSpec = ""
	if _, findings, _ := Check(bad, cfg); len(findings) != 0 {
		t.Errorf("参数为空时规则不该生效: %+v", findings)
	}
	// 非法正则不能 panic，也不能拦（宁可不生效，不误拦）
	cfg = structureCfg()
	cfg.IndexNameSpec = "(["
	if _, findings, _ := Check(bad, cfg); len(findings) != 0 {
		t.Errorf("非法正则应视为不生效: %+v", findings)
	}
}

func TestCheckCollectionNaming(t *testing.T) {
	cfg := structureCfg()
	cfg.CollectionPrefix = "app_"
	_, findings, _ := Check(`{"create":"users","validator":{}}`, cfg)
	if len(findings) != 1 || findings[0].Rule != RuleCollectionPrefix {
		t.Fatalf("users 不匹配前缀 app_，应只命中前缀规则, got %+v", findings)
	}
	// 合规集合名：前缀匹配且在长度内
	if _, findings, _ := Check(`{"create":"app_users","validator":{}}`, cfg); len(findings) != 0 {
		t.Errorf("合规集合名不该命中: %+v", findings)
	}
	// 只有长度规则开启时：只要超长就命中
	cfg.CollectionPrefix = ""
	long := `{"create":"app_averyveryverylongcollectionname","validator":{}}`
	_, findings, _ = Check(long, cfg)
	if len(findings) != 1 || findings[0].Rule != RuleMaxCollectionNameLen {
		t.Errorf("超长集合名应命中长度规则, got %+v", findings)
	}
	cfg.MaxCollectionNameLen = 0
	if _, findings, _ := Check(long, cfg); len(findings) != 0 {
		t.Errorf("长度上限为 0 时不该命中: %+v", findings)
	}
}

// QueryOnlyAndSharedCfg 性能规则：4 条共用（变更+查询），3 条只用查询模式
func perfCfg(mode Mode) Config {
	return Config{
		Mode:                mode,
		RegexUnanchored:     true,
		NegationOperator:    true,
		OrClause:            true,
		LargeInList:         100,
		QueryForbidNoFilter: true,
		QueryForbidLookup:   true,
		QueryForbidNoLimit:  true,
	}
}

func TestCheckPerfOperatorsInBothModes(t *testing.T) {
	cases := map[string]string{
		RuleRegexUnanchored:  `{"name":{"$regex":"abc"}}`,
		RuleNegationOperator: `{"age":{"$ne":18}}`,
		RuleOrClause:         `{"$or":[{"a":1},{"b":2}]}`,
	}
	// 变更模式：filter 在 updates[].q
	for rule, filter := range cases {
		q := `{"update":"users","updates":[{"q":` + filter + `,"u":{"$set":{"x":1}}}]}`
		_, findings, err := Check(q, perfCfg(ModeWrite))
		if err != nil {
			t.Fatalf("Check(%s) 不该报用法错误: %v", q, err)
		}
		if !hasRule(findings, rule) {
			t.Errorf("变更模式下 %s 应命中 %s, got %+v", q, rule, findings)
		}
	}
	// 查询模式：filter 在顶层
	for rule, filter := range cases {
		q := `{"find":"users","filter":` + filter + `}`
		if _, findings, _ := Check(q, perfCfg(ModeQuery)); !hasRule(findings, rule) {
			t.Errorf("查询模式下 %s 应命中 %s, got %+v", q, rule, findings)
		}
	}
	// 锚定的正则该放行
	if _, findings, _ := Check(`{"find":"users","filter":{"name":{"$regex":"^abc"}}}`, perfCfg(ModeQuery)); hasRule(findings, RuleRegexUnanchored) {
		t.Error("以 ^ 锚定的正则不该命中")
	}
}

func TestCheckLargeInList(t *testing.T) {
	big := `{"find":"users","filter":{"a":{"$in":[` + strings.Repeat("1,", 100) + `1]}}}`
	_, findings, err := Check(big, perfCfg(ModeQuery))
	if err != nil {
		t.Fatalf("不该报用法错误: %v", err)
	}
	if !hasRule(findings, RuleLargeInList) {
		t.Errorf("101 个元素超过上限 100 应命中, got %+v", findings)
	}
	small := `{"find":"users","filter":{"a":{"$in":[` + strings.Repeat("1,", 98) + `1]}}}`
	if _, findings, _ := Check(small, perfCfg(ModeQuery)); hasRule(findings, RuleLargeInList) {
		t.Error("99 个元素不该命中")
	}
}

func TestCheckQueryRules(t *testing.T) {
	cfg := perfCfg(ModeQuery)

	noFilter := []string{
		`{"find":"users"}`,
		`{"find":"users","filter":{}}`,
		`{"aggregate":"users","pipeline":[{"$group":{"_id":"$a"}}],"cursor":{}}`,
	}
	for _, q := range noFilter {
		if _, findings, _ := Check(q, cfg); !hasRule(findings, RuleQueryNoFilter) {
			t.Errorf("%s 应命中全集合扫描（无 filter/$match）, got %+v", q, findings)
		}
	}
	ok := []string{
		`{"find":"users","filter":{"a":1}}`,
		`{"aggregate":"users","pipeline":[{"$match":{"a":1}}],"cursor":{}}`,
	}
	for _, q := range ok {
		if _, findings, _ := Check(q, cfg); hasRule(findings, RuleQueryNoFilter) {
			t.Errorf("%s 不该命中无 filter 规则, got %+v", q, findings)
		}
	}

	if _, findings, _ := Check(`{"aggregate":"users","pipeline":[{"$lookup":{"from":"o","localField":"a","foreignField":"b","as":"x"}}],"cursor":{}}`, cfg); !hasRule(findings, RuleQueryForbidLookup) {
		t.Errorf("$lookup 应命中, got %+v", findings)
	}
	if _, findings, _ := Check(`{"find":"users","filter":{"a":1},"sort":{"a":1}}`, cfg); !hasRule(findings, RuleQueryNoLimit) {
		t.Errorf("带 sort 无 limit 应命中, got %+v", findings)
	}
	if _, findings, _ := Check(`{"find":"users","filter":{"a":1},"sort":{"a":1},"limit":20}`, cfg); hasRule(findings, RuleQueryNoLimit) {
		t.Errorf("带 limit 不该命中, got %+v", findings)
	}
}

// TestCheckQueryOnlyRulesNotInWriteMode 查询侧规则不能参与变更审核
func TestCheckQueryOnlyRulesNotInWriteMode(t *testing.T) {
	cfg := perfCfg(ModeWrite)
	_, findings, err := Check(`{"update":"users","updates":[{"q":{"a":1},"u":{"$set":{"b":2}}}]}`, cfg)
	if err != nil {
		t.Fatalf("不该报用法错误: %v", err)
	}
	for _, f := range findings {
		if strings.HasPrefix(f.Rule, "MongoQuery") {
			t.Errorf("变更模式不该跑查询侧规则, got %+v", f)
		}
	}
}

func hasRule(findings []Finding, rule string) bool {
	for _, f := range findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}
