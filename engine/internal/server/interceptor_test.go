package server

import (
	"context"
	"net"
	"testing"

	enginev1 "engine/gen/engine/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// startTestEngine 起一个带令牌认证的真实 gRPC server（随机端口），返回地址与关闭函数。
func startTestEngine(t *testing.T, token string) (string, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := grpc.NewServer(
		grpc.ChainUnaryInterceptor(UnaryAuthInterceptor(token)),
		grpc.ChainStreamInterceptor(StreamAuthInterceptor(token)),
	)
	enginev1.RegisterEngineServiceServer(s, NewEngine())
	healthpb.RegisterHealthServer(s, health.NewServer())
	go func() { _ = s.Serve(lis) }()
	return lis.Addr().String(), s.Stop
}

// TestAuthInterceptorEndToEnd 令牌链路端到端：
//   - 业务方法无令牌/错令牌 → Unauthenticated；对令牌 → 到达业务逻辑
//   - 健康检查不带令牌仍可用（容器探针不会携带令牌）
func TestAuthInterceptorEndToEnd(t *testing.T) {
	addr, stop := startTestEngine(t, "s3cret-token")
	defer stop()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	cli := enginev1.NewEngineServiceClient(conn)
	hc := healthpb.NewHealthClient(conn)

	// 无令牌 → 拒绝
	if _, err := cli.Check(context.Background(), &enginev1.CheckRequest{Sql: "SELECT 1"}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("无令牌应 Unauthenticated, got %v", err)
	}
	// 错令牌 → 拒绝
	bad := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer wrong")
	if _, err := cli.Check(bad, &enginev1.CheckRequest{Sql: "SELECT 1"}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("错令牌应 Unauthenticated, got %v", err)
	}
	// 对令牌 → 到达业务逻辑（空 SQL 报业务错误而非认证错误）
	good := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer s3cret-token")
	if _, err := cli.Check(good, &enginev1.CheckRequest{Sql: "SELECT 1"}); status.Code(err) == codes.Unauthenticated {
		t.Fatalf("对令牌不应报认证错误, got %v", err)
	}
	// 健康检查豁免：探针不带令牌
	if _, err := hc.Check(context.Background(), &healthpb.HealthCheckRequest{}); err != nil {
		t.Fatalf("健康检查应豁免认证: %v", err)
	}
}

// TestAuthInterceptorDisabled 令牌为空（未启用认证）时全部放行——纯内网部署的现状兼容。
func TestAuthInterceptorDisabled(t *testing.T) {
	addr, stop := startTestEngine(t, "")
	defer stop()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	if _, err := enginev1.NewEngineServiceClient(conn).Check(context.Background(), &enginev1.CheckRequest{Sql: "SELECT 1"}); status.Code(err) == codes.Unauthenticated {
		t.Fatalf("未启用认证时不应拒绝: %v", err)
	}
}

// TestBearerToken 解析形态：Bearer 前缀大小写不敏感、无前缀视为缺失。
func TestBearerToken(t *testing.T) {
	incoming := func(vals ...string) context.Context {
		return metadata.NewIncomingContext(context.Background(), metadata.MD{"authorization": vals})
	}
	if got := bearerToken(incoming("bearer  abc ")); got != "abc" {
		t.Fatalf("小写 bearer 也应解析: %q", got)
	}
	if got := bearerToken(incoming("raw-token")); got != "" {
		t.Fatalf("无 Bearer 前缀应视为缺失: %q", got)
	}
	if got := bearerToken(context.Background()); got != "" {
		t.Fatalf("无 metadata 应返回空: %q", got)
	}
}
