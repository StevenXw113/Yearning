package server

import (
	"context"
	"testing"

	enginev1 "engine/gen/engine/v1"
)

func TestQuerySplitAndInsulate(t *testing.T) {
	rep, err := NewEngine().Query(context.Background(), &enginev1.QueryRequest{
		Sql:              "SELECT id FROM users; SELECT * FROM orders",
		InsulateWordList: "phone, email",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !rep.Ok {
		t.Fatalf("ok=false: %s", rep.Error)
	}
	if len(rep.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(rep.Records))
	}
	for _, r := range rep.Records {
		if r.Error != "" {
			t.Fatalf("unexpected error: %s", r.Error)
		}
		if len(r.InsulateWordList) != 2 {
			t.Fatalf("expected 2 insulate words, got %v", r.InsulateWordList)
		}
	}
}

func TestQuerySyntaxError(t *testing.T) {
	rep, _ := NewEngine().Query(context.Background(), &enginev1.QueryRequest{Sql: "SELECT * FORM t"})
	if !rep.Ok {
		// 整段语法失败，返回 Ok=false。
		return
	}
	// 若被拆出记录，则其中应有 error 标记。
	if len(rep.Records) == 0 {
		t.Fatalf("expected records or ok=false")
	}
	for _, r := range rep.Records {
		if r.Error != "" {
			return
		}
	}
	t.Fatalf("expected some syntax error signal, got %+v", rep.Records)
}

func TestQueryEmpty(t *testing.T) {
	rep, _ := NewEngine().Query(context.Background(), &enginev1.QueryRequest{Sql: ""})
	if rep.Ok {
		t.Fatalf("expected ok=false for empty")
	}
}
