package redischeck

import (
	"reflect"
	"strings"
	"testing"

	"engine/api"
)

// allOn 打开全部开关，用于验证规则本身；参数取一个宽松值
func allOn() Config {
	return Config{
		ForbidDangerous:   true,
		ForbidAdmin:       true,
		ForbidScript:      true,
		ForbidReplica:     true,
		ForbidKeysCommand: true,
		ForbidRename:      true,
		KeyPrefix:         "",
		MaxKeyLen:         0,
		ForbidMove:        true,
		MaxAffectKeys:     0,
		MaxValueLen:       0,
		RequireTTL:        false,
		ForbidPersist:     true,
		ScanRequireMatch:  false,
		ForbidBlocking:    true,
	}
}

func TestRegisteredRules(t *testing.T) {
	all := All()
	if len(all) == 0 {
		t.Fatal("未注册任何 Redis 规则")
	}
	seen := map[string]bool{}
	for _, r := range all {
		if r.Name() == "" || r.Desc() == "" {
			t.Errorf("规则 %q 缺少 Name 或 Desc", r.Name())
		}
		if seen[r.Name()] {
			t.Errorf("规则重复: %s", r.Name())
		}
		seen[r.Name()] = true
		if len(r.Modes()) == 0 {
			t.Errorf("规则 %s 未声明适用模式", r.Name())
		}
		if !ValidLevel(r.DefaultLevel()) {
			t.Errorf("规则 %s 默认级别非法: %s", r.Name(), r.DefaultLevel())
		}
		// 安全类必须默认拦截（Register 也会 panic 校验，这里是双保险）
		if r.Category() == CategorySecurity && r.DefaultLevel() != LevelError {
			t.Errorf("安全类规则 %s 的默认级别应为 error", r.Name())
		}
		// 规则名必须是 Redis 前缀，才能被 ConfigFromAPI 的反射映射取到
		if !strings.HasPrefix(r.Name(), "Redis") {
			t.Errorf("规则 %s 的名称应以 Redis 开头（proto 字段前缀约定）", r.Name())
		}
	}
	if len(all) < 12 {
		t.Errorf("规则数量 %d 偏少，是否漏了 init 注册？", len(all))
	}
}

func TestCheckWriteMode(t *testing.T) {
	cfg := allOn()

	allow := []string{
		"SET app:k v EX 60",
		"DEL app:k1 app:k2",
		"HSET app:h f v",
		"LPUSH app:l a b",
		"EXPIRE app:k 60",
	}
	for _, line := range allow {
		if _, findings, err := Check(line, cfg); err != nil {
			t.Errorf("Check(%q) 不该报错: %v", line, err)
		} else if Blocked(findings) != nil {
			t.Errorf("Check(%q) 不该被拦截: %+v", line, Blocked(findings))
		}
	}

	block := map[string]string{
		"FLUSHALL":               "FLUSHALL",
		"FLUSHDB":                "FLUSHDB",
		"SHUTDOWN NOSAVE":        "SHUTDOWN",
		"CONFIG SET maxmemory 0": "CONFIG",
		"DEBUG SLEEP 1":          "DEBUG",
		`EVAL "return 1" 0`:      "EVAL",
		"REPLICAOF NO ONE":       "REPLICAOF",
		"KEYS *":                 "KEYS",
	}
	for line, want := range block {
		_, findings, err := Check(line, cfg)
		if err != nil {
			// 危险命令被判为 KindDDL，不应落到「不是变更命令」
			t.Errorf("Check(%q) 不该报用法错误: %v", line, err)
			continue
		}
		hit := Blocked(findings)
		if hit == nil {
			t.Errorf("Check(%q) 应被拦截", line)
			continue
		}
		if !strings.Contains(hit.Message, want) {
			t.Errorf("Check(%q) 提示应含 %q, got %v", line, want, hit.Message)
		}
	}
}

// 性能/风险类规则默认只提示（warn / observe）：应该命中，但不该拦下提交
func TestWarnLevelRulesDoNotBlock(t *testing.T) {
	cfg := allOn()
	cases := map[string]string{
		"BLPOP q 30":    "BLPOP",
		"PERSIST app:k": "PERSIST",
	}
	for line, want := range cases {
		_, findings, err := Check(line, cfg)
		if err != nil {
			t.Errorf("Check(%q) 不该报用法错误: %v", line, err)
			continue
		}
		if len(findings) == 0 {
			t.Errorf("Check(%q) 应命中（提示级）", line)
			continue
		}
		if hit := Blocked(findings); hit != nil {
			t.Errorf("Check(%q) 默认不该拦截（规则 %s 的默认级别是提示级）, got %+v", line, hit.Rule, hit)
		}
		if !strings.Contains(Messages(findings), want) {
			t.Errorf("Check(%q) 的说明应提到 %q, got %s", line, want, Messages(findings))
		}
	}
}

