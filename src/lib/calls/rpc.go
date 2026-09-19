package calls

import (
	"Yearning-go/src/model"

	enginev1 "engine/gen/engine/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// retryPolicy 让 gRPC 自动重试「引擎瞬时不可用」。
//
// 引擎是独立容器，滚动/替换容器的那一两秒里请求会拿到 UNAVAILABLE；
// 主程序每次请求都新建连接，重试时会重新拨号，正好覆盖这个窗口，
// 否则审核人得自己再点一次「SQL检测」。这里用 gRPC 原生的重试策略，
// 不需要在调用点写重试循环。
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

// NewClient 返回到审核引擎的 gRPC 客户端与连接。
// 调用方必须在使用结束后 conn.Close()，否则会泄漏一条 gRPC 连接。
// RpcAddr 语义为审核引擎的 gRPC 监听地址，如 0.0.0.0:13307。
func NewClient() (enginev1.EngineServiceClient, *grpc.ClientConn, error) {
	conn, err := grpc.NewClient(model.C.General.RpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(retryPolicy),
	)
	if err != nil {
		return nil, nil, err
	}
	return enginev1.NewEngineServiceClient(conn), conn, nil
}
