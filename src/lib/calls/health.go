package calls

import (
	"sync/atomic"

	"Yearning-go/src/lib/metrics"
	"Yearning-go/src/lib/pusher"

	"github.com/cookieY/yee/logger"
)

// engineDown 引擎当前是否处于「不可用」状态（用于健康翻转告警：只在
// 健康→异常、异常→恢复各通知一次，防止每次请求都告警形成风暴）。
var engineDown atomic.Bool

// engineHealthFlipped 健康状态翻转处理：变化时更新 Gauge 并告警一次。
// 判定由 client.go 的 call() 归因:仅传输失败(连接失败/非 200/解码失败)
// 记为不可用;业务错误(Ok=false)是审核/执行结果,不代表引擎不可用。
func engineHealthFlipped(healthy bool, reason string) {
	if healthy {
		metrics.EngineHealthy.Store(1)
		if engineDown.CompareAndSwap(true, false) {
			logger.DefaultLogger.Info("审核引擎恢复可用")
			pusher.SystemAlert("审核引擎恢复", "引擎 HTTP 通道已恢复可用。")
		}
		return
	}
	metrics.EngineHealthy.Store(0)
	if engineDown.CompareAndSwap(false, true) {
		logger.DefaultLogger.Errorf("审核引擎不可用: %s", reason)
		pusher.SystemAlert("审核引擎不可用", "引擎调用失败："+reason+"。工单执行/SQL 检测将失败，请检查引擎容器状态。")
	}
}

// IsEngineHealthy 引擎最近一次调用是否成功（/health 用）。
func IsEngineHealthy() bool { return engineDown.Load() == false }
