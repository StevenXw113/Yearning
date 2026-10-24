package apis

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cookieY/yee"
)

func newProbeCore() *yee.Core {
	e := yee.New()
	// 模拟 AddRouter 的关键组合：静态单段路由与探针路由共存
	e.GET("/login", func(c yee.Context) error { return c.String(200, "login") })
	RegisterProbeRoutes(e)
	return e
}

// TestProbeRoutesServe 验证 /:tp 参数路由与 /login 等静态路由共存且互不抢路由：
// gin 内核同层静态优先，但这里用真实请求锁死行为，防止 yee 升级后悄悄改变。
func TestProbeRoutesServe(t *testing.T) {
	e := newProbeCore()

	cases := []struct {
		path   string
		status int
		body   string
	}{
		{"/login", 200, "login"},          // 静态路由不受探针路由影响
		{"/health", 200, `"status":"ok"`}, // 探针
		{"/metrics", 200, "# HELP"},       // Prometheus 文本
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, c.path, nil))
		if w.Code != c.status {
			t.Errorf("GET %s = %d, want %d", c.path, w.Code, c.status)
			continue
		}
		if c.body != "" && !contains(w.Body.String(), c.body) {
			t.Errorf("GET %s body missing %q: %s", c.path, c.body, w.Body.String())
		}
	}
}

// TestHealthBodyShape /health 必须含 engine 状态与 uptime（编排/人看的核心字段）。
func TestHealthBodyShape(t *testing.T) {
	e := newProbeCore()
	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	for _, key := range []string{`"engine"`, `"uptime"`, `"now"`} {
		if !contains(w.Body.String(), key) {
			t.Errorf("/health 缺少字段 %s: %s", key, w.Body.String())
		}
	}
}

// TestMetricsFormat metrics 输出必须是合法的 Prometheus text exposition 骨架。
func TestMetricsFormat(t *testing.T) {
	e := newProbeCore()
	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := w.Body.String()
	for _, key := range []string{
		"yearning_engine_healthy",
		"yearning_engine_rpc_duration_seconds_sum",
		"yearning_process_uptime_seconds",
	} {
		if !contains(body, key) {
			t.Errorf("metrics 缺少指标 %s", key)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
