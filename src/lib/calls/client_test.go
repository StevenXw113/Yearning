package calls

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"engine/api"
)

const testBackoff = time.Millisecond

// newTestClient 直连 httptest 服务器,绕过单例与全局配置;退避压到 1ms。
func newTestClient(t *testing.T, srv *httptest.Server, token string) *EngineClient {
	t.Helper()
	return &EngineClient{base: srv.URL, token: token, http: srv.Client(), backoff: func(int) time.Duration { return testBackoff }}
}

// TestCheckRetriesOnUnavailable 幂等方法传输失败自动重试:
// 第一次 503,第二次成功 → 调用方拿到正常结果,无需感知抖动。
func TestCheckRetriesOnUnavailable(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(&api.CheckReply{Ok: true, Records: []*api.Record{{Sql: "SELECT 1"}}})
	}))
	defer srv.Close()

	rep, err := newTestClient(t, srv, "").Check(context.Background(), &api.CheckRequest{Sql: "SELECT 1"})
	if err != nil {
		t.Fatalf("重试后应成功: %v", err)
	}
	if !rep.Ok || len(rep.Records) != 1 {
		t.Fatalf("结果形态错误: %+v", rep)
	}
	if attempts.Load() != 2 {
		t.Fatalf("应重试一次(共 2 次尝试), got %d", attempts.Load())
	}
}

// TestExecDoesNotRetry P1 修复:Exec 非幂等,引擎执行到一半挂掉时
// 自动重发会让已生效语句二次执行——必须单次尝试,失败即返回。
func TestExecDoesNotRetry(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv, "").Exec(context.Background(),
		&api.ExecRequest{Order: &api.Order{Sql: "UPDATE t SET a=1"}})
	if err == nil {
		t.Fatal("持续不可用应返回错误")
	}
	if attempts.Load() != 1 {
		t.Fatalf("Exec 绝不能自动重试, got %d 次尝试", attempts.Load())
	}
}

// TestStopDelayDoesNotRetry 同 Exec:非幂等方法不重试。
func TestStopDelayDoesNotRetry(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, _ = newTestClient(t, srv, "").StopDelay(context.Background(), &api.StopDelayRequest{})
	if attempts.Load() != 1 {
		t.Fatalf("StopDelay 不应自动重试, got %d", attempts.Load())
	}
}

// TestIdempotentRetryGivesUp 持续不可用时幂等方法有界放弃(不无限重试)。
func TestIdempotentRetryGivesUp(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv, "").Check(context.Background(), &api.CheckRequest{Sql: "SELECT 1"})
	if err == nil {
		t.Fatal("持续不可用应返回错误")
	}
	if attempts.Load() != maxAttempts {
		t.Fatalf("应在 %d 次后有界放弃, got %d", maxAttempts, attempts.Load())
	}
}

// TestBusinessErrorIsNotTransportFailure 业务错误(Ok=false/Error 非空)
// 是 200 + 业务体:rep 非空、err 为空,健康状态不受影响。
func TestBusinessErrorIsNotTransportFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(&api.CheckReply{Ok: false, Error: "语句 1 审核不通过"})
	}))
	defer srv.Close()

	engineDown.Store(true) // 模拟此前不可用,业务成功应翻转回健康
	rep, err := newTestClient(t, srv, "").Check(context.Background(), &api.CheckRequest{Sql: "DROP TABLE t"})
	if err != nil {
		t.Fatalf("业务错误不应表现为传输错误: %v", err)
	}
	if rep == nil || rep.Ok || rep.Error == "" {
		t.Fatalf("业务结果形态错误: %+v", rep)
	}
	if got := CombineReplyErr(rep, err); got == nil || got.Error() != "语句 1 审核不通过" {
		t.Fatalf("CombineReplyErr 应透出业务错误: %v", got)
	}
	if IsEngineHealthy() != true {
		t.Fatal("业务错误不应让引擎标记为不可用")
	}
}

// TestHealthFlipOnTransportFailure 传输失败翻转健康并告警一次,恢复再翻转。
func TestHealthFlipOnTransportFailure(t *testing.T) {
	var down atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if down.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(&api.CheckReply{Ok: true})
	}))
	defer srv.Close()
	cli := newTestClient(t, srv, "")

	engineDown.Store(false)
	down.Store(true)
	_, err := cli.Query(context.Background(), &api.QueryRequest{Sql: "SELECT 1"})
	if err == nil || IsEngineHealthy() {
		t.Fatalf("传输失败应翻转健康: err=%v healthy=%v", err, IsEngineHealthy())
	}

	down.Store(false)
	rep, err := cli.Query(context.Background(), &api.QueryRequest{Sql: "SELECT 1"})
	if err != nil || rep == nil || !IsEngineHealthy() {
		t.Fatalf("恢复后应翻转回健康: err=%v healthy=%v", err, IsEngineHealthy())
	}
}

// TestAuthTokenHeader 配置令牌后每个请求携带 authorization: Bearer <token>。
func TestAuthTokenHeader(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(&api.CheckReply{Ok: true})
	}))
	defer srv.Close()

	if _, err := newTestClient(t, srv, "tok-123").Check(context.Background(), &api.CheckRequest{Sql: "SELECT 1"}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "Bearer tok-123" {
		t.Fatalf("Authorization 头形态错误: %q", got)
	}
}

// TestTransportErrorClassification 传输错误分类:网关类 5xx 可重试,401/500 不重试。
func TestTransportErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		status    int
		retryable bool
	}{{503, true}, {502, true}, {504, true}, {500, false}, {401, false}, {400, false}} {
		if got := gatewayRetryable(tc.status); got != tc.retryable {
			t.Fatalf("HTTP %d gatewayRetryable=%v, want %v", tc.status, got, tc.retryable)
		}
	}
	// 连接层失败(引擎进程死亡/容器重启窗口)可重试——仅幂等方法会走到这里
	if !isRetryable(&transportError{method: "Check", cause: errors.New("connection refused")}) {
		t.Fatal("连接层失败应可重试")
	}
	// 非网关 5xx(如 500)不可重试
	if isRetryable(&transportError{method: "Check", status: 500}) {
		t.Fatal("500 不应重试")
	}
}
