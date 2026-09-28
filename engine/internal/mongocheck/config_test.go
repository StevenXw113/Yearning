package mongocheck

import (
	"testing"

	enginev1 "engine/gen/engine/v1"
)

// TestConfigFromProto 核对反射取数这一层：它是「新增规则不必再写逐行映射」的关键，
// 也是最容易静默失效的一层——名字对不上不会报错，只是开关永远读到零值。
func TestConfigFromProto(t *testing.T) {
	role := &enginev1.AuditRole{
		MongoForbidEmptyFilter: true,
		MongoForbidWhere:       true,
		RuleLevels:             map[string]string{RuleEmptyFilter: "warn"},
	}
	cfg := ConfigFromProto(role, ModeQuery)

	if !cfg.ForbidEmptyFilter {
		t.Error("mongo_forbid_empty_filter=true 应填进 Config.ForbidEmptyFilter")
	}
	if !cfg.ForbidWhere {
		t.Error("mongo_forbid_where=true 应填进 Config.ForbidWhere")
	}
	if cfg.ForbidDangerous || cfg.ForbidDropCollection {
		t.Error("没开的开关不该被填成 true")
	}
	if cfg.Mode != ModeQuery {
		t.Errorf("Mode = %q, want %q", cfg.Mode, ModeQuery)
	}
	// 级别：配了的走配置，没配的取规则自带的默认级别
	if got := cfg.Level(emptyFilterRule); got != LevelWarn {
		t.Errorf("空 filter 的级别应为配置里的 warn, got %q", got)
	}
	if got := cfg.Level(whereRule); got != LevelError {
		t.Errorf("未配级别时应取默认 error, got %q", got)
	}
	// 配了非法级别：回落默认级别，不静默变成「不拦」
	bad := ConfigFromProto(&enginev1.AuditRole{MongoForbidWhere: true, RuleLevels: map[string]string{RuleWhere: "bogus"}}, ModeWrite)
	if got := bad.Level(whereRule); got != LevelError {
		t.Errorf("非法级别应回落 error, got %q", got)
	}
	// 规则集为空（数据源没绑规则集）：全部关闭，不 panic
	empty := ConfigFromProto(nil, ModeWrite)
	if empty.ForbidEmptyFilter || empty.ForbidDangerous || empty.ForbidWhere || empty.ForbidDropCollection {
		t.Error("规则集为空时不应有任何开关开启")
	}
	if empty.queryMode() {
		t.Error("mode=write 不应被当成查询模式")
	}
}

// TestConfigQueryModeNormalize 模式的空白与大小写不敏感（主程序传的是原始字符串）
func TestConfigQueryModeNormalize(t *testing.T) {
	for _, m := range []Mode{"query", "QUERY", " Query ", ModeQuery} {
		if !(Config{Mode: m}).queryMode() {
			t.Errorf("Mode=%q 应识别为查询模式", m)
		}
	}
	for _, m := range []Mode{"", "write", "WRITE", "unknown"} {
		if (Config{Mode: m}).queryMode() {
			t.Errorf("Mode=%q 不该被识别为查询模式", m)
		}
	}
}
