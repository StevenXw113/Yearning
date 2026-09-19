package roles

import (
	"testing"

	"Yearning-go/src/engine"
)

func TestShouldRejectEmptyRuleSet(t *testing.T) {
	full := engine.AuditRole{DMLWhere: true, DMLMaxInsertRows: 10}
	zero := engine.AuditRole{}
	partial := engine.AuditRole{DMLMaxInsertRows: 10}

	cases := []struct {
		name    string
		before  *engine.AuditRole
		after   *engine.AuditRole
		confirm bool
		want    bool
	}{
		{"空载荷清空规则集应拒绝", &full, &zero, false, true},
		{"空载荷但显式 confirm 应放行", &full, &zero, true, false},
		{"普通改动应放行", &full, &partial, false, false},
		{"本来就是空集应放行", &zero, &zero, false, false},
		{"读不到旧值(规则集不存在)应放行", nil, &zero, false, false},
	}
	for _, c := range cases {
		if got := shouldRejectEmptyRuleSet(c.before, c.after, c.confirm); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
