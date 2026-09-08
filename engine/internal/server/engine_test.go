package server

import (
	"context"
	"net"
	"testing"

	enginev1 "engine/gen/engine/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func newTestConn(t *testing.T) (enginev1.EngineServiceClient, func()) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	enginev1.RegisterEngineServiceServer(s, NewEngine())
	go func() {
		_ = s.Serve(lis)
	}()
	cc, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return enginev1.NewEngineServiceClient(cc), func() {
		_ = cc.Close()
		s.Stop()
		_ = lis.Close()
	}
}

func TestEngineRPCWire(t *testing.T) {
	client, cleanup := newTestConn(t)
	defer cleanup()
	ctx := context.Background()

	// Check 已实现，应返回 Ok=true 且对合法 SQL 拆分出记录。
	if rep, err := client.Check(ctx, &enginev1.CheckRequest{Sql: "SELECT 1; SELECT 2"}); err != nil {
		t.Fatalf("Check err: %v", err)
	} else if !rep.Ok {
		t.Fatalf("Check unexpected ok=false, error=%q", rep.Error)
	} else if len(rep.Records) != 2 {
		t.Fatalf("Check expected 2 records, got %d", len(rep.Records))
	}

	// MergeAlterTables 已实现：同表 ALTER 应返回 Ok=true 与合并结果。
	if rep, err := client.MergeAlterTables(ctx, &enginev1.MergeAlterTablesRequest{Sqls: "ALTER TABLE t ADD a INT; ALTER TABLE t ADD b INT"}); err != nil {
		t.Fatalf("MergeAlterTables err: %v", err)
	} else if !rep.Ok {
		t.Fatalf("MergeAlterTables unexpected ok=false, error=%q", rep.Error)
	} else if rep.Sql == "" {
		t.Fatalf("MergeAlterTables returned empty sql")
	}

	// Exec：空工单/无数据源应返回 Ok=false 且不 panic（真实连库场景在集成测试验证）。
	if rep, err := client.Exec(ctx, &enginev1.ExecRequest{Order: &enginev1.Order{Sql: "SELECT 1"}, Source: &enginev1.DataSource{Ip: "", Port: 3306}}); err != nil {
		t.Fatalf("Exec err: %v", err)
	} else if rep.Ok {
		t.Fatalf("Exec unexpected ok=true")
	}

	// Query 已实现：合法 SQL 返回 Ok=true 并拆出语句。
	if rep, err := client.Query(ctx, &enginev1.QueryRequest{Sql: "SELECT 1; SELECT 2", InsulateWordList: "phone,email"}); err != nil {
		t.Fatalf("Query err: %v", err)
	} else if !rep.Ok {
		t.Fatalf("Query unexpected ok=false, error=%q", rep.Error)
	} else if len(rep.Records) != 2 {
		t.Fatalf("Query expected 2 records, got %d", len(rep.Records))
	}

	if rep, err := client.StopDelay(ctx, &enginev1.StopDelayRequest{}); err != nil {
		t.Fatalf("StopDelay err: %v", err)
	} else if rep.Ok {
		t.Fatalf("StopDelay unexpected ok=true")
	}
}
