package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"engine/api"
)

// startHTTPTest 起一个真实 HTTP 引擎服务(随机端口),返回 base URL 与关闭函数。
func startHTTPTest(t *testing.T, token string) (string, func()) {
	t.Helper()
	srv := httptest.NewServer(NewHandler(NewEngine(), token))
	return srv.URL, srv.Close
}

// postJSON 向引擎发送业务请求,返回响应与原始 body。
func postJSON(t *testing.T, cli *http.Client, url, token string, v any) (*http.Response, []byte) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rep, err := cli.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer rep.Body.Close()
	body, err := io.ReadAll(rep.Body)
	if err != nil {
		t.Fatal(err)
	}
	return rep, body
}

// TestEngineHTTPWire 全方法线缆端到端:JSON 进出,业务语义与旧 gRPC 版一致。
func TestEngineHTTPWire(t *testing.T) {
	base, stop := startHTTPTest(t, "")
	defer stop()
	cli := &http.Client{Timeout: 10 * time.Second}

	// Check 已实现,应返回 Ok=true 且对合法 SQL 拆分出记录。
	rep, body := postJSON(t, cli, base+"/v1/check", "", &api.CheckRequest{Sql: "SELECT 1; SELECT 2"})
	if rep.StatusCode != http.StatusOK {
		t.Fatalf("Check status=%d body=%s", rep.StatusCode, body)
	}
	var ck api.CheckReply
	if err := json.Unmarshal(body, &ck); err != nil {
		t.Fatalf("decode CheckReply: %v", err)
	}
	if !ck.Ok || len(ck.Records) != 2 {
		t.Fatalf("Check unexpected: ok=%v records=%d body=%s", ck.Ok, len(ck.Records), body)
	}

	// MergeAlterTables:同表 ALTER 合并。
	rep, body = postJSON(t, cli, base+"/v1/merge-alter-tables", "",
		&api.MergeAlterTablesRequest{Sqls: "ALTER TABLE t ADD a INT; ALTER TABLE t ADD b INT"})
	var mg api.MergeAlterTablesReply
	_ = json.Unmarshal(body, &mg)
	if rep.StatusCode != http.StatusOK || !mg.Ok || mg.Sql == "" {
		t.Fatalf("MergeAlterTables unexpected: status=%d body=%s", rep.StatusCode, body)
	}

	// Exec:空工单/无数据源应返回 Ok=false 且不 panic(业务错误走 200+Ok=false)。
	rep, body = postJSON(t, cli, base+"/v1/exec", "",
		&api.ExecRequest{Order: &api.Order{Sql: "SELECT 1"}, Source: &api.DataSource{Ip: "", Port: 3306}})
	var ex api.ExecReply
	_ = json.Unmarshal(body, &ex)
	if rep.StatusCode != http.StatusOK || ex.Ok {
		t.Fatalf("Exec unexpected: status=%d ok=%v body=%s", rep.StatusCode, ex.Ok, body)
	}

	// Query:合法 SQL 拆出语句。
	rep, body = postJSON(t, cli, base+"/v1/query", "",
		&api.QueryRequest{Sql: "SELECT 1; SELECT 2", InsulateWordList: "phone,email"})
	var qr api.QueryReply
	_ = json.Unmarshal(body, &qr)
	if rep.StatusCode != http.StatusOK || !qr.Ok || len(qr.Records) != 2 {
		t.Fatalf("Query unexpected: status=%d ok=%v records=%d body=%s", rep.StatusCode, qr.Ok, len(qr.Records), body)
	}

	// StopDelay:未接入调度,业务失败形态。
	rep, body = postJSON(t, cli, base+"/v1/stop-delay", "", &api.StopDelayRequest{})
	var sd api.StopDelayReply
	_ = json.Unmarshal(body, &sd)
	if rep.StatusCode != http.StatusOK || sd.Ok {
		t.Fatalf("StopDelay unexpected: status=%d ok=%v body=%s", rep.StatusCode, sd.Ok, body)
	}

	// 方法不匹配 → 405(Go 1.22 method routing 默认行为)。
	res, err := http.Get(base + "/v1/check")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET /v1/check 应 405, got %d", res.StatusCode)
	}
}

