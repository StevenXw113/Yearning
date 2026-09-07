package calls

import (
	"Yearning-go/src/model"
	"net/rpc"
)

// NewRpc 返回到审核引擎的 RPC 客户端。
// 调用方必须在使用结束后 Close，否则每次调用都会泄漏一条 TCP 连接。
func NewRpc() (*rpc.Client, error) {
	client, err := rpc.DialHTTP("tcp", model.C.General.RpcAddr)
	if err != nil {
		return nil, err
	}
	return client, nil
}
