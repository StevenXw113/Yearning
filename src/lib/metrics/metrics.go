// Package metrics 提供进程内指标采集与 Prometheus 文本格式导出。
//
// 刻意零依赖：需要的只是原子计数器 + text exposition（Prometheus 的文本
// 协议很简单），不值得为此引入 client_golang 及其 prometheus/common 一串
// 传递依赖。所有指标原子操作，无锁读。
package metrics

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

// startTime 进程启动时刻（/health 的 uptime 用）
var startTime = time.Now()

// UptimeSeconds 进程已运行秒数
func UptimeSeconds() float64 {
	return time.Since(startTime).Seconds()
}

// Counter 单调递增计数器
type Counter struct{ v atomic.Int64 }

func (c *Counter) Inc()         { c.v.Add(1) }
func (c *Counter) Add(n int64)  { c.v.Add(n) }
func (c *Counter) Value() int64 { return c.v.Load() }

// LatencyRecorder 记录耗时分布：次数 / 总和 / 最大值 / 最近一次。
// 不做分桶直方图（客户端聚合 avg/p99-in-promql 足够），保持实现最小。
type LatencyRecorder struct {
	count atomic.Int64
	sumNs atomic.Int64
	maxNs atomic.Int64
	last  atomic.Int64 // 最近一次耗时（纳秒）
}

// Observe 记录一次耗时
func (l *LatencyRecorder) Observe(d time.Duration) {
	ns := d.Nanoseconds()
	l.count.Add(1)
	l.sumNs.Add(ns)
	for {
		cur := l.maxNs.Load()
		if ns <= cur || l.maxNs.CompareAndSwap(cur, ns) {
			break
		}
	}
	l.last.Store(ns)
}

func (l *LatencyRecorder) Count() int64  { return l.count.Load() }
func (l *LatencyRecorder) Sum() float64  { return float64(l.sumNs.Load()) / 1e9 }
func (l *LatencyRecorder) Max() float64  { return float64(l.maxNs.Load()) / 1e9 }
func (l *LatencyRecorder) Last() float64 { return float64(l.last.Load()) / 1e9 }

// ---- 主程序指标 ----

var (
	// OrdersTotal 工单执行计数，label: type=dml|ddl, result=success|failed
	OrdersTotal = newLabeledCounter("yearning_orders_total", "type", "result")
	// OrderExecDuration 工单执行总耗时（引擎 Exec 全程，不含 gh-ost）
	OrderExecDuration LatencyRecorder
	// EngineRPCDuration 引擎 gRPC 调用耗时（Check/Exec/Query 全部）
	EngineRPCDuration LatencyRecorder
	// EngineRPCTotal 引擎 gRPC 调用计数，label: method, result=ok|error
	EngineRPCTotal = newLabeledCounter("yearning_engine_rpc_total", "method", "result")
	// EngineUnavailableTotal 引擎不可用（连接失败/UNAVAILABLE）次数
	EngineUnavailableTotal Counter
	// QueryStatementsTotal 查询页执行的语句数
	QueryStatementsTotal Counter
	// EngineHealthy 引擎健康状态 Gauge：1 健康 0 异常（由 calls 包装器维护）
	EngineHealthy atomic.Int32
)

func init() { EngineHealthy.Store(1) }

// labeledCounter 带固定 label 组合的计数器组（label 值在注册时声明，
// 不做动态 label——避免无界基数）。
type labeledCounter struct {
	name   string
	labels []string
	values map[string]*Counter
}

func newLabeledCounter(name string, labels ...string) *labeledCounter {
	return &labeledCounter{name: name, labels: labels, values: map[string]*Counter{}}
}

// With 返回指定 label 组合的计数器（并发安全，首次惰性创建）。
func (lc *labeledCounter) With(labelValues ...string) *Counter {
	key := strings.Join(labelValues, "\x00")
	if c, ok := lc.values[key]; ok {
		return c
	}
	c := &Counter{}
	lc.values[key] = c
	return c
}

// ---- 导出 ----

// RenderPrometheus 输出全部指标的 Prometheus text exposition 格式。
func RenderPrometheus() string {
	var b strings.Builder
	writeCounter := func(name, help string, v int64, labels string) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s counter\n%s%s %d\n", name, help, name, name, labels, v)
	}

	// 工单执行：type/result 两个 label 的笛卡尔组合按实际出现输出
	for _, combo := range OrdersTotal.sortedKeys() {
		writeCounter(OrdersTotal.name, "Yearning order executions",
			OrdersTotal.values[combo].Value(), comboPromLabels(OrdersTotal.labels, combo))
	}
	for _, combo := range EngineRPCTotal.sortedKeys() {
		writeCounter(EngineRPCTotal.name, "Engine gRPC calls",
			EngineRPCTotal.values[combo].Value(), comboPromLabels(EngineRPCTotal.labels, combo))
	}

	writeCounter("yearning_engine_unavailable_total", "Engine unreachable events", EngineUnavailableTotal.Value(), "")
	writeCounter("yearning_query_statements_total", "Query statements executed via query page", QueryStatementsTotal.Value(), "")

	// 耗时（summary 形态：count/sum/max/last）
	writeSummary(&b, "yearning_order_exec_duration_seconds", "Order execution duration (engine Exec)", &OrderExecDuration)
	writeSummary(&b, "yearning_engine_rpc_duration_seconds", "Engine gRPC call duration", &EngineRPCDuration)

	// Gauge
	healthy := EngineHealthy.Load()
	fmt.Fprintf(&b, "# HELP yearning_engine_healthy Engine reachable (1) or not (0)\n# TYPE yearning_engine_healthy gauge\nyearning_engine_healthy %d\n", healthy)

	// 进程基础指标
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	fmt.Fprintf(&b, "# HELP yearning_process_uptime_seconds Process uptime\n# TYPE yearning_process_uptime_seconds gauge\nyearning_process_uptime_seconds %f\n", UptimeSeconds())
	fmt.Fprintf(&b, "# HELP yearning_process_goroutines Current goroutine count\n# TYPE yearning_process_goroutines gauge\nyearning_process_goroutines %d\n", runtime.NumGoroutine())
	fmt.Fprintf(&b, "# HELP yearning_process_heap_bytes Heap in use\n# TYPE yearning_process_heap_bytes gauge\nyearning_process_heap_bytes %d\n", ms.HeapInuse)
	return b.String()
}

func writeSummary(b *strings.Builder, name, help string, l *LatencyRecorder) {
	fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s summary\n", name, help, name)
	fmt.Fprintf(b, "%s_count %d\n", name, l.Count())
	fmt.Fprintf(b, "%s_sum %f\n", name, l.Sum())
	fmt.Fprintf(b, "%s_max %f\n", name, l.Max())
	fmt.Fprintf(b, "%s_last %f\n", name, l.Last())
}

// sortedKeys 稳定输出顺序（label 组合 map 无序）。
func (lc *labeledCounter) sortedKeys() []string {
	keys := make([]string, 0, len(lc.values))
	for k := range lc.values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// comboPromLabels 把 "\x00" 连接的 key 还原成 Prometheus label 序列。
func comboPromLabels(labels []string, combo string) string {
	vals := strings.Split(combo, "\x00")
	parts := make([]string, 0, len(vals))
	for i, v := range vals {
		name := "label"
		if i < len(labels) {
			name = labels[i]
		}
		parts = append(parts, fmt.Sprintf("%s=%q", name, v))
	}
	if len(parts) == 0 {
		return ""
	}
	return "{" + strings.Join(parts, ",") + "}"
}
