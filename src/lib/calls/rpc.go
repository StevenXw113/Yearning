package calls

import (
	"Yearning-go/src/model"

	enginev1 "engine/gen/engine/v1"

	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"strings"
	"sync"
)

// retryPolicy 让 gRPC 自动重试「引擎瞬时不可用」。
//
// 引擎是独立容器，滚动/替换容器的那一两秒里请求会拿到 UNAVAILABLE。
// 连接是进程级单例（见 Client），grpc.ClientConn 自带断线重连，配合这里的
// UNAVAILABLE 重试即可覆盖容器重启窗口，不需要在调用点写重试循环。
//
// maxAttempts=5 + 指数退避（0.3s 起、上限 2s）≈ 4.5s 重试窗口：
// 足够覆盖容器重启，又不会在引擎真的挂掉时让人干等太久。
const retryPolicy = `{
  "methodConfig": [{
    "name": [{}],
    "retryPolicy": {
      "maxAttempts": 5,
      "initialBackoff": "0.3s",
      "maxBackoff": "2s",
      "backoffMultiplier": 2,
      "retryableStatusCodes": ["UNAVAILABLE"]
    }
  }]
}`

var (
	clientOnce sync.Once
	sharedCli  enginev1.EngineServiceClient
	clientErr  error
)

// Client 返回进程级共享的引擎 gRPC 客户端。
// grpc.ClientConn 线程安全且多路复用，懒连接、断线自动重连——此前每次调用
// NewClient 新建连接再 Close，高频执行/检测场景白付一次拨号握手。
// 连接生命周期与进程一致，调用方不再持有 *grpc.ClientConn、也不需要 Close。
// RpcAddr 语义为审核引擎的 gRPC 监听地址，如 0.0.0.0:13307。
// RpcToken 非空时每个请求自动携带 authorization: Bearer <token>（引擎侧
// 以 -auth-token / ENGINE_AUTH_TOKEN 启用对称校验）。
func Client() (enginev1.EngineServiceClient, error) {
	clientOnce.Do(func() {
		opts := []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithDefaultServiceConfig(retryPolicy),
		}
		if t := strings.TrimSpace(model.C.General.RpcToken); t != "" {
			opts = append(opts, grpc.WithPerRPCCredentials(rpcToken(t)))
		}
		conn, err := grpc.NewClient(model.C.General.RpcAddr, opts...)
		if err != nil {
			clientErr = err
			return
		}
		// 包装一层埋点客户端：全部 RPC 自动记耗时/成败/健康状态（含引擎不可用告警）
		sharedCli = instrumented{enginev1.NewEngineServiceClient(conn)}
	})
	return sharedCli, clientErr
}

// rpcToken 把共享令牌注入每个请求的 metadata。RequireTransportSecurity=false
// 是因为当前传输是内网明文（引擎启用令牌时建议确保网络隔离，mTLS 属后续演进）。
type rpcToken string

func (t rpcToken) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + string(t)}, nil
}

func (t rpcToken) RequireTransportSecurity() bool { return false }

// 确保 rpcToken 实现 PerRPCCredentials（编译期约束）。
var _ credentials.PerRPCCredentials = rpcToken("")
