package server

import (
	"context"
	"strings"
	"testing"

	enginev1 "engine/gen/engine/v1"
)

// TestAffectRowOverLimit 规则集影响行数上限的判定：DML 用 MaxAffectRows、
// DDL 用 MaxDdlAffectRows，未配置（0）不限制。
func TestAffectRowOverLimit(t *testing.T) {
	rules := &enginev1.AuditRole{MaxAffectRows: 10, MaxDdlAffectRows: 100}
	cases := []struct {
		rules *enginev1.AuditRole
		tp    string
		rows  uint32
		over  bool
	}{
		{rules, "dml", 10, false},          // 等于上限不拦
		{rules, "dml", 11, true},           // 超过拦截
		{rules, "ddl", 101, true},          // DDL 用另一档
		{nil, "dml", 999999, false},        // 无规则集不拦
		{&enginev1.AuditRole{}, "dml", 1, false}, // 未配置（0）不拦
	}
	for _, c := range cases {
		if over, _ := affectRowOverLimit(c.rules, c.tp, c.rows); over != c.over {
			t.Errorf("affectRowOverLimit(%+v, %s, %d) = %v, want %v", c.rules, c.tp, c.rows, over, c.over)
		}
	}
}

// TestCheckAffectRowLimitOnEstimate 审核期比对：MySQL 的 INSERT 估算行数
// （VALUES 元组数，无需连库）超过 MaxAffectRows 时判「审核不通过」。
func TestCheckAffectRowLimitOnEstimate(t *testing.T) {
	sql := "INSERT INTO t (id, name) VALUES (1,'a'),(2,'b'),(3,'c')"
	rep, err := testEngine().Check(context.Background(), &enginev1.CheckRequest{
		Sql:  sql,
		Rule: &enginev1.AuditRole{MaxAffectRows: 2},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	r := rep.Records[0]
	if r.AffectRows != 3 {
		t.Fatalf("估算行数应为 3: %+v", r)
	}
	if r.Level != 1 || !strings.Contains(r.Error, "超过规则集上限") {
		t.Fatalf("超限应判审核不通过: %+v", r)
	}

	// 未配置上限：估算行数照记，但不拦截
	rep2, _ := testEngine().Check(context.Background(), &enginev1.CheckRequest{
		Sql:  sql,
		Rule: &enginev1.AuditRole{},
	})
	if r2 := rep2.Records[0]; r2.AffectRows != 3 || r2.Level != 0 || r2.Error != "" {
		t.Fatalf("未配置上限应通过且保留估算行数: %+v", r2)
	}
}
