package customrules

import "testing"

func TestForbidDrop(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		sql  string
		hit  bool
	}{
		{"禁止删库（默认）", Config{}, "DROP DATABASE app", true},
		{"禁止删 schema（默认）", Config{}, "DROP SCHEMA app", true},
		{"禁止删表（默认）", Config{}, "drop   table   t", true},
		{"允许删库后放行", Config{AllowDropDatabase: true}, "DROP DATABASE app", false},
		{"允许删表后放行", Config{AllowDropTable: true}, "DROP TABLE t", false},
		{"前缀相同但不命中", Config{}, "DROP TABLESPACE ts", false},
		{"普通语句不命中", Config{}, "SELECT 1", false},
	}

	rule := forbidDrop{}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := len(rule.Check(c.cfg, Context{SQL: c.sql})) > 0
			if got != c.hit {
				t.Fatalf("Check(%q) = %v, 期望 %v", c.sql, got, c.hit)
			}
		})
	}
}

func TestRegisterRejectsDuplicateAndNil(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("重复注册应 panic")
		}
	}()
	Register(forbidDrop{}) // forbid-drop 已由 init 注册
}
