package server

// Engine 实现引擎业务方法(审核/执行/查询等,原 gRPC EngineService)。
type Engine struct{}

func NewEngine() *Engine {
	return &Engine{}
}