// 把提示级规则在规则集里调成 error，就应该拦下来
func TestWarnRuleCanBeRaised(t *testing.T) {
	cfg := allOn()
	cfg.Levels = map[string]string{"RedisForbidBlocking": "error"}
	_, findings, err := Check("BLPOP q 30", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if Blocked(findings) == nil {
		t.Error("把级别调成 error 后应拦截")
	}
}

func TestCheckQueryMode(t *testing.T) {
	cfg := allOn()
	cfg.Mode = ModeQuery

	// 只读命令放行
	for _, line := range []string{"GET app:k", "SCAN 0 MATCH app:* COUNT 10", "HGETALL app:h"} {
		if _, _, err := Check(line, cfg); err != nil {
			t.Errorf("查询模式 Check(%q) 不该报错: %v", line, err)
		}
	}
	// 写命令在查询页直接拒绝（不可配置）
	for _, line := range []string{"SET app:k v", "DEL app:k", "HSET app:h f v"} {
		_, _, err := Check(line, cfg)
		if err == nil {
			t.Errorf("查询模式 Check(%q) 应被拒", line)
			continue
		}
		if !strings.Contains(err.Error(), "只读命令") {
			t.Errorf("查询模式 Check(%q) 的文案应说明只能只读, got %v", line, err)
		}
	}
}

func TestCheckUsageErrors(t *testing.T) {
	cfg := allOn()
	if _, _, err := Check("   ", cfg); err == nil {
		t.Error("空命令应报错")
	}
	if _, _, err := Check(`SET "unclosed`, cfg); err == nil {
		t.Error("引号未闭合应报错")
	}
	// 变更模式下只读命令应提示「不是变更命令」
	_, _, err := Check("GET app:k", cfg)
	if err == nil || !strings.Contains(err.Error(), "不是变更命令") {
		t.Errorf("变更模式下只读命令应报「不是变更命令」, got %v", err)
	}
}

func TestLevelFromRuleLevels(t *testing.T) {
	cfg := allOn()
	line := "FLUSHALL"

	// 未配置级别：用规则默认（安全类 error，拦截）
	_, findings, err := Check(line, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if Blocked(findings) == nil {
		t.Error("默认应拦截")
	}

	// 规则集里调成 warn：仍然命中，但不再阻断
	cfg.Levels = map[string]string{"RedisForbidDangerous": "warn"}
	_, findings, err = Check(line, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if Blocked(findings) != nil {
		t.Error("warn 级不该拦截")
	}
	if len(findings) != 1 || findings[0].Level != LevelWarn {
		t.Errorf("应有一条 warn 命中, got %+v", findings)
	}
	if Severity(findings) != 2 {
		t.Errorf("Severity = %d, want 2", Severity(findings))
	}

	// 非法级别值回落到默认
	cfg.Levels = map[string]string{"RedisForbidDangerous": "bogus"}
	if _, findings, _ := Check(line, cfg); Blocked(findings) == nil {
		t.Error("非法级别应回落为规则默认级别（error）")
	}
}

func TestSwitchesOffMeansNoFinding(t *testing.T) {
	// 开关全关：什么都不拦（规则集没配就是关，与 Mongo/SQL 规则一致）
	cfg := Config{}
	for _, line := range []string{"FLUSHALL", "CONFIG SET a b", "KEYS *", `EVAL "return 1" 0`, "BLPOP q 0"} {
		if _, findings, err := Check(line, cfg); err != nil {
			t.Errorf("Check(%q) 不该报用法错误: %v", line, err)
		} else if len(findings) != 0 {
			t.Errorf("开关全关时 Check(%q) 不该命中, got %+v", line, findings)
		}
	}
}

func TestConfigFromAPIFillsSwitches(t *testing.T) {
	role := &api.AuditRole{
		RedisForbidDangerous: true,
		RedisMaxAffectKeys:   100,
		RedisKeyPrefix:       "app:",
		RedisRequireTtl:      true,
		RuleLevels:           map[string]string{"RedisForbidDangerous": "warn"},
	}
	cfg := ConfigFromAPI(role, ModeQuery)

	if !cfg.ForbidDangerous {
		t.Error("redis_forbid_dangerous 未映射到 ForbidDangerous")
	}
	if cfg.MaxAffectKeys != 100 {
		t.Errorf("redis_max_affect_keys 未映射, got %d", cfg.MaxAffectKeys)
	}
	if cfg.KeyPrefix != "app:" {
		t.Errorf("redis_key_prefix 未映射, got %q", cfg.KeyPrefix)
	}
	if !cfg.RequireTTL {
		t.Error("redis_require_ttl 未映射到 RequireTTL")
	}
	if cfg.Mode != ModeQuery {
		t.Errorf("Mode 未设置, got %q", cfg.Mode)
	}
	// 没开的开关不应被误置为 true
	if cfg.ForbidAdmin {
		t.Error("未配置的开关不该为 true")
	}
	if l := cfg.Level(forbidDangerous{}); l != LevelWarn {
		t.Errorf("RuleLevels 未生效, got %s", l)
	}
}

// 每个 AuditRole 里的 Redis 开关都必须能在 Config 里找到对应字段
// （archguard 里有全局版本，这里保证本包自身可用）
func TestConfigCoversContractSwitches(t *testing.T) {
	st := reflect.TypeOf(api.AuditRole{})
	found := 0
	for i := 0; i < st.NumField(); i++ {
		name := st.Field(i).Name
		if !strings.HasPrefix(name, "Redis") {
			continue
		}
		found++
		if !(Config{}).Has(name) {
			t.Errorf("契约字段 %s 在 Config 里没有对应字段（会导致该开关永远不生效）", name)
		}
	}
	if found == 0 {
		t.Fatal("AuditRole 里没有任何 Redis 字段，契约包是否已同步？")
	}
}
