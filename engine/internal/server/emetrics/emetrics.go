// Package emetrics 引擎侧进程内指标（零依赖，与主程序 src/lib/metrics 同思路）。
// 覆盖引擎自身视角的关键信号：RPC 调用量与延迟、执行中的工单数、内存/goroutine。
package emetrics

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// startTime 进程启动时刻
	startTime = time.Now()

	// RPCTotal RPC 调用计数，label method 与 result
	rpcTotal sync.Map // "Check\x00ok" -> *atomic.Int64
	// RPCDurationNsSum RPC 耗时总和（纳秒）
	rpcDurationNsSum atomic.Int64
	// RPCDurationNsMax RPC 耗时最大值
	rpcDurationNsMax atomic.Int64
	// RPCCalls RPC 调用总次数
	rpcCalls atomic.Int64
	// ActiveExecs 正在执行的 Exec 数（Gauge）
	ActiveExecs atomic.Int64
)

// ObserveRPC 记录一次 RPC 的方法与耗时（成败由调用方分别调 Observe）。
func ObserveRPC(method string, d time.Duration, ok bool) {
	result := "ok"
	if !ok {
		result = "error"
	}
	key := method + "\x00" + result
	if v, ok := rpcTotal.Load(key); ok {
		v.(*atomic.Int64).Add(1)
	} else {
		c := &atomic.Int64{}
		c.Store(1)
		rpcTotal.Store(key, c)
	}
	rpcCalls.Add(1)
	ns := d.Nanoseconds()
	rpcDurationNsSum.Add(ns)
	for {
		cur := rpcDurationNsMax.Load()
		if ns <= cur || rpcDurationNsMax.CompareAndSwap(cur, ns) {
			break
		}
	}
}

// Render 输出 Prometheus text exposition。
func Render() string {
	var b []byte
	append := func(s string) { b = append(b, s...) }

	append("# HELP engine_rpc_total Engine gRPC calls\n# TYPE engine_rpc_total counter\n")
	rpcTotal.Range(func(k, v any) bool {
		parts := splitKey(k.(string))
		append(fmt.Sprintf("engine_rpc_total{method=%q,result=%q} %d\n", parts[0], parts[1], v.(*atomic.Int64).Load()))
		return true
	})
	append("# HELP engine_rpc_duration_seconds Engine gRPC call duration\n# TYPE engine_rpc_duration_seconds summary\n")
	append(fmt.Sprintf("engine_rpc_duration_seconds_count %d\n", rpcCalls.Load()))
	append(fmt.Sprintf("engine_rpc_duration_seconds_sum %f\n", float64(rpcDurationNsSum.Load())/1e9))
	append(fmt.Sprintf("engine_rpc_duration_seconds_max %f\n", float64(rpcDurationNsMax.Load())/1e9))
	append("# HELP engine_active_execs Exec RPCs currently running\n# TYPE engine_active_execs gauge\n")
	append(fmt.Sprintf("engine_active_execs %d\n", ActiveExecs.Load()))
	append("# HELP engine_process_uptime_seconds Process uptime\n# TYPE engine_process_uptime_seconds gauge\n")
	append(fmt.Sprintf("engine_process_uptime_seconds %f\n", time.Since(startTime).Seconds()))
	append("# HELP engine_process_goroutines Current goroutine count\n# TYPE engine_process_goroutines gauge\n")
	append(fmt.Sprintf("engine_process_goroutines %d\n", runtime.NumGoroutine()))
	return string(b)
}

func splitKey(k string) (parts [2]string) {
	for i, r := range []byte(k) {
		if r == 0 {
			parts[0], parts[1] = k[:i], k[i+1:]
			return
		}
	}
	return [2]string{k, ""}
}
