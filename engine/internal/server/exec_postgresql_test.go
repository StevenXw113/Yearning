package server

import (
	"context"
	"strings"
	"testing"

	enginev1 "engine/gen/engine/v1"
)

// TestPostgreSQLDatabaseCandidates 默认库候选：普通用户先 postgres 再试同名库，
// postgres 用户不重复。
func TestPostgreSQLDatabaseCandidates(t *testing.T) {
	if got := postgresqlDatabaseCandidates("alice"); len(got) != 2 || got[0] != "postgres" || got[1] != "alice" {
		t.Fatalf("alice 候选应为 [postgres alice]，got %v", got)
	}
	if got := postgresqlDatabaseCandidates("postgres"); len(got) != 1 || got[0] != "postgres" {
		t.Fatalf("postgres 用户候选应为 [postgres]，got %v", got)
	}
	if got := postgresqlDatabaseCandidates(""); len(got) != 1 {
		t.Fatalf("空用户候选应为 [postgres]，got %v", got)
	}
}

// TestPostgreSQLDSNValue DSN 值引用：安全字符直拼，含空白/引号的单引号包裹并双写内部引号，
// 否则口令里的空格会把 keyword=value 连接串拆坏。
func TestPostgreSQLDSNValue(t *testing.T) {
	cases := []struct{ in, want string }{
		{"postgres", "postgres"},
		{"my-secret", "my-secret"},
		{"", "''"},
		{"p@ss word", "'p@ss word'"},
		{"it's", "'it''s'"},
	}
	for _, c := range cases {
		if got := postgresqlDSNValue(c.in); got != c.want {
			t.Errorf("postgresqlDSNValue(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestPostgreSQLConnectRejectsEmptyIP 数据源 IP 为空必须在建立连接前就报错。
func TestPostgreSQLConnectRejectsEmptyIP(t *testing.T) {
	if _, err := connectPostgreSQL(context.Background(), nil, "public"); err == nil || !strings.Contains(err.Error(), "IP") {
		t.Fatalf("nil 数据源应报 IP 为空: %v", err)
	}
	if _, err := connectPostgreSQL(context.Background(), &enginev1.DataSource{}, "public"); err == nil || !strings.Contains(err.Error(), "IP") {
		t.Fatalf("空 IP 应报错: %v", err)
	}
}
