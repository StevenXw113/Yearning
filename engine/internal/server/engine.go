package server

import (
	"context"

	enginev1 "engine/gen/engine/v1"
)

// Engine 实现 engine.v1.EngineService。
// 首期打通 gRPC 通信骨架，各方法先返回占位，待接入 MySQL parser/advisor/executor。
type Engine struct {
	enginev1.UnimplementedEngineServiceServer
}

func NewEngine() *Engine {
	return &Engine{}
}

func (e *Engine) StopDelay(_ context.Context, _ *enginev1.StopDelayRequest) (*enginev1.StopDelayReply, error) {
	// 延迟工单调度需接入 Yearning 元库读取延迟工单并在到点触发 Exec。
	// 该能力依赖引擎配置 Yearning 元库连接与延迟调度器，当前版本暂未接入。
	return &enginev1.StopDelayReply{
		Ok:      false,
		Error:   "延迟调度尚未接入：需配置 Yearning 元库连接与延迟调度器",
		Message: "",
	}, nil
}
