package server

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func TestLiteral(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, "NULL"},
		{[]byte("a'b"), `'a\'b'`},
		{[]byte("plain"), "'plain'"},
		{int64(12), "12"},
		{true, "1"},
		{time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC), "'2026-09-11 10:00:00'"},
	}
	for _, c := range cases {
		if got := literal(c.in); got != c.want {
			t.Errorf("literal(%v) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestHasLimit(t *testing.T) {
	if !hasLimit("ORDER BY id LIMIT 3") || hasLimit("ORDER BY id") {
		t.Fatal("hasLimit 判定错误")
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
		got := rb.build(ctx, "UPDATE rb_probe SET name='z' WHERE id=1")
		if !strings.Contains(got, "UPDATE `demo`.`rb_probe` SET") || !strings.Contains(got, "`name` = 'a'") || !strings.Contains(got, "WHERE `id` = 1") {
			t.Fatalf("回滚语句不符预期: %s", got)
		}
	})
	t.Run("DELETE", func(t *testing.T) {
		got := rb.build(ctx, "DELETE FROM rb_probe WHERE id=2")
		if !strings.Contains(got, "INSERT INTO `demo`.`rb_probe`") || !strings.Contains(got, "'b'") {
			t.Fatalf("回滚语句不符预期: %s", got)
		}
	})
	t.Run("INSERT", func(t *testing.T) {
		got := rb.build(ctx, "INSERT INTO rb_probe (id,name,note) VALUES (7,'g','h')")
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
			_ = rb.build(ctx, s)
		}
		if got := rb.build(ctx, "UPDATE rb_probe SET name='z'"); got != "" {
			t.Fatalf("无 WHERE 的 UPDATE 不应生成回滚: %s", got)
		}
		if got := rb.build(ctx, "UPDATE rb_probe SET id=9 WHERE id=1"); got != "" {
			t.Fatalf("改主键的 UPDATE 不应生成回滚: %s", got)
		}
		if got := rb.build(ctx, "INSERT INTO rb_probe (name) VALUES ('x')"); got != "" {
			t.Fatalf("未给主键的 INSERT 不应生成回滚: %s", got)
		}
	})
	_, _ = db.Exec("DROP TABLE rb_probe")
}
