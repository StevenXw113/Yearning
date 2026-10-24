package model

// 工单状态机常量（core_sql_orders.status）。
//
// 与前端 front/src/types/index.ts 的 OrderState 枚举一一对应，契约由
// src/archguard 的测试锁死：改这里（或前端）必须同步另一端。
// 放在 model 包（最底层）：audit → flow 已有依赖，常量放 audit 会让
// flow 反向引用成环；所有按状态操作工单的包统一引用此处，禁止裸值。
const (
	OrderStatusRejected  = 0 // 已驳回（前端 OrderState.REJECT）
	OrderStatusFinished  = 1 // 完成（SUCCESS）
	OrderStatusAuditing  = 2 // 审核中（AUDIT）
	OrderStatusExecuting = 3 // 执行中（PROCESS）
	OrderStatusFailed    = 4 // 失败（ERROR）
	OrderStatusWaiting   = 5 // 待执行（WAIT）
	OrderStatusUndo      = 6 // 已撤销（Undo；7 Terminate 为前端预留，后端未使用）
)

// OrderTerminalStatuses 终态集合：归档等批量操作只允许触碰这些状态。
var OrderTerminalStatuses = []int{OrderStatusRejected, OrderStatusFinished, OrderStatusFailed, OrderStatusUndo}
