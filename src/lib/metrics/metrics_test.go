package metrics

import (
	"strings"
	"testing"
	"time"
)

// TestLabeledCounter 同一 label 组合累加、不同组合独立计数。
func TestLabeledCounter(t *testing.T) {
	lc := newLabeledCounter("test_total", "type", "result")
	lc.With("dml", "success").Inc()
	lc.With("dml", "success").Add(2)
	lc.With("dml", "failed").Inc()
	if got := lc.With("dml", "success").Value(); got != 3 {
		t.Fatalf("dml/success 应为 3: %d", got)
	}
	if got := lc.With("dml", "failed").Value(); got != 1 {
		t.Fatalf("dml/failed 应为 1: %d", got)
	}
}

// TestLatencyRecorder 耗时记录：count/sum/max/last 的基本语义。
func TestLatencyRecorder(t *testing.T) {
	var l LatencyRecorder
	l.Observe(100 * time.Millisecond)
	l.Observe(300 * time.Millisecond)
	if l.Count() != 2 {
		t.Fatalf("count 应为 2: %d", l.Count())
	}
	if l.Sum() < 0.399 || l.Sum() > 0.401 {
		t.Fatalf("sum 应约 0.4: %f", l.Sum())
	}
	if l.Max() < 0.299 || l.Max() > 0.301 {
		t.Fatalf("max 应约 0.3: %f", l.Max())
	}
	if l.Last() < 0.299 || l.Last() > 0.301 {
		t.Fatalf("last 应约 0.3: %f", l.Last())
	}
}

// TestRenderPrometheus 输出骨架：每个声明的指标都有 HELP/TYPE/样本行，
// label 组合正确渲染且输出稳定（排序）。
func TestRenderPrometheus(t *testing.T) {
	OrdersTotal.With("dml", "success").Add(7)
	OrdersTotal.With("dml", "failed").Inc()
	EngineRPCTotal.With("Exec", "ok").Add(5)
	EngineUnavailableTotal.Inc()
	OrderExecDuration.Observe(250 * time.Millisecond)

	out := RenderPrometheus()
	for _, want := range []string{
		`# TYPE yearning_orders_total counter`,
		`yearning_orders_total{type="dml",result="success"} 7`,
		`yearning_orders_total{type="dml",result="failed"} 1`,
		`yearning_engine_rpc_total{method="Exec",result="ok"} 5`,
		`yearning_engine_unavailable_total 1`,
		`yearning_order_exec_duration_seconds_count 1`,
		`yearning_order_exec_duration_seconds_sum 0.25`,
		`yearning_engine_healthy 1`,
		`yearning_process_uptime_seconds`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("输出缺少 %q:\n%s", want, out)
		}
	}
}
