package server

import (
	"context"
	"strings"
	"testing"

	"engine/api"

	storepb "engine/internal/bytebase/generated-go/store"
)

func testEngine() *Engine { return NewEngine() }

func TestCheckSyntaxError(t *testing.T) {
	rep, err := testEngine().Check(context.Background(), &api.CheckRequest{
		Sql:  "SELEC 1 FROM",
		Rule: &api.AuditRole{},
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
	rep, err := testEngine().Check(context.Background(), &api.CheckRequest{
		Sql:  "SELECT 1; SELECT 2; SELECT 3",
		Rule: &api.AuditRole{},
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
	rep, _ := testEngine().Check(context.Background(), &api.CheckRequest{
		Sql:  "DROP TABLE users",
		Rule: &api.AuditRole{},
	})
	if rep.Records[0].Error == "" {
		t.Fatalf("expected drop blocked, got %+v", rep.Records[0])
	}
	// 开启后放行。
	rep2, _ := testEngine().Check(context.Background(), &api.CheckRequest{
		Sql:  "DROP TABLE users",
		Rule: &api.AuditRole{DdlEnableDropTable: true},
	})
	if rep2.Records[0].Error != "" {
		t.Fatalf("expected drop allowed, got %+v", rep2.Records[0])
	}
}

func TestCheckCreateTableRequirePK(t *testing.T) {
	sql := "CREATE TABLE t (id INT, name VARCHAR(10))"
	rep, _ := testEngine().Check(context.Background(), &api.CheckRequest{
		Sql:  sql,
		Rule: &api.AuditRole{DdlEnablePrimaryKey: true},
	})
	if !strings.Contains(rep.Records[0].Error, "TABLE_REQUIRE_PK") {
		t.Fatalf("expected pk rule hit, got %+v", rep.Records[0])
	}

	rep2, _ := testEngine().Check(context.Background(), &api.CheckRequest{
		Sql:  "CREATE TABLE t (id INT PRIMARY KEY, name VARCHAR(10))",
		Rule: &api.AuditRole{DdlEnablePrimaryKey: true},
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
	rep, err := testEngine().Check(context.Background(), &api.CheckRequest{
		Sql: "CREATE TABLE t (id INT PRIMARY KEY, name VARCHAR(20))",
		Rule: &api.AuditRole{
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
	rep, _ := testEngine().Check(context.Background(), &api.CheckRequest{
		Sql:  "CREATE TABLE bad_name (id INT PRIMARY KEY)",
		Rule: &api.AuditRole{DdlTablePrefix: "app_"},
	})
	if !strings.Contains(rep.Records[0].Error, "NAMING_TABLE") {
		t.Fatalf("expected naming rule hit, got %+v", rep.Records[0])
	}
}

func TestCheckUpdateWithoutWhere(t *testing.T) {
	// 规则要求 DML 必须有 where
	rep, _ := testEngine().Check(context.Background(), &api.CheckRequest{
		Sql:  "UPDATE users SET name='x'",
		Rule: &api.AuditRole{DmlWhere: true},
	})
	if !strings.Contains(rep.Records[0].Error, "WHERE") {
		t.Fatalf("expected no-where blocked, got %+v", rep.Records[0])
	}

	// 合法 update 应放行
	rep2, _ := testEngine().Check(context.Background(), &api.CheckRequest{
		Sql:  "UPDATE users SET name='x' WHERE id=1",
		Rule: &api.AuditRole{DmlWhere: true},
	})
	if rep2.Records[0].Error != "" {
		t.Fatalf("expected pass with where, got %+v", rep2.Records[0])
	}
}

func TestCheckDeleteWithoutWhere(t *testing.T) {
	rep, _ := testEngine().Check(context.Background(), &api.CheckRequest{
		Sql:  "DELETE FROM orders",
		Rule: &api.AuditRole{DmlWhere: true},
	})
	if !strings.Contains(rep.Records[0].Error, "WHERE") {
		t.Fatalf("expected delete blocked, got %+v", rep.Records[0])
	}
}

// 规则级别：默认 error 拦截，warn/observe 只提示/记录（都不拦提交）。
func TestReviewRulesLevelGroups(t *testing.T) {
	role := &api.AuditRole{
		DmlWhere:            true,
		DmlOrder:            true,
		DdlEnablePrimaryKey: true,
		RuleLevels: map[string]string{
			"DMLOrder":            "warn",
			"DDLEnablePrimaryKey": "observe",
		},
	}
	plan := reviewRules(role, storepb.Engine_MYSQL)
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
	rep, _ := testEngine().Check(context.Background(), &api.CheckRequest{
		Sql: "DELETE FROM orders",
		Rule: &api.AuditRole{
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

	rep2, _ := testEngine().Check(context.Background(), &api.CheckRequest{
		Sql:  "DELETE FROM orders",
		Rule: &api.AuditRole{DmlWhere: true},
	})
	if rep2.Records[0].Level != 1 {
		t.Fatalf("未配置级别时仍应拦截（level=1），实际 %+v", rep2.Records[0])
	}
}

// TestCheckDispatchesBySourceKind 核对 kind → 方言的分派：MongoDB / Redis 走自研规则，
// 未设置 source 时仍按 MySQL。这三条分支此前没有任何测试覆盖。
func TestCheckDispatchesBySourceKind(t *testing.T) {
	// mongodb：查询模式下只读命令通过、写命令被拒（说明没进 SQL 解析器）
	rep, err := testEngine().Check(context.Background(), &api.CheckRequest{
		Sql:    `{"find":"users"}`,
		Mode:   "query",
		Source: &api.DataSource{Kind: "mongodb"},
		Rule:   &api.AuditRole{},
	})
	if err != nil || !rep.Ok {
		t.Fatalf("mongodb 只读命令应通过: err=%v rep=%+v", err, rep)
	}
	rep, err = testEngine().Check(context.Background(), &api.CheckRequest{
		Sql:    `{"delete":"users","deletes":[{"q":{"a":1},"limit":0}]}`,
		Mode:   "query",
		Source: &api.DataSource{Kind: "mongodb"},
		Rule:   &api.AuditRole{},
	})
	if err != nil || rep.Ok || !strings.Contains(rep.Error, "只读") {
		t.Fatalf("mongodb 写命令在查询模式下应被拒: err=%v rep=%+v", err, rep)
	}

	// redis：合法命令按命令审核，不应出现 SQL 语法错误
	rep, err = testEngine().Check(context.Background(), &api.CheckRequest{
		Sql:    "SET a b",
		Source: &api.DataSource{Kind: "redis"},
		Rule:   &api.AuditRole{},
	})
	if err != nil || !rep.Ok || len(rep.Records) != 1 {
		t.Fatalf("redis 命令应走命令审核: err=%v rep=%+v", err, rep)
	}
	if strings.Contains(rep.Records[0].Error, "语法错误") {
		t.Fatalf("redis 命令不该被当成 SQL 解析: %+v", rep.Records[0])
	}

	// 未设置 source：仍按 MySQL（老调用方行为不变）
	rep, err = testEngine().Check(context.Background(), &api.CheckRequest{
		Sql:  "SELEC 1",
		Rule: &api.AuditRole{},
	})
	if err != nil || len(rep.Records) != 1 || !strings.Contains(rep.Records[0].Error, "语法错误") {
		t.Fatalf("空 source 应仍按 MySQL 解析: err=%v rep=%+v", err, rep)
	}
}

// TestFirstKeyword 首关键字提取要能跳过前导注释（-- / # / /* */），否则带注释的
// DDL 语句会被漏数、多条 DDL 检测失效。
func TestFirstKeyword(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"alter table t add c int", "ALTER"},
		{"  \n CREATE TABLE t(id int)", "CREATE"},
		{"-- 注释\nDROP TABLE t", "DROP"},
		{"# hash 注释\ntruncate table t", "TRUNCATE"},
		{"/* 块注释 */rename table a to b", "RENAME"},
		{"update t set x=1", "UPDATE"},
		{"", ""},
		{"/* 未闭合注释", ""},
	}
	for _, c := range cases {
		if got := firstKeyword(c.in); got != c.want {
			t.Fatalf("firstKeyword(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestCheckMultiDDLRules DdlMultiToCommit（允许工单内多条 DDL）此前是死配置，
// 引擎从未校验。此用例固定它的两条行为：开关关闭时多条 DDL 报整单级错误，
// 打开后放行；单条 DDL 不受影响。
func TestCheckMultiDDLRules(t *testing.T) {
	multiDDL := "CREATE TABLE a(id int);\nCREATE TABLE b(id int);"

	rep, _ := testEngine().Check(context.Background(), &api.CheckRequest{Sql: multiDDL, Rule: &api.AuditRole{}})
	if !rep.Ok {
		t.Fatalf("ok=false: %s", rep.Error)
	}
	found := false
	for _, r := range rep.Records {
		if r.Sql == "(整单校验)" {
			found = true
			if r.Level != 1 || !strings.Contains(r.Error, "DDLMultiToCommit") {
				t.Fatalf("multi-DDL 违规应为 Level1 错误记录, got %+v", r)
			}
		}
	}
	if !found {
		t.Fatalf("未开启 DDLMultiToCommit 时应追加整单校验错误记录, records=%+v", rep.Records)
	}

	rep, _ = testEngine().Check(context.Background(), &api.CheckRequest{
		Sql:  multiDDL,
		Rule: &api.AuditRole{DdlMultiToCommit: true},
	})
	for _, r := range rep.Records {
		if r.Sql == "(整单校验)" {
			t.Fatalf("开启 DDLMultiToCommit 后不应再报整单校验错误, got %+v", r)
		}
	}

	// 单条 DDL + 若干 DML：不算多条 DDL，不应触发
	rep, _ = testEngine().Check(context.Background(), &api.CheckRequest{
		Sql:  "CREATE TABLE a(id int);\nINSERT INTO a VALUES(1);",
		Rule: &api.AuditRole{},
	})
	for _, r := range rep.Records {
		if r.Sql == "(整单校验)" {
			t.Fatalf("单条 DDL 不应触发整单校验, got %+v", r)
		}
	}
}

// TestCheckMultiDDLRulesPostgreSQL PostgreSQL 入口与 MySQL 行为一致。
func TestCheckMultiDDLRulesPostgreSQL(t *testing.T) {
	rep, _ := testEngine().Check(context.Background(), &api.CheckRequest{
		Sql:    "CREATE TABLE a(id int);\nDROP TABLE b;",
		Schema: "public",
		Source: &api.DataSource{Kind: "postgres"},
		Rule:   &api.AuditRole{},
	})
	found := false
	for _, r := range rep.Records {
		if r.Sql == "(整单校验)" {
			found = true
		}
	}
	if !found {
		t.Fatalf("PostgreSQL 多条 DDL 也应触发整单校验, records=%+v", rep.Records)
	}
}

// TestCheckTableCommentConvention 回归:DDLCheckTableComment 映射的
// CommentConventionPayload 曾漏设 MaxLength,proto3 零值 0 命中上游 advisor 的
// 「MaxLength>=0 且 len>MaxLength」分支,任何非空注释都被判超长——开关永不可满足。
// 正确语义:只校验注释必填,不限长度(MaxLength=-1)。
func TestCheckTableCommentConvention(t *testing.T) {
	with := "CREATE TABLE t (id INT) COMMENT '业务表'"
	rep, _ := testEngine().Check(context.Background(), &api.CheckRequest{
		Sql:  with,
		Rule: &api.AuditRole{DdlCheckTableComment: true},
	})
	for _, r := range rep.Records {
		if strings.Contains(r.Error, "TABLE_COMMENT") {
			t.Fatalf("带注释的建表不应命中 TABLE_COMMENT, got %+v", r)
		}
	}

	without := "CREATE TABLE t (id INT)"
	rep2, _ := testEngine().Check(context.Background(), &api.CheckRequest{
		Sql:  without,
		Rule: &api.AuditRole{DdlCheckTableComment: true},
	})
	found := false
	for _, r := range rep2.Records {
		if strings.Contains(r.Error, "TABLE_COMMENT") {
			found = true
		}
	}
	if !found {
		t.Fatalf("无注释的建表应命中 TABLE_COMMENT, got %+v", rep2.Records)
	}
}
