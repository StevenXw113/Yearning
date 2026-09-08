package server

import (
	"context"
	"strings"
	"testing"

	enginev1 "engine/gen/engine/v1"
)

func TestMergeAlterSameTable(t *testing.T) {
	rep, err := NewEngine().MergeAlterTables(context.Background(), &enginev1.MergeAlterTablesRequest{
		Sqls: "ALTER TABLE users ADD age INT; ALTER TABLE users ADD city VARCHAR(50)",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !rep.Ok {
		t.Fatalf("ok=false: %s", rep.Error)
	}
	wantPrefix := "ALTER TABLE users ADD age INT, ADD city VARCHAR(50)"
	if !strings.EqualFold(strings.TrimSpace(rep.Sql), wantPrefix) {
		t.Fatalf("unexpected merged sql: %q", rep.Sql)
	}
}

func TestMergeAlterDifferentTableKeepsOriginal(t *testing.T) {
	rep, _ := NewEngine().MergeAlterTables(context.Background(), &enginev1.MergeAlterTablesRequest{
		Sqls: "ALTER TABLE a ADD x INT; ALTER TABLE b ADD y INT",
	})
	if !rep.Ok {
		t.Fatalf("ok=false")
	}
	// 跨表不强制合并，返回原始两条（分号连接）。
	if !strings.Contains(rep.Sql, "ALTER TABLE a") || !strings.Contains(rep.Sql, "ALTER TABLE b") {
		t.Fatalf("expected original kept, got %q", rep.Sql)
	}
}

func TestMergeAlterEmpty(t *testing.T) {
	rep, _ := NewEngine().MergeAlterTables(context.Background(), &enginev1.MergeAlterTablesRequest{Sqls: ""})
	if rep.Ok {
		t.Fatalf("expected ok=false for empty input")
	}
}
