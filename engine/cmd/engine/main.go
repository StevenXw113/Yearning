package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	enginev1 "engine/gen/engine/v1"
	"engine/internal/server"

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
	flag.Parse()

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", *addr, err)
	}

	s := grpc.NewServer(
		grpc.MaxRecvMsgSize(maxMsgSize),
		grpc.MaxSendMsgSize(maxMsgSize),
	)
	enginev1.RegisterEngineServiceServer(s, server.NewEngine())
	// 标准健康检查：容器探针 / 编排系统就绪检测用（grpc_healthProbe / k8s grpc 探针）
	healthpb.RegisterHealthServer(s, health.NewServer())
	// 服务反射：grpcurl 等工具可直接调试，无需手写 proto 文件
	reflection.Register(s)

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
