package server

import (
	"context"

	"engine/api"
)

// Engine 实现引擎业务方法(审核/执行/查询等,原 gRPC EngineService)。
type Engine struct{}

func NewEngine() *Engine {
	return &Engine{}
}

func (e *Engine) StopDelay(_ context.Context, _ *api.StopDelayRequest) (*api.StopDelayReply, error) {
	// 延迟工单调度需接入 Yearning 元库读取延迟工单并在到点触发 Exec。
	// 该能力依赖引擎配置 Yearning 元库连接与延迟调度器，当前版本暂未接入。
	return &api.StopDelayReply{
		Ok:      false,
		Error:   "延迟调度尚未接入：需配置 Yearning 元库连接与延迟调度器",
		Message: "",
	}, nil
}
