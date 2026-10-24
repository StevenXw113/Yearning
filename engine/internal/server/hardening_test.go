package server

import (
	"context"
	"strings"
	"testing"

	"engine/api"
)

// TestUnknownKindFailsClosed 未知 kind 显式拒绝：新增数据源若引擎侧未实现，
// 静默按 MySQL 审会产出误报/漏报都难察觉的假结果。三个 RPC 行为一致。
func TestUnknownKindFailsClosed(t *testing.T) {
	src := &api.DataSource{Kind: "clickhouse"}

	rep, err := NewEngine().Check(context.Background(), &api.CheckRequest{
		Sql: "SELECT 1", Rule: &api.AuditRole{}, Source: src,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if rep.Ok || !strings.Contains(rep.Error, "不支持") {
		t.Fatalf("Check 未知 kind 应拒绝: %+v", rep)
	}

	erep, _ := NewEngine().Exec(context.Background(), &api.ExecRequest{
		Order:  &api.Order{Sql: "SELECT 1", Type: 0},
		Source: src,
	})
	if erep.Ok || !strings.Contains(erep.Error, "不支持") {
		t.Fatalf("Exec 未知 kind 应拒绝: %+v", erep)
	}

	qrep, _ := NewEngine().Query(context.Background(), &api.QueryRequest{Sql: "SELECT 1", Kind: "clickhouse"})
	if qrep.Ok || !strings.Contains(qrep.Error, "不支持") {
		t.Fatalf("Query 未知 kind 应拒绝: %+v", qrep)
	}

	// mysql 与空 kind 仍按 MySQL（历史调用方行为不变）
	for _, kind := range []string{"mysql", ""} {
		mrep, _ := NewEngine().Check(context.Background(), &api.CheckRequest{
			Sql: "SELECT 1", Rule: &api.AuditRole{}, Source: &api.DataSource{Kind: kind},
		})
		if !mrep.Ok {
			t.Fatalf("kind=%q 应按 MySQL 放行: %+v", kind, mrep)
		}
	}
}

// TestIsReadOnlyQuery 只读白名单的词法判定：注释藏关键字、字符串里的 "delete"
// 都骗不过；PostgreSQL 的 WITH ... INSERT 也能识别为写。
func TestIsReadOnlyQuery(t *testing.T) {
	cases := []struct {
		sql  string
		want bool
	}{
		{"SELECT 1", true},
		{"select * from t where a=1", true},
		{"SHOW TABLES", true},
		{"DESC t", true},
		{"EXPLAIN SELECT 1", true},
		{"DELETE FROM t", false},
		{"UPDATE t SET a=1", false},
		{"INSERT INTO t VALUES (1)", false},
		{"TRUNCATE TABLE t", false},
		{"  \n SELECT 1", true},
		// 注释开头不影响首关键字识别；注释里的关键字也不被当作子句
		{"-- delete all\nSELECT 1", true},
		{"/* update */ SELECT 1", true},
		// 字符串里的关键字不误判
		{"SELECT * FROM t WHERE note='please delete me'", true},
		{"SELECT 'update' FROM t", true},
		// WITH：只读 CTE 放行；PostgreSQL 的 WITH ... INSERT 拒绝
		{"WITH c AS (SELECT 1) SELECT * FROM c", true},
		{"WITH c AS (SELECT 1) INSERT INTO t SELECT * FROM c", false},
		{"WITH del AS (DELETE FROM t RETURNING *) SELECT * FROM del", false},
		// 括号里的子查询关键字不算顶层
		{"SELECT * FROM t WHERE id IN (SELECT id FROM u)", true},
	}
	for _, c := range cases {
		if got := isReadOnlyQuery(c.sql); got != c.want {
			t.Errorf("isReadOnlyQuery(%q) = %v, want %v", c.sql, got, c.want)
		}
	}
}

// TestQueryRejectsWriteStatement 查询页预检端到端：写语句以 level=1 记录返回，
// 主程序 PreCheck 读到 rec.Error 即拒绝，本地不会再执行。
func TestQueryRejectsWriteStatement(t *testing.T) {
	for _, kind := range []string{"", "mysql", "postgres", "doris"} {
		rep, err := NewEngine().Query(context.Background(), &api.QueryRequest{Sql: "DELETE FROM t WHERE id=1", Kind: kind})
		if err != nil {
			t.Fatalf("kind=%q err: %v", kind, err)
		}
		if !rep.Ok || len(rep.Records) != 1 {
			t.Fatalf("kind=%q 应返回记录级拒绝: %+v", kind, rep)
		}
		r := rep.Records[0]
		if r.Level != 1 || !strings.Contains(r.Error, "只读") {
			t.Fatalf("kind=%q 写语句应被拒: %+v", kind, r)
		}
	}
}

// TestMergeAlterKnownUnsafeCombos 词法拼接的已知不安全形态放弃合并、原样返回。
func TestMergeAlterKnownUnsafeCombos(t *testing.T) {
	cases := []string{
		// 一条 ALTER 只能有一个 ALGORITHM/LOCK 子句
		"ALTER TABLE t ADD a INT, ALGORITHM=INPLACE; ALTER TABLE t ADD b INT, ALGORITHM=COPY",
		// FULLTEXT 索引一条 ALTER 只能加一个
		"ALTER TABLE t ADD FULLTEXT INDEX f1(a); ALTER TABLE t ADD FULLTEXT INDEX f2(b)",
	}
	for _, sql := range cases {
		rep, err := NewEngine().MergeAlterTables(context.Background(), &api.MergeAlterTablesRequest{Sqls: sql})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !rep.Ok {
			t.Fatalf("ok=false: %s", rep.Error)
		}
		if strings.Count(strings.ToUpper(rep.Sql), "ALTER TABLE") != 2 {
			t.Fatalf("不安全组合应放弃合并、原样返回: %q", rep.Sql)
		}
	}
}

// TestRedisParseDB 非法库名报错，不再静默落 db0。
func TestRedisParseDB(t *testing.T) {
	if n, err := redisParseDB("db5"); err != nil || n != 5 {
		t.Fatalf("db5 应解析为 5: %d %v", n, err)
	}
	if n, err := redisParseDB(""); err != nil || n != 0 {
		t.Fatalf("空名应默认 db0: %d %v", n, err)
	}
	for _, bad := range []string{"5", "db", "dbx", "db-1", "database1"} {
		if _, err := redisParseDB(bad); err == nil {
			t.Fatalf("%q 应报错而不是静默落 db0", bad)
		}
	}
}
