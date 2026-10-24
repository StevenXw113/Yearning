package apis

import (
	"net/http"
	"time"

	"Yearning-go/src/lib/calls"
	"Yearning-go/src/lib/metrics"

	"github.com/cookieY/yee"
)

// RegisterProbeRoutes 探针与指标端点。挂在 JWT 中间件之外（/api/v2 组外）：
// 容器探针与 Prometheus 不会携带用户令牌。两个端点都只暴露状态与聚合计数，
// 不含任何工单内容/数据源凭据；建议仅在内网可达范围监听。
//
// 用具体路径而非 /:tp 参数路由：yee 的路由树不允许根层参数段与 /login 等
// 静态单段路由共存（注册即 panic）。
func RegisterProbeRoutes(e *yee.Core) {
	e.GET("/health", func(c yee.Context) error {
		engineOK := calls.IsEngineHealthy()
		// 引擎异常仍是「进程活着、依赖降级」：探针语义上不报进程死亡，
		// 让编排系统保留实例（引擎恢复后自动转好），异常由告警通道通知
		_ = engineOK
		return c.JSON(http.StatusOK, map[string]interface{}{
			"status": "ok",
			"engine": map[string]bool{"ok": engineOK},
			"uptime": int64(metrics.UptimeSeconds()),
			"now":    time.Now().Format("2006-01-02 15:04:05"),
		})
	})
	e.GET("/metrics", func(c yee.Context) error {
		c.Response().Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		return c.String(http.StatusOK, metrics.RenderPrometheus())
	})
}
