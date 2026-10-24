package server

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"engine/api"

	_ "github.com/go-sql-driver/mysql"
)

func TestLiteral(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, "NULL"},
		{[]byte("plain"), "'plain'"},
		{int64(12), "12"},
		{true, "1"},
		{time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC), "'2026-09-11 10:00:00'"},
		// DATETIME(6)/TIMESTAMP(6) 的微秒必须保留，截断到秒会把回滚本身写坏
		{time.Date(2026, 9, 11, 10, 0, 0, 123456000, time.UTC), "'2026-09-11 10:00:00.123456'"},
		{time.Date(2026, 9, 11, 10, 0, 0, 120000000, time.UTC), "'2026-09-11 10:00:00.12'"},
		// 含单引号/反斜杠的值用十六进制：'' 双写在两种 sql_mode 下都成立，但反斜杠
		// 无论转不转义都必踩一种模式（默认模式丢反斜杠 / NO_BACKSLASH_ESCAPES 写坏）
		{[]byte("a'b"), "0x612762"},
		{[]byte(`a\b`), "0x615c62"},
	}
	for _, c := range cases {
		if got := literal(c.in); got != c.want {
			t.Errorf("literal(%v) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestHasLimit(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"ORDER BY id LIMIT 3", true},
		{"ORDER BY id", false},
		// 引号里的 limit 只是文本，不是子句（原实现按空白切词会误判，
		// 跳过保护性 LIMIT 后大表前镜像全量拉取）
		{`WHERE note='no limit here'`, false},
		// 子查询的 LIMIT 约束不了外层，不算
		{"WHERE id IN (SELECT id FROM t LIMIT 5)", false},
		{`WHERE note='x' AND id IN (SELECT 1) LIMIT 10`, true},
	}
	for _, c := range cases {
		if got := hasLimit(c.in); got != c.want {
			t.Errorf("hasLimit(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestQuoteIdent(t *testing.T) {
	if got := quoteIdent("t"); got != "`t`" {
		t.Fatalf("普通标识符: %s", got)
	}
	// 带反引号的标识符（CREATE TABLE `we``ird`）必须双写，否则闭合引用拼出注入面
	if got := quoteIdent("we`ird"); got != "`we``ird`" {
		t.Fatalf("含反引号标识符: %s", got)
	}
}

func TestTruncationWarning(t *testing.T) {
	if truncationWarning(999) != "" {
		t.Fatal("未达上限不应告警")
	}
	if w := truncationWarning(1000); w == "" || !strings.Contains(w, "1000") {
		t.Fatalf("抓满上限应告警: %q", w)
	}
}

// TestAttachRollbackWarning 回滚告警必须挂到最后一条记录上（Status 不变），
// 已有错误时追加而不是覆盖——静默吞掉会让 backup=1 的工单无人知晓地失去回滚。
func TestAttachRollbackWarning(t *testing.T) {
	recs := []*api.Record{{Sql: "a", Status: "已执行"}}
	attachRollbackWarning(recs, "回滚抓取失败")
	if recs[0].Error != "回滚抓取失败" || recs[0].Status != "已执行" {
		t.Fatalf("应挂到末条记录且不改状态: %+v", recs[0])
	}
	recs[0].Error = "执行失败: xxx"
	attachRollbackWarning(recs, "回滚不完整")
	if !strings.Contains(recs[0].Error, "执行失败: xxx") || !strings.Contains(recs[0].Error, "回滚不完整") {
		t.Fatalf("已有错误时应追加: %+v", recs[0])
	}
	// 空告警 / 空记录不动
	attachRollbackWarning(nil, "x")
	attachRollbackWarning(recs, "")
}

// TestRollbackBuilderSkipsODKUAndReplace 回归：REPLACE / ON DUPLICATE KEY UPDATE
// 会改写工单之前就存在的行，按显式主键生成 DELETE 回滚会把那行旧行删掉。
// 这类语句必须返回空（宁可不给回滚），且判定在建连查元数据之前就能完成（离线可测）。
func TestRollbackBuilderSkipsODKUAndReplace(t *testing.T) {
	rb := newRollbackBuilder(nil, "")
	ctx := context.Background()
	for _, s := range []string{
		"INSERT INTO t (id, name) VALUES (1, 'a') ON DUPLICATE KEY UPDATE name = VALUES(name)",
		"REPLACE INTO t (id, name) VALUES (1, 'a')",
	} {
		if got, warn := rb.build(ctx, s); got != "" || warn != "" {
			t.Fatalf("不应生成回滚语句（会删掉工单前就存在的行）: %s -> %s (%s)", s, got, warn)
		}
	}
}

// TestRollbackBuilder 需要可写的 MySQL：YEARING_TEST_DSN="user:pass@tcp(host:3306)/demo?..."。
// 未设置时跳过，保证 CI 不依赖外部环境。
func TestRollbackBuilder(t *testing.T) {
	dsn := os.Getenv("YEARING_TEST_DSN")
	if dsn == "" {
		t.Skip("未设置 YEARING_TEST_DSN，跳过集成用例")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS rb_probe (id INT PRIMARY KEY, name VARCHAR(32), note VARCHAR(64))"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO rb_probe (id,name,note) VALUES (1,'a','x'),(2,'b','y') ON DUPLICATE KEY UPDATE note=VALUES(note)"); err != nil {
		t.Fatal(err)
	}
	rb := newRollbackBuilder(db, "demo")
	ctx := context.Background()

	t.Run("UPDATE", func(t *testing.T) {
		got, warn := rb.build(ctx, "UPDATE rb_probe SET name='z' WHERE id=1")
		if warn != "" {
			t.Fatalf("不应有告警: %q", warn)
		}
		if !strings.Contains(got, "UPDATE `demo`.`rb_probe` SET") || !strings.Contains(got, "`name` = 'a'") || !strings.Contains(got, "WHERE `id` = 1") {
			t.Fatalf("回滚语句不符预期: %s", got)
		}
	})
	t.Run("DELETE", func(t *testing.T) {
		got, _ := rb.build(ctx, "DELETE FROM rb_probe WHERE id=2")
		if !strings.Contains(got, "INSERT INTO `demo`.`rb_probe`") || !strings.Contains(got, "'b'") {
			t.Fatalf("回滚语句不符预期: %s", got)
		}
	})
	t.Run("INSERT", func(t *testing.T) {
		got, _ := rb.build(ctx, "INSERT INTO rb_probe (id,name,note) VALUES (7,'g','h')")
		if got != "DELETE FROM `demo`.`rb_probe` WHERE `id` = 7;" {
			t.Fatalf("回滚语句不符预期: %s", got)
		}
	})
	t.Run("不支持场景返回空", func(t *testing.T) {
		for _, s := range []string{
			"UPDATE rb_probe SET name='z'",                                // 无 WHERE
			"UPDATE rb_probe SET id=9 WHERE id=1",                         // 改主键
			"INSERT INTO rb_probe (name,note) VALUES ('x','y')",           // 未给主键
			"UPDATE rb_probe SET name=CONCAT('a','b') WHERE id=1 AND 1=0", // 无命中行也无妨
		} {
			_, _ = rb.build(ctx, s)
		}
		if got, _ := rb.build(ctx, "UPDATE rb_probe SET name='z'"); got != "" {
			t.Fatalf("无 WHERE 的 UPDATE 不应生成回滚: %s", got)
		}
		if got, _ := rb.build(ctx, "UPDATE rb_probe SET id=9 WHERE id=1"); got != "" {
			t.Fatalf("改主键的 UPDATE 不应生成回滚: %s", got)
		}
		if got, _ := rb.build(ctx, "INSERT INTO rb_probe (name) VALUES ('x')"); got != "" {
			t.Fatalf("未给主键的 INSERT 不应生成回滚: %s", got)
		}
	})
	_, _ = db.Exec("DROP TABLE rb_probe")
}
