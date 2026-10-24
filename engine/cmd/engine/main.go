package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	enginev1 "engine/gen/engine/v1"
	"engine/internal/server"
	"engine/internal/server/emetrics"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// maxMsgSize 单条 gRPC 消息上限：默认 4MB 装不下长 INSERT 工单（大 VALUES 批次），
// 放宽到 64MB；再大的工单本就该拆分提交。
const maxMsgSize = 64 << 20

func main() {
	addr := flag.String("addr", ":13307", "gRPC 监听地址，如 0.0.0.0:13307")
	// metricsAddr Prometheus 指标端点：独立 HTTP 口，0 = 关闭。仅暴露聚合计数
	// 与进程状态，无业务数据；建议绑内网口。
	metricsAddr := flag.String("metrics-addr", "", "Prometheus /metrics 监听地址（如 :13308），空 = 关闭")
	// authToken 共享访问令牌：非空时所有业务方法要求 metadata
	// authorization: Bearer <token>（健康检查/服务反射豁免）。
	// 引擎是「拿到数据源凭据即可执行 SQL」的代理，跨不完全可信网络部署必须启用。
	authToken := flag.String("auth-token", "", "共享访问令牌（也可用环境变量 ENGINE_AUTH_TOKEN 注入）；空 = 不启用认证")
	flag.Parse()

	token := strings.TrimSpace(*authToken)
	if token == "" {
		token = strings.TrimSpace(os.Getenv("ENGINE_AUTH_TOKEN"))
	}

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", *addr, err)
	}

	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(maxMsgSize),
		grpc.MaxSendMsgSize(maxMsgSize),
		// 埋点在最内层：统计的是业务处理本身的耗时（认证通过之后的）
		grpc.ChainUnaryInterceptor(server.UnaryStatsInterceptor()),
		grpc.ChainStreamInterceptor(server.StreamStatsInterceptor()),
	}
	if token != "" {
		log.Printf("engine auth: enabled (shared token)")
		opts = append(opts,
			grpc.ChainUnaryInterceptor(server.UnaryAuthInterceptor(token)),
			grpc.ChainStreamInterceptor(server.StreamAuthInterceptor(token)),
		)
	} else {
		log.Printf("engine auth: DISABLED (no token configured; safe only on fully trusted networks)")
	}
	s := grpc.NewServer(opts...)
	enginev1.RegisterEngineServiceServer(s, server.NewEngine())
	// 标准健康检查：容器探针 / 编排系统就绪检测用（grpc_healthProbe / k8s grpc 探针）
	healthpb.RegisterHealthServer(s, health.NewServer())
	// 服务反射：grpcurl 等工具可直接调试，无需手写 proto 文件
	reflection.Register(s)

	if *metricsAddr != "" {
		go serveMetrics(*metricsAddr)
	}

	go func() {
		log.Printf("engine gRPC server listening on %s", *addr)
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down engine")
	s.GracefulStop()
}

// serveMetrics 独立 HTTP 口暴露 /metrics（Prometheus text exposition）。
func serveMetrics(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(emetrics.Render()))
	})
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("engine metrics listening on %s/metrics", addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Printf("metrics server stopped: %v", err)
	}
}
