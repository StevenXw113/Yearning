package redischeck

import (
	"reflect"
	"strings"
	"testing"

	"engine/api"
)

// allOn 打开全部开关，用于验证规则本身；阈值取一个宽松值。
// Key 规范类配置（前缀 / 保护键 / 敏感键 / DB 范围）留空，避免干扰其他用例。
func allOn() Config {
	return Config{
		CommandSyntax:           true,
		UnknownCommand:          true,
		KeyExtraction:           true,
		KeyPrefix:               "",
		MaxKeyLen:               256,
		ForbiddenKeyPattern:     "",
		SensitiveKeyAccess:      "",
		DbScope:                 "",
		ForbidConnectionControl: true,

		ForbidDangerous:     true,
		ForbidAdmin:         true,
		ForbidDebug:         true,
		ForbidMonitor:       true,
		ForbidAclChange:     true,
		ForbidModuleChange:  true,
		ForbidReplica:       true,
		ForbidClusterChange: true,
		ForbidPersistence:   true,
		ForbidMigration:     true,
		ForbidMove:          true,

		ForbidKeysCommand:  true,
		ForbidRandomKey:    true,
		ScanMatchRequired:  true,
		MaxScanCount:       1000,
		FullCollectionRead: true,
		MaxReadElements:    1000,
		MaxReadKeys:        100,
		ExpensiveRead:      true,

		MaxValueSize:       1 << 20,
		MaxCommandPayload:  8 << 20,
		MaxWriteKeys:       100,
		MaxWriteElements:   1000,
		NonIdempotentWrite: true,
		LargeOffsetWrite:   1 << 20,
		ForbidRename:       true,
		CopyOverwrite:      true,
		StoreOperation:     true,

		RequireTTL:         true,
		TtlLossOnOverwrite: false,
		ForbidPersist:      true,
		MinTTL:             1,
		MaxTTL:             0,
		ImmediateExpire:    true,

		DeleteKey:          true,
		MaxDeleteKeys:      100,
		ProtectedKeyDelete: "",
		LargeRangeDelete:   true,

		ForbidScript:           true,
		ForbidScriptManagement: true,
		ForbidWatch:            true,
		ForbidBlockingCommand:  true,
		ForbidSubscription:     true,
		StreamStateChange:      true,
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
		// 每条规则都要有对应的 Config 字段，否则开关永远读不到
		if !(Config{}).Has(r.Name()) {
			t.Errorf("规则 %s 在 Config 里没有同名字段：开关会变成「勾了不生效」", r.Name())
		}
	}
	if len(all) < 50 {
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
		"SADD app:s a b",
	}
	for _, line := range allow {
		if _, findings, err := Check(line, cfg); err != nil {
			t.Errorf("Check(%q) 不该报错: %v", line, err)
		} else if Blocked(findings) != nil {
			t.Errorf("Check(%q) 不该被拦截: %+v", line, Blocked(findings))
		}
	}

	block := map[string]string{
		"FLUSHALL":                  "FLUSHALL",
		"FLUSHDB":                   "FLUSHDB",
		"SHUTDOWN NOSAVE":           "SHUTDOWN",
		"CONFIG SET maxmemory 0":    "CONFIG",
		"DEBUG SLEEP 1":             "DEBUG",
		`EVAL "return 1" 0`:         "EVAL",
		"REPLICAOF NO ONE":          "REPLICAOF",
		"KEYS *":                    "KEYS",
		"ACL SETUSER u on":          "ACL",
		"MODULE LOAD /x.so":         "MODULE",
		"CLUSTER SETSLOT 1 NODE x":  "CLUSTER",
		"SAVE":                      "SAVE",
		"MIGRATE h 6379 key 0 1000": "MIGRATE",
		"RESTORE k 0 payload":       "RESTORE",
		"AUTH secret":               "AUTH",
		"MONITOR":                   "MONITOR",
		"SUBSCRIBE ch":              "SUBSCRIBE",
		"BLPOP q 30":                "BLPOP",
		"WATCH k":                   "WATCH",
		"MOVE k 1":                  "MOVE",
		"RENAME a b":                "RENAME",
	}
	for line, want := range block {
		_, findings, err := Check(line, cfg)
		if err != nil {
			// 危险/管理命令被判为 KindDDL，不应落到「不是变更命令」
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

// 只读子命令不该被管理类规则一刀切拦下
func TestReadOnlySubcommandsAllowed(t *testing.T) {
	cfg := allOn()
	for _, line := range []string{"CONFIG GET maxmemory", "CLIENT LIST", "ACL LIST", "MODULE LIST", "CLUSTER INFO", "SCRIPT EXISTS abc"} {
		_, findings, err := Check(line, cfg)
		if err != nil {
			t.Errorf("Check(%q) 不该报用法错误: %v", line, err)
			continue
		}
		if hit := Blocked(findings); hit != nil {
			t.Errorf("Check(%q) 不该被拦截: %+v", line, hit)
		}
	}
}

func TestCommandSyntax(t *testing.T) {
	cfg := Config{CommandSyntax: true}
	bad := []string{
		"SET app:k",
		"SET app:k v NX XX",
		"SET app:k v EX 60 KEEPTTL",
		"HSET app:h f",
		"MSET app:k1 v1 app:k2",
		"EXPIRE app:k",
	}
	for _, line := range bad {
		_, findings, err := Check(line, cfg)
		if err != nil {
			t.Errorf("Check(%q) 不该报用法错误: %v", line, err)
			continue
		}
		if Blocked(findings) == nil {
			t.Errorf("Check(%q) 应被语法规则拦截", line)
		}
	}
	for _, line := range []string{"SET app:k v", "HSET app:h f v", "MSET app:k1 v1 app:k2 v2"} {
		if _, findings, _ := Check(line, cfg); Blocked(findings) != nil {
			t.Errorf("Check(%q) 语法合法，不该拦截", line)
		}
	}
}

func TestUnknownCommand(t *testing.T) {
	cfg := Config{UnknownCommand: true}
	for _, line := range []string{"JSON.SET app:k $ {}", "BF.ADD app:k v", "FT.SEARCH idx *"} {
		_, findings, _ := Check(line, cfg)
		if Blocked(findings) == nil {
			t.Errorf("Check(%q) 应被 RedisUnknownCommand 拦截", line)
		}
	}
	if _, findings, _ := Check("SET app:k v", cfg); Blocked(findings) != nil {
		t.Error("已登记的命令不该被拦截")
	}
}

func TestKeyExtraction(t *testing.T) {
	// 源键与目标键都要识别出来
	cases := map[string][]string{
		"RENAME a b":            {"a", "b"},
		"COPY a b":              {"a", "b"},
		"MSET k1 v1 k2 v2":      {"k1", "k2"},
		"GET app:k":             {"app:k"},
		"ZUNIONSTORE out 2 a b": {"out", "a", "b"},
		"BITOP AND dest a b":    {"dest", "a", "b"},
	}
	for line, want := range cases {
		cmd, err := Parse(line)
		if err != nil {
			t.Fatalf("Parse(%q): %v", line, err)
		}
		got := map[string]bool{}
		for _, k := range cmd.Keys {
			got[k] = true
		}
		for _, w := range want {
			if !got[w] {
				t.Errorf("Parse(%q) 未识别出键 %q, got %v", line, w, cmd.Keys)
			}
		}
		if !cmd.KeysKnown {
			t.Errorf("Parse(%q) 应能识别键", line)
		}
	}

	// 未登记语义的命令无法识别键
	cmd, err := Parse("JSON.SET app:k $ {}")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.KeysKnown {
		t.Error("未登记命令的 KeysKnown 应为 false")
	}
}

func TestKeyPrefixChecksTargetKeys(t *testing.T) {
	cfg := Config{KeyPrefix: "app:", Mode: ModeWrite}
	_, findings, _ := Check("RENAME app:x other:y", cfg)
	hit := Blocked(findings)
	if hit == nil {
		t.Fatal("目标键不在前缀范围内应被拦截")
	}
	if !strings.Contains(hit.Message, "other:y") {
		t.Errorf("提示应指出目标键, got %s", hit.Message)
	}

	if _, findings, _ := Check("RENAME app:x app:y", cfg); Blocked(findings) != nil {
		t.Error("源键与目标键都在前缀内，不该拦截")
	}
}

func TestKeyPatternRules(t *testing.T) {
	cfg := Config{
		ForbiddenKeyPattern: "sys:*, lock:*",
		SensitiveKeyAccess:  "*token*, *secret*",
		Mode:                ModeWrite,
	}
	_, findings, _ := Check("DEL lock:order:1", cfg)
	if f := Blocked(findings); f == nil || f.Rule != "RedisForbiddenKeyPattern" {
		t.Errorf("受保护键应被拦截, got %+v", findings)
	}
	_, findings, _ = Check("SET app:token:1 v", cfg)
	if !hasFinding(findings, "RedisSensitiveKeyAccess", LevelWarn) {
		t.Errorf("敏感键应给出提示, got %+v", findings)
	}
}

func TestDbScope(t *testing.T) {
	cfg := Config{DbScope: "0-3", Mode: ModeWrite}
	_, findings, _ := Check("SELECT 5", cfg)
	if Blocked(findings) == nil {
		t.Error("超出 DB 范围应被拦截")
	}
	if _, findings, _ := Check("SELECT 2", cfg); Blocked(findings) != nil {
		t.Error("范围内不该拦截")
	}
}

func TestTtlRules(t *testing.T) {
	cfg := Config{MinTTL: 60, MaxTTL: 3600, ImmediateExpire: true, Mode: ModeWrite}

	_, findings, _ := Check("SET app:k v EX 30", cfg)
	if !hasFinding(findings, "RedisMinTtl", LevelWarn) {
		t.Errorf("TTL 过小应提示, got %+v", findings)
	}
	_, findings, _ = Check("SET app:k v EX 86400", cfg)
	if !hasFinding(findings, "RedisMaxTtl", LevelWarn) {
		t.Errorf("TTL 过大应提示, got %+v", findings)
	}
	_, findings, _ = Check("EXPIRE app:k 0", cfg)
	if !hasFinding(findings, "RedisImmediateExpire", LevelWarn) {
		t.Errorf("立即过期应提示, got %+v", findings)
	}
	// value 恰好叫 EX 时不能把 value 当选项
	if _, findings, _ = Check("SET app:k EX", cfg); hasFinding(findings, "RedisMaxTtl", LevelWarn) {
		t.Error("value 是 EX 时不该被当作过期选项")
	}
}

func TestWriteLimitRules(t *testing.T) {
	cfg := Config{MaxWriteKeys: 2, MaxDeleteKeys: 2, MaxWriteElements: 2, Mode: ModeWrite}

	_, findings, _ := Check("DEL a b c", cfg)
	if !hasFinding(findings, "RedisMaxDeleteKeys", LevelError) {
		t.Errorf("删除键数超限应拦截, got %+v", findings)
	}
	_, findings, _ = Check("MSET a 1 b 2 c 3", cfg)
	if !hasFinding(findings, "RedisMaxWriteKeys", LevelError) {
		t.Errorf("写入键数超限应拦截, got %+v", findings)
	}
	_, findings, _ = Check("HSET h f1 v1 f2 v2 f3 v3", cfg)
	if !hasFinding(findings, "RedisMaxWriteElements", LevelError) {
		t.Errorf("写入元素数超限应拦截, got %+v", findings)
	}
}

func TestConfigFromAPIFillsSwitches(t *testing.T) {
	role := &api.AuditRole{
		RedisForbidDangerous:     true,
		RedisMaxWriteKeys:        100,
		RedisKeyPrefix:           "app:",
		RedisRequireTtl:          true,
		RedisDbScope:             "0-7",
		RedisForbiddenKeyPattern: "sys:*",
		RuleLevels:               map[string]string{"RedisForbidDangerous": "warn"},
	}
	cfg := ConfigFromAPI(role, ModeQuery)

	if !cfg.ForbidDangerous {
		t.Error("redis_forbid_dangerous 未映射到 ForbidDangerous")
	}
	if cfg.MaxWriteKeys != 100 {
		t.Errorf("redis_max_write_keys 未映射, got %d", cfg.MaxWriteKeys)
	}
	if cfg.KeyPrefix != "app:" {
		t.Errorf("redis_key_prefix 未映射, got %q", cfg.KeyPrefix)
	}
	if !cfg.RequireTTL {
		t.Error("redis_require_ttl 未映射到 RequireTTL")
	}
	if cfg.DbScope != "0-7" || cfg.ForbiddenKeyPattern != "sys:*" {
		t.Errorf("字符串类开关未映射, got %+v", cfg)
	}
	if cfg.Mode != ModeQuery {
		t.Errorf("Mode 未设置, got %q", cfg.Mode)
	}
	if cfg.ForbidAdmin {
		t.Error("未配置的开关不该为 true")
	}
	if l := cfg.Level(commandRule{name: "RedisForbidDangerous", level: LevelError}); l != LevelWarn {
		t.Errorf("RuleLevels 未生效, got %s", l)
	}
}

func TestLevelFromRuleLevels(t *testing.T) {
	cfg := allOn()
	line := "FLUSHALL"

	_, findings, err := Check(line, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if Blocked(findings) == nil {
		t.Error("默认应拦截")
	}

	cfg.Levels = map[string]string{"RedisForbidDangerous": "warn"}
	_, findings, err = Check(line, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if Blocked(findings) != nil {
		t.Error("warn 级不该拦截")
	}
	if Severity(findings) != 2 {
		t.Errorf("Severity = %d, want 2", Severity(findings))
	}

	cfg.Levels = map[string]string{"RedisForbidDangerous": "bogus"}
	if _, findings, _ := Check(line, cfg); Blocked(findings) == nil {
		t.Error("非法级别应回落为规则默认级别（error）")
	}
}

func TestSwitchesOffMeansNoFinding(t *testing.T) {
	cfg := Config{}
	for _, line := range []string{"FLUSHALL", "CONFIG SET a b", "KEYS *", `EVAL "return 1" 0`, "BLPOP q 0", "JSON.SET k $ v"} {
		if _, findings, err := Check(line, cfg); err != nil {
			t.Errorf("Check(%q) 不该报用法错误: %v", line, err)
		} else if len(findings) != 0 {
			t.Errorf("开关全关时 Check(%q) 不该命中, got %+v", line, findings)
		}
	}
}

func TestCheckQueryMode(t *testing.T) {
	cfg := allOn()
	cfg.Mode = ModeQuery

	for _, line := range []string{"GET app:k", "SCAN 0 MATCH app:* COUNT 10", "HGETALL app:h"} {
		if _, _, err := Check(line, cfg); err != nil {
			t.Errorf("查询模式 Check(%q) 不该报错: %v", line, err)
		}
	}
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
	_, _, err := Check("GET app:k", cfg)
	if err == nil || !strings.Contains(err.Error(), "不是变更命令") {
		t.Errorf("变更模式下只读命令应报「不是变更命令」, got %v", err)
	}
}

// 每个 AuditRole 里的 Redis 开关都必须能在 Config 里找到对应字段
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

// hasFinding 是否命中指定规则且级别一致
func hasFinding(findings []Finding, rule string, level Level) bool {
	for _, f := range findings {
		if f.Rule == rule && f.Level == level {
			return true
		}
	}
	return false
}

// 多行命令必须显式拒绝：换行在切词里等同空格，第二条会被吞成参数
func TestRedisParseRejectsMultiLine(t *testing.T) {
	for _, line := range []string{"SET a 1\nSET b 2", "SET a 1\r\nDEL a"} {
		if _, err := Parse(line); err == nil {
			t.Errorf("多行命令应被拒绝: %q", line)
		}
	}
	if _, err := Parse("SET a 1 EX 60"); err != nil {
		t.Errorf("单行命令不应报错: %v", err)
	}
}

// OBJECT / MEMORY 缺键时不能切 args 造成越界 panic
func TestRedisKeyExtractionWithoutKey(t *testing.T) {
	for _, line := range []string{"OBJECT ENCODING", "MEMORY USAGE"} {
		cmd, err := Parse(line)
		if err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		if cmd.KeysKnown {
			t.Errorf("%s 缺键时应按「键未知」处理", line)
		}
	}
	cmd, err := Parse("OBJECT ENCODING app:k")
	if err != nil {
		t.Fatalf("带键的 OBJECT 不应报错: %v", err)
	}
	if !cmd.KeysKnown || len(cmd.Keys) != 1 || cmd.Keys[0] != "app:k" {
		t.Errorf("带键时应识别出键: %+v", cmd)
	}
}

// SCAN 的 MATCH 必须限定业务前缀：MATCH * 等同全量遍历
func TestRedisScanMatchRequiredScope(t *testing.T) {
	cfg := Config{ScanMatchRequired: true, Mode: ModeQuery}
	blocked := func(line string) bool {
		_, findings, err := Check(line, cfg)
		if err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		for _, f := range findings {
			if f.Rule == "RedisScanMatchRequired" {
				return true
			}
		}
		return false
	}
	for _, line := range []string{"SCAN 0", "SCAN 0 COUNT 10", "SCAN 0 MATCH *", "HSCAN app:h 0 MATCH *"} {
		if !blocked(line) {
			t.Errorf("%q 未限定业务前缀，应被拦截", line)
		}
	}
	for _, line := range []string{"SCAN 0 MATCH app:*", "HSCAN app:h 0 MATCH field:*"} {
		if blocked(line) {
			t.Errorf("%q 已限定前缀，不应拦截", line)
		}
	}
}

// 单值大小只看「值」，不能把键名算进去（MSET 长键短值不应误报）
func TestRedisMaxValueSizeIgnoresKeys(t *testing.T) {
	cfg := Config{MaxValueSize: 8, Mode: ModeWrite}
	blocked := func(line string) bool {
		_, findings, err := Check(line, cfg)
		if err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		for _, f := range findings {
			if f.Rule == "RedisMaxValueSize" {
				return true
			}
		}
		return false
	}
	if blocked("MSET verylongkeyname short") {
		t.Error("MSET 的键名长于值上限时不应命中")
	}
	if !blocked("MSET k verylongvalue123") {
		t.Error("值超过上限时应命中")
	}
	if !blocked("SET k verylongvalue123") {
		t.Error("SET 的值超过上限时应命中")
	}
	if blocked("SET verylongkeyname v") {
		t.Error("SET 的键名不应被当成值")
	}
}

// DB 范围配置非法时必须拦截，不能静默放行
func TestRedisDbScopeInvalidConfig(t *testing.T) {
	cfg := Config{DbScope: "abc", Mode: ModeWrite}
	_, findings, err := Check("SELECT 3", cfg)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	found := false
	for _, f := range findings {
		if f.Rule == "RedisDbScope" {
			found = true
		}
	}
	if !found {
		t.Error("DbScope 配置非法时应拦截")
	}

	ok := Config{DbScope: "0-15", Mode: ModeWrite}
	_, findings, err = Check("SELECT 3", ok)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, f := range findings {
		if f.Rule == "RedisDbScope" {
			t.Errorf("合法范围内不应拦截: %s", f.Message)
		}
	}
}
