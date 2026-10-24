package server

import (
	"context"
	"crypto/subtle"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// 引擎是「拿到数据源凭据即可执行 SQL」的执行代理，必须有认证边界：
// 部署在非完全可信网络上时，13307 暴露等于暴露一个无鉴权的数据库执行入口。
// 首期用共享令牌（主程序 [General].RpcToken ↔ 引擎 ENGINE_AUTH_TOKEN）；
// 令牌为空表示引擎以无认证模式运行（纯内网部署的现状兼容）。

// authMetadataKey 令牌所在的 metadata 键，沿用 gRPC 惯例：
// authorization: Bearer <token>
const authMetadataKey = "authorization"

// bearerPrefix Bearer 方案前缀（大小写不敏感解析，见 bearerToken）。
const bearerPrefix = "bearer "

// publicMethods 无需令牌的方法：标准健康检查（容器探针不会带令牌）与服务反射
// （grpcurl 调试用，只暴露服务清单、不触及任何数据/执行能力）。
var publicMethods = map[string]struct{}{
	"/grpc.health.v1.Health/Check":                                   {},
	"/grpc.health.v1.Health/Watch":                                   {},
	"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo": {},
	"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo":      {},
}

// UnaryAuthInterceptor 校验请求携带的共享令牌（token 为空 = 不启用认证）。
func UnaryAuthInterceptor(token string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if !requestAuthorized(ctx, info.FullMethod, token) {
			return nil, errUnauthenticated()
		}
		return handler(ctx, req)
	}
}

// StreamAuthInterceptor 同一校验的流式版本（健康 Watch / 反射是流式方法）。
func StreamAuthInterceptor(token string) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if !requestAuthorized(ss.Context(), info.FullMethod, token) {
			return errUnauthenticated()
		}
		return handler(srv, ss)
	}
}

func errUnauthenticated() error {
	return status.Error(codes.Unauthenticated, "无效或缺失的引擎访问令牌（需 metadata authorization: Bearer <token>）")
}

// requestAuthorized 判定请求是否放行：令牌未启用、公开方法直接放行；
// 其余请求必须携带匹配的 Bearer 令牌（常数时间比较）。
func requestAuthorized(ctx context.Context, fullMethod, token string) bool {
	if token == "" {
		return true
	}
	if _, ok := publicMethods[fullMethod]; ok {
		return true
	}
	got := bearerToken(ctx)
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}

// bearerToken 从请求 metadata 提取 Bearer 令牌；缺失或方案不符返回空。
func bearerToken(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	for _, v := range md.Get(authMetadataKey) {
		v = strings.TrimSpace(v)
		if len(v) > len(bearerPrefix) && strings.EqualFold(v[:len(bearerPrefix)], bearerPrefix) {
			return strings.TrimSpace(v[len(bearerPrefix):])
		}
	}
	return ""
}