// TestAuthHTTPEndToEnd 令牌链路端到端:
//   - 业务方法无令牌/错令牌 → 401;对令牌 → 到达业务逻辑
//   - /healthz 不带令牌仍可用(容器探针不会携带令牌)
func TestAuthHTTPEndToEnd(t *testing.T) {
	base, stop := startHTTPTest(t, "s3cret-token")
	defer stop()
	cli := &http.Client{Timeout: 10 * time.Second}

	// 无令牌 → 401
	rep, _ := postJSON(t, cli, base+"/v1/check", "", &api.CheckRequest{Sql: "SELECT 1"})
	if rep.StatusCode != http.StatusUnauthorized {
		t.Fatalf("无令牌应 401, got %d", rep.StatusCode)
	}
	// 错令牌 → 401
	rep, _ = postJSON(t, cli, base+"/v1/check", "wrong", &api.CheckRequest{Sql: "SELECT 1"})
	if rep.StatusCode != http.StatusUnauthorized {
		t.Fatalf("错令牌应 401, got %d", rep.StatusCode)
	}
	// 对令牌 → 到达业务逻辑(200,业务结果非认证错误)
	rep, body := postJSON(t, cli, base+"/v1/check", "s3cret-token", &api.CheckRequest{Sql: "SELECT 1"})
	if rep.StatusCode != http.StatusOK {
		t.Fatalf("对令牌应到达业务逻辑, got %d body=%s", rep.StatusCode, body)
	}
	// 健康检查豁免:探针不带令牌
	res, err := http.Get(base + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("健康检查应豁免认证, got %d", res.StatusCode)
	}
}

// TestAuthHTTPDisabled 令牌为空(未启用认证)时全部放行——纯内网部署的现状兼容。
func TestAuthHTTPDisabled(t *testing.T) {
	base, stop := startHTTPTest(t, "")
	defer stop()
	cli := &http.Client{Timeout: 10 * time.Second}
	rep, body := postJSON(t, cli, base+"/v1/check", "", &api.CheckRequest{Sql: "SELECT 1"})
	if rep.StatusCode != http.StatusOK {
		t.Fatalf("未启用认证时不应拒绝: %d body=%s", rep.StatusCode, body)
	}
}

// TestBearerTokenHTTP 解析形态:Bearer 前缀大小写不敏感、无前缀视为缺失。
func TestBearerTokenHTTP(t *testing.T) {
	header := func(v string) *http.Request {
		req, _ := http.NewRequest(http.MethodPost, "/", nil)
		if v != "" {
			req.Header.Set("Authorization", v)
		}
		return req
	}
	if got := bearerToken(header("bearer  abc ")); got != "abc" {
		t.Fatalf("小写 bearer 也应解析: %q", got)
	}
	if got := bearerToken(header("Bearer abc")); got != "abc" {
		t.Fatalf("标准形态应解析: %q", got)
	}
	if got := bearerToken(header("raw-token")); got != "" {
		t.Fatalf("无 Bearer 前缀应视为缺失: %q", got)
	}
	if got := bearerToken(header("")); got != "" {
		t.Fatalf("无 Header 应返回空: %q", got)
	}
}

// TestBodySizeLimit 消息上限:超过 64MB 的请求体在解码层被拒(400),
// 不允许超大工单把引擎内存打爆。
func TestBodySizeLimit(t *testing.T) {
	base, stop := startHTTPTest(t, "")
	defer stop()
	cli := &http.Client{Timeout: 30 * time.Second}

	huge := &api.CheckRequest{Sql: strings.Repeat("a", 65<<20)}
	b, err := json.Marshal(huge)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, base+"/v1/check", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	rep, err := cli.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer rep.Body.Close()
	if rep.StatusCode != http.StatusBadRequest {
		t.Fatalf("超限请求体应 400, got %d", rep.StatusCode)
	}
}
