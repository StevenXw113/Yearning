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
)

func main() {
	addr := flag.String("addr", ":13307", "gRPC 监听地址，如 0.0.0.0:13307")
	flag.Parse()

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", *addr, err)
	}

	s := grpc.NewServer()
	enginev1.RegisterEngineServiceServer(s, server.NewEngine())

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
