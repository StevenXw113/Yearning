package calls

import (
	"Yearning-go/src/model"

	enginev1 "engine/gen/engine/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// NewClient 返回到审核引擎的 gRPC 客户端与连接。
// 调用方必须在使用结束后 conn.Close()，否则会泄漏一条 gRPC 连接。
// RpcAddr 语义为审核引擎的 gRPC 监听地址，如 0.0.0.0:13307。
func NewClient() (enginev1.EngineServiceClient, *grpc.ClientConn, error) {
	conn, err := grpc.NewClient(model.C.General.RpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}
	return enginev1.NewEngineServiceClient(conn), conn, nil
}
