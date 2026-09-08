package server

import (
	"context"
	"strings"
	"testing"

	enginev1 "engine/gen/engine/v1"
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
		if r.Error != "" || r.Level == 0 || r.Level == 1 {
			t.Fatalf("legal select should pass, got %+v", r)
		}
	}
}

func TestCheckDropWhenForbidden(t *testing.T) {
	// 规则禁止 DROP TABLE
	rep, _ := testEngine().Check(context.Background(), &enginev1.CheckRequest{
		Sql:  "DROP TABLE users",
		Rule: &enginev1.AuditRole{DdlEnableDropTable: true},
	})
	if rep.Records[0].Error == "" {
		t.Fatalf("expected drop blocked, got %+v", rep.Records[0])
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
