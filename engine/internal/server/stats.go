package server

import (
	"context"
	"time"

	"engine/internal/server/emetrics"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// methodOf 从 FullMethod "/engine.v1.EngineService/Check" 提取短方法名。
func methodOf(fullMethod string) string {
	for i := len(fullMethod) - 1; i >= 0; i-- {
		if fullMethod[i] == '/' {
			return fullMethod[i+1:]
		}
	}
	return fullMethod
}

// UnaryStatsInterceptor RPC 计数/耗时埋点（供 -metrics-addr 的 /metrics 暴露）。
// 只统计不鉴权，与认证拦截器链式叠加（ChainUnaryInterceptor 顺序：先认证后统计）。
func UnaryStatsInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		emetrics.ObserveRPC(methodOf(info.FullMethod), time.Since(start), status.Code(err) != codes.Internal)
		return resp, err
	}
}

// StreamStatsInterceptor 流式方法的同一埋点。
func StreamStatsInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		err := handler(srv, ss)
		emetrics.ObserveRPC(methodOf(info.FullMethod), time.Since(start), status.Code(err) != codes.Internal)
		return err
	}
}
