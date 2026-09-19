package server

import (
	"context"
	"strings"
	"testing"

	enginev1 "engine/gen/engine/v1"

	storepb "engine/internal/bytebase/generated-go/store"
)

func testEngine() *Engine { return NewEngine() }

func TestCheckSyntaxError(t *testing.T) {
	rep, err := testEngine().Check(context.Background(), &enginev1.CheckRequest{
		Sql:  "SELEC 1 FROM",
		Rule: &enginev1.AuditRole{},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !rep.Ok {
		t.Fatalf("ok=false")
	}
	if len(rep.Records) != 1 || !strings.Contains(rep.Records[0].Error, "语法错误") {
		t.Fatalf("expected syntax error record, got %+v", rep.Records)
	}
}

func TestCheckMultiStatementSplit(t *testing.T) {
	rep, err := testEngine().Check(context.Background(), &enginev1.CheckRequest{
		Sql:  "SELECT 1; SELECT 2; SELECT 3",
		Rule: &enginev1.AuditRole{},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(rep.Records) != 3 {
		t.Fatalf("expected 3 records, got %d: %+v", len(rep.Records), rep.Records)
	}
	for _, r := range rep.Records {
		// Yearning 约定：0=通过，1=错误，2=警告
		if r.Error != "" || r.Level != 0 {
			t.Fatalf("legal select should pass with level 0, got %+v", r)
		}
	}
}

func TestCheckDropWhenForbidden(t *testing.T) {
	// DDLEnableDropTable 语义为“允许删除表”，未开启即禁止。
	rep, _ := testEngine().Check(context.Background(), &enginev1.CheckRequest{
		Sql:  "DROP TABLE users",
		Rule: &enginev1.AuditRole{},
	})
	if rep.Records[0].Error == "" {
		t.Fatalf("expected drop blocked, got %+v", rep.Records[0])
	}
	// 开启后放行。
	rep2, _ := testEngine().Check(context.Background(), &enginev1.CheckRequest{
		Sql:  "DROP TABLE users",
		Rule: &enginev1.AuditRole{DdlEnableDropTable: true},
	})
	if rep2.Records[0].Error != "" {
		t.Fatalf("expected drop allowed, got %+v", rep2.Records[0])
	}
}

func TestCheckCreateTableRequirePK(t *testing.T) {
	sql := "CREATE TABLE t (id INT, name VARCHAR(10))"
	rep, _ := testEngine().Check(context.Background(), &enginev1.CheckRequest{
		Sql:  sql,
		Rule: &enginev1.AuditRole{DdlEnablePrimaryKey: true},
	})
	if !strings.Contains(rep.Records[0].Error, "TABLE_REQUIRE_PK") {
		t.Fatalf("expected pk rule hit, got %+v", rep.Records[0])
	}

	rep2, _ := testEngine().Check(context.Background(), &enginev1.CheckRequest{
		Sql:  "CREATE TABLE t (id INT PRIMARY KEY, name VARCHAR(10))",
		Rule: &enginev1.AuditRole{DdlEnablePrimaryKey: true},
	})
	if rep2.Records[0].Error != "" {
		t.Fatalf("expected pass with pk, got %+v", rep2.Records[0])
	}
}

// TestCheckDDLNotPanicOnMetadataRules 回归：COLUMN_NO_NULL / INDEX_TOTAL_NUMBER_LIMIT
// 依赖 advisor.Context.FinalMetadata（库内元数据快照），引擎不提供该快照。
// 这两个开关开启时，规则内曾 nil 解引用 panic，被 advisor 兜住后误报成「SQL 语法错误」，
// 导致所有 DDL 审核都失败。
func TestCheckDDLNotPanicOnMetadataRules(t *testing.T) {
	rep, err := testEngine().Check(context.Background(), &enginev1.CheckRequest{
		Sql: "CREATE TABLE t (id INT PRIMARY KEY, name VARCHAR(20))",
		Rule: &enginev1.AuditRole{
			DdlMaxKey:              5,
			DdlCheckColumnNullable: true,
		},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(rep.Records) != 1 {
		t.Fatalf("expected 1 record, got %+v", rep.Records)
	}
	if r := rep.Records[0]; r.Error != "" || r.Level != 0 {
		t.Fatalf("DDL 审核不应 panic 或报语法错误，got %+v", r)
	}
}

func TestCheckTableNamingPrefix(t *testing.T) {
	rep, _ := testEngine().Check(context.Background(), &enginev1.CheckRequest{
		Sql:  "CREATE TABLE bad_name (id INT PRIMARY KEY)",
		Rule: &enginev1.AuditRole{DdlTablePrefix: "app_"},
	})
	if !strings.Contains(rep.Records[0].Error, "NAMING_TABLE") {
		t.Fatalf("expected naming rule hit, got %+v", rep.Records[0])
	}
}

func TestCheckUpdateWithoutWhere(t *testing.T) {
	// 规则要求 DML 必须有 where
	rep, _ := testEngine().Check(context.Background(), &enginev1.CheckRequest{
		Sql:  "UPDATE users SET name='x'",
		Rule: &enginev1.AuditRole{DmlWhere: true},
	})
	if !strings.Contains(rep.Records[0].Error, "WHERE") {
		t.Fatalf("expected no-where blocked, got %+v", rep.Records[0])
	}

	// 合法 update 应放行
	rep2, _ := testEngine().Check(context.Background(), &enginev1.CheckRequest{
		Sql:  "UPDATE users SET name='x' WHERE id=1",
		Rule: &enginev1.AuditRole{DmlWhere: true},
	})
	if rep2.Records[0].Error != "" {
		t.Fatalf("expected pass with where, got %+v", rep2.Records[0])
	}
}

func TestCheckDeleteWithoutWhere(t *testing.T) {
	rep, _ := testEngine().Check(context.Background(), &enginev1.CheckRequest{
		Sql:  "DELETE FROM orders",
		Rule: &enginev1.AuditRole{DmlWhere: true},
	})
	if !strings.Contains(rep.Records[0].Error, "WHERE") {
		t.Fatalf("expected delete blocked, got %+v", rep.Records[0])
	}
}

// 规则级别：默认 error 拦截，warn/observe 只提示/记录（都不拦提交）。
func TestReviewRulesLevelGroups(t *testing.T) {
	role := &enginev1.AuditRole{
		DmlWhere:            true,
		DmlOrder:            true,
		DdlEnablePrimaryKey: true,
		RuleLevels: map[string]string{
			"DMLOrder":            "warn",
			"DDLEnablePrimaryKey": "observe",
		},
	}
	plan := reviewRules(role)
	has := func(rules []*storepb.SQLReviewRule, want storepb.SQLReviewRule_Type) bool {
		for _, r := range rules {
			if r.GetType() == want {
				return true
			}
		}
		return false
	}
	if !has(plan.error, storepb.SQLReviewRule_STATEMENT_WHERE_REQUIRE_UPDATE_DELETE) {
		t.Errorf("未配置级别的规则应归入 error 组: %+v", plan.error)
	}
	if has(plan.error, storepb.SQLReviewRule_STATEMENT_DISALLOW_ORDER_BY) || !has(plan.warning, storepb.SQLReviewRule_STATEMENT_DISALLOW_ORDER_BY) {
		t.Error("warn 级别的 DMLOrder 应只出现在 warning 组")
	}
	if !has(plan.observe, storepb.SQLReviewRule_TABLE_REQUIRE_PK) {
		t.Error("observe 级别的 DDLEnablePrimaryKey 应出现在 observe 组")
	}
	// warn/observe 必须映射成 Bytebase 的 WARNING，否则引擎自己先把它当错误拦下
	for _, r := range append(plan.warning, plan.observe...) {
		if r.GetLevel() != storepb.SQLReviewRule_WARNING {
			t.Errorf("warn/observe 规则应报 WARNING，实际 %v", r.GetLevel())
		}
	}
	for _, k := range []string{"DMLWhere", "NoSuchKey", ""} {
		if got := ruleLevel(role, k); got != ruleLevelError {
			t.Errorf("ruleLevel(%q)=%q，未配置时应为 error", k, got)
		}
	}
}

// 端到端：warn 不影响通过，error 才判不通过。
func TestCheckWarnDoesNotBlock(t *testing.T) {
	rep, _ := testEngine().Check(context.Background(), &enginev1.CheckRequest{
		Sql: "DELETE FROM orders",
		Rule: &enginev1.AuditRole{
			DmlWhere:   true,
			RuleLevels: map[string]string{"DMLWhere": "warn"},
		},
	})
	rec := rep.Records[0]
	if rec.Level != 2 || rec.Status != "警告" {
		t.Fatalf("warn 级别应记为 level=2/警告，实际 %+v", rec)
	}
	if rec.Error == "" {
		t.Fatal("warn 命中仍应给出提示文案")
	}

	rep2, _ := testEngine().Check(context.Background(), &enginev1.CheckRequest{
		Sql:  "DELETE FROM orders",
		Rule: &enginev1.AuditRole{DmlWhere: true},
	})
	if rep2.Records[0].Level != 1 {
		t.Fatalf("未配置级别时仍应拦截（level=1），实际 %+v", rep2.Records[0])
	}
}
