package mongocheck

import (
	"strings"
	"testing"
)

// allOn 打开全部开关，用于验证规则本身；默认级别按 error
func allOn() Config {
	return Config{
		ForbidEmptyFilter:    true,
		ForbidDangerous:      true,
		ForbidWhere:          true,
		ForbidDropCollection: true,
	}
}

func TestCheckAllowsSaneCommands(t *testing.T) {
	allow := []string{
		`{"update":"users","updates":[{"q":{"_id":1},"u":{"$set":{"a":2}}}]}`,
		`{"delete":"users","deletes":[{"q":{"age":{"$gt":30}},"limit":0}]}`,
		`{"findAndModify":"users","query":{"_id":1},"update":{"$set":{"a":1}}}`,
		`{"insert":"users","documents":[{"a":1}]}`,
		`{"createIndexes":"users","indexes":[{"key":{"a":1},"name":"a_1"}]}`,
	}
	for _, q := range allow {
		_, findings, err := Check(q, allOn())
		if err != nil {
			t.Errorf("Check(%s) 应通过, got %v", q, err)
			continue
		}
		if hit := Blocked(findings); hit != nil {
			t.Errorf("Check(%s) 不该被拦截, got %+v", q, hit)
		}
	}
}

func TestCheckUsageErrors(t *testing.T) {
	// 不是变更命令
	if _, _, err := Check(`{"find":"users","limit":10}`, allOn()); err == nil {
		t.Error("查询命令应报用法错误")
	}
	// 不是合法 JSON
	if _, _, err := Check(`not json`, allOn()); err == nil {
		t.Error("非法 JSON 应报错")
	}
	// 空命令
	if _, _, err := Check(`{}`, allOn()); err == nil {
		t.Error("空命令应报错")
	}
}

func TestCheckBlockedWhenRulesOn(t *testing.T) {
	block := map[string]string{
		`{"update":"users","updates":[{"q":{},"u":{"$set":{"a":1}}}]}`: "非空 filter",
		`{"update":"users","updates":[{"u":{"$set":{"a":1}}}]}`:        "非空 filter",
		`{"delete":"users","deletes":[{"limit":0}]}`:                   "非空 filter",
		`{"update":"users","q":{},"u":{"$set":{"a":1}}}`:               "非空 filter",
		`{"findAndModify":"users","update":{"$set":{"a":1}}}`:          "非空 filter",
		`{"dropDatabase":1}`:       "dropDatabase",
		`{"eval":"while(true){}"}`: "eval",
		`{"delete":"users","deletes":[{"q":{"$where":"1"},"limit":0}]}`: "$where",
		`{"drop":"users"}`: "renameCollection",
	}
	for q, want := range block {
		_, findings, err := Check(q, allOn())
		if err != nil {
			t.Errorf("Check(%s) 不该报用法错误: %v", q, err)
			continue
		}
		hit := Blocked(findings)
		if hit == nil {
			t.Errorf("Check(%s) 应被拦截", q)
			continue
		}
		if !strings.Contains(hit.Message, want) {
			t.Errorf("Check(%s) 提示应含 %q, got %v", q, want, hit.Message)
		}
	}
}

func TestCheckRespectsSwitches(t *testing.T) {
	emptyFilter := `{"update":"users","updates":[{"q":{},"u":{"$set":{"a":1}}}]}`

	// 开关全关：什么都不拦（规则集没配就是关，与 SQL 规则一致）
	if _, findings, err := Check(emptyFilter, Config{}); err != nil || len(findings) != 0 {
		t.Errorf("开关全关时不应命中, got %+v, %v", findings, err)
	}
	// 只开空 filter 检查
	cfg := Config{ForbidEmptyFilter: true}
	_, findings, _ := Check(emptyFilter, cfg)
	if len(findings) != 1 || findings[0].Rule != RuleEmptyFilter {
		t.Errorf("应只命中空 filter, got %+v", findings)
	}
}

func TestCheckRespectsLevels(t *testing.T) {
	emptyFilter := `{"update":"users","updates":[{"q":{},"u":{"$set":{"a":1}}}]}`
	cfg := Config{
		ForbidEmptyFilter: true,
		Levels:            map[string]string{RuleEmptyFilter: "warn"},
	}
	_, findings, _ := Check(emptyFilter, cfg)
	if Blocked(findings) != nil {
		t.Error("warn 级不该拦截")
	}
	if len(findings) != 1 || findings[0].Level != "warn" {
		t.Errorf("应有一条 warn 命中, got %+v", findings)
	}
	if got := Severity(findings); got != 2 {
		t.Errorf("Severity = %d, want 2", got)
	}
	// 非法级别值回落到 error
	cfg.Levels = map[string]string{RuleEmptyFilter: "bogus"}
	if _, f, _ := Check(emptyFilter, cfg); Blocked(f) == nil {
		t.Error("非法级别应回落为 error 并拦截")
	}
}

// TestCheckQueryMode 查询模式（查询页）：只读命令照常审核，写命令一律拒绝。
// 写命令的拒绝是**硬拦**：开关全关也必须拦——查询页能改数据等于绕过工单审批。
func TestCheckQueryMode(t *testing.T) {
	allow := []string{
		`{"find":"users","filter":{"a":1},"limit":10}`,
		`{"aggregate":"users","pipeline":[{"$match":{"a":1}}]}`,
		`{"count":"users"}`,
		`{"distinct":"users","key":"a"}`,
	}
	for _, q := range allow {
		_, findings, err := Check(q, Config{Mode: ModeQuery})
		if err != nil {
			t.Errorf("Check(%s) 在查询模式下应通过, got %v", q, err)
			continue
		}
		if len(findings) != 0 {
			t.Errorf("Check(%s) 不该命中变更规则, got %+v", q, findings)
		}
	}

	deny := []string{
		`{"update":"users","updates":[{"q":{"_id":1},"u":{"$set":{"a":2}}}]}`,
		`{"delete":"users","deletes":[{"q":{"a":1},"limit":0}]}`,
		`{"insert":"users","documents":[{"a":1}]}`,
		`{"createIndexes":"users","indexes":[{"key":{"a":1},"name":"a_1"}]}`,
		`{"drop":"users"}`,
		`{"dropDatabase":1}`,
		`{"mapReduce":"users","out":"r"}`,
	}
	for _, q := range deny {
		_, _, err := Check(q, Config{Mode: ModeQuery})
		if err == nil || !strings.Contains(err.Error(), "只读") {
			t.Errorf("Check(%s) 在查询模式下应被拒绝, got %v", q, err)
		}
	}

	// 变更模式（默认）下只读命令仍报用法错误：申请页里填 find 的老行为不能变
	if _, _, err := Check(`{"find":"users"}`, Config{}); err == nil {
		t.Error("变更模式下 find 仍应报用法错误")
	}
	// mode 大小写与空白不敏感
	if _, _, err := Check(`{"find":"users"}`, Config{Mode: " QUERY "}); err != nil {
		t.Errorf("mode 应大小写/空白不敏感, got %v", err)
	}
}

func TestCommandNameIsFirstKey(t *testing.T) {
	// 命令名取 JSON 的首个键，且大小写不敏感
	_, findings, err := Check(`{"Drop":"users"}`, allOn())
	if err != nil {
		t.Fatalf("应识别 drop 命令: %v", err)
	}
	if Blocked(findings) == nil {
		t.Error("drop 集合在开关打开时应被拦截")
	}
	// 只读命令（首键为 find）应被判为查询
	if _, _, err := Check(`{"find":"users"}`, allOn()); err == nil {
		t.Error("find 应报用法错误")
	}
}
