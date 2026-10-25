package roles

import (
	"testing"

	"Yearning-go/src/engine"
	"Yearning-go/src/model"
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

// TestMergeRuleSetRefs 规则集列表引用可见:把 core_data_sources 的引用行
// 聚合进规则集列表(引用数 + 引用方名单),让管理员改规则集前看清影响面。
func TestMergeRuleSetRefs(t *testing.T) {
	rules := []model.CoreRules{
		{ID: 1, Desc: "严格模式"},
		{ID: 2, Desc: "宽松模式"},
	}

	t.Run("引用聚合:计数与名单", func(t *testing.T) {
		rows := []ruleSetRefRow{
			{RuleID: 1, Source: "prod-a"},
			{RuleID: 1, Source: "prod-b"},
			{RuleID: 2, Source: "dev"},
		}
		got := mergeRuleSetRefs(rules, rows)
		if got[0].RefCount != 2 || got[0].RefSources[0] != "prod-a" || got[0].RefSources[1] != "prod-b" {
			t.Errorf("规则集1引用聚合错误: %d %v", got[0].RefCount, got[0].RefSources)
		}
		if got[1].RefCount != 1 {
			t.Errorf("规则集2引用数错误: %d", got[1].RefCount)
		}
	})

	t.Run("无引用:计数为0,名单是空数组而非null", func(t *testing.T) {
		got := mergeRuleSetRefs(rules, nil)
		if got[0].RefCount != 0 {
			t.Errorf("应无引用: %d", got[0].RefCount)
		}
		if got[0].RefSources == nil || len(got[0].RefSources) != 0 {
			t.Errorf("RefSources 应为空数组(序列化成[]而非null): %v", got[0].RefSources)
		}
	})

	t.Run("悬空引用(指向已删规则集)忽略,输出保持规则集原顺序", func(t *testing.T) {
		rows := []ruleSetRefRow{
			{RuleID: 99, Source: "ghost"},
			{RuleID: 2, Source: "dev"},
		}
		got := mergeRuleSetRefs(rules, rows)
		if got[0].ID != 1 || got[1].ID != 2 {
			t.Fatalf("输出应保持输入顺序: %v %v", got[0].ID, got[1].ID)
		}
		if got[0].RefCount != 0 {
			t.Errorf("悬空引用不应计入: %d", got[0].RefCount)
		}
		if got[1].RefCount != 1 {
			t.Errorf("有效引用应计入: %d", got[1].RefCount)
		}
	})

	t.Run("空规则集返回空列表", func(t *testing.T) {
		got := mergeRuleSetRefs(nil, []ruleSetRefRow{{RuleID: 1, Source: "x"}})
		if len(got) != 0 {
			t.Errorf("应返回空列表: %v", got)
		}
	})
}
