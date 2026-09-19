package customrules

import "testing"

func TestForbidTruncate(t *testing.T) {
	rule := forbidTruncate{}
	cases := []struct {
		name string
		sql  string
		cfg  Config
		want bool
	}{
		{"开关开启时命中", "truncate table t", Config{ForbidTruncate: true}, true},
		{"大小写与多余空白也命中", "  TRUNCATE   TABLE  t;", Config{ForbidTruncate: true}, true},
		{"开关关闭时不命中（默认不影响既有行为）", "truncate table t", Config{}, false},
		{"同前缀标识符不误伤", "truncate_log_insert", Config{ForbidTruncate: true}, false},
		{"无关语句不命中", "delete from t", Config{ForbidTruncate: true}, false},
	}
	for _, c := range cases {
		if got := len(rule.Check(c.cfg, Context{SQL: c.sql})) > 0; got != c.want {
			t.Errorf("%s: Check(%q, cfg=%+v) = %v，期望 %v", c.name, c.sql, c.cfg, got, c.want)
		}
	}
	if rule.Name() != "forbid-truncate" {
		t.Errorf("规则名被改动: %s", rule.Name())
	}
}
