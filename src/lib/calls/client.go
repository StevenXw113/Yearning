package calls

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"Yearning-go/src/lib/metrics"
	"Yearning-go/src/model"

	engineapi "engine/api"
)

// 重试参数:与旧 gRPC retryPolicy 等价的退避曲线(0.3s 起、×2、上限 2s,
// ≤5 次 ≈ 4.5s 窗口,覆盖引擎容器滚动重启)。仅幂等方法使用,见各方法注释。
const (
	maxAttempts    = 5
	initialBackoff = 300 * time.Millisecond
	maxBackoff     = 2 * time.Second
)

var (
	clientOnce sync.Once
	sharedCli  *EngineClient
	clientErr  error
)

// EngineClient 引擎 HTTP 客户端(JSON 线缆契约见 engine/api)。
//
// 传输层替代原 gRPC ClientConn:连接池复用(http.Transport 自带)、断线重连。
// 重试语义收敛为**按幂等性区分**(修复旧 gRPC retryPolicy 全方法生效的缺口):
//   - Check / Query / MergeAlterTables 幂等,传输失败自动重试 ≤5 次;
//   - Exec 非幂等,绝不自动重试——引擎执行到一半挂掉时重发
//     会让已生效语句二次执行。
type EngineClient struct {
	base    string
	token   string
	http    *http.Client
	backoff func(attempt int) time.Duration
}

// Client 返回进程级共享的引擎客户端。懒初始化,生命周期与进程一致,
// 调用方不需要 Close。
// RpcAddr 语义为审核引擎的 HTTP 监听地址,如 0.0.0.0:13307。
// RpcToken 非空时每个请求自动携带 authorization: Bearer <token>(引擎侧
// 以 -auth-token / ENGINE_AUTH_TOKEN 启用对称校验)。
func Client() (*EngineClient, error) {
	clientOnce.Do(func() {
		tlsCfg, err := rpcTLSConfig()
		if err != nil {
			clientErr = err
			return
		}
		g := model.C.General
		transport := http.DefaultTransport.(*http.Transport).Clone()
		if tlsCfg != nil {
			transport.TLSClientConfig = tlsCfg
		}
		sharedCli = &EngineClient{
			base:    engineBaseURL(g.RpcAddr, tlsCfg != nil),
			token:   strings.TrimSpace(g.RpcToken),
			http:    &http.Client{Transport: transport},
			backoff: defaultBackoff,
		}
	})
	return sharedCli, clientErr
}

// engineBaseURL 归一化引擎地址:无 scheme 时按是否启用 TLS 补 http(s)://。
func engineBaseURL(addr string, withTLS bool) string {
	addr = strings.TrimSuffix(strings.TrimSpace(addr), "/")
	if strings.Contains(addr, "://") {
		return addr
	}
	if withTLS {
		return "https://" + addr
	}
	return "http://" + addr
}

// defaultBackoff 指数退避:0.3s、0.6s、1.2s、2s、2s。
func defaultBackoff(attempt int) time.Duration {
	d := initialBackoff << attempt
	if d > maxBackoff || d <= 0 {
		return maxBackoff
	}
	return d
}

// rpcTLSConfig 按配置构造客户端 TLS(CA 校验引擎身份;再配客户端证书即 mTLS)。
// 未配置 CA 时返回 nil(明文,与引擎侧明文对应,仅限可信内网)。
func rpcTLSConfig() (*tls.Config, error) {
	g := model.C.General
	if strings.TrimSpace(g.RpcTLSCA) == "" {
		return nil, nil
	}
	cfg, err := commonTLSConfig(g.RpcTLSCA, g.RpcTLSCert, g.RpcTLSKey)
	if err != nil {
		return nil, err
	}
	if sn := strings.TrimSpace(g.RpcTLSServerName); sn != "" {
		cfg.ServerName = sn
	}
	return cfg, nil
}

func (c *EngineClient) Check(ctx context.Context, req *engineapi.CheckRequest) (*engineapi.CheckReply, error) {
	return call[*engineapi.CheckReply](ctx, c, true, "Check", "/v1/check", req)
}

func (c *EngineClient) MergeAlterTables(ctx context.Context, req *engineapi.MergeAlterTablesRequest) (*engineapi.MergeAlterTablesReply, error) {
	return call[*engineapi.MergeAlterTablesReply](ctx, c, true, "MergeAlterTables", "/v1/merge-alter-tables", req)
}

func (c *EngineClient) Exec(ctx context.Context, req *engineapi.ExecRequest) (*engineapi.ExecReply, error) {
	return call[*engineapi.ExecReply](ctx, c, false, "Exec", "/v1/exec", req)
}

func (c *EngineClient) Query(ctx context.Context, req *engineapi.QueryRequest) (*engineapi.QueryReply, error) {
	return call[*engineapi.QueryReply](ctx, c, true, "Query", "/v1/query", req)
}

// call 统一计时/埋点/健康归因。业务错误(引擎返回 Ok=false / Error 非空)
// 不算「不可用」——那是审核/执行结果;只有传输失败才告警(见 health.go)。
func call[T any](ctx context.Context, c *EngineClient, retryable bool, method, path string, req any) (T, error) {
	var zero T
	body, err := json.Marshal(req)
	if err != nil {
		return zero, fmt.Errorf("%s 请求编码失败: %w", method, err)
	}

	start := time.Now()
	rep, err := send[T](ctx, c, retryable, method, path, body)
	metrics.EngineRPCDuration.Observe(time.Since(start))

	if err != nil {
		metrics.EngineUnavailableTotal.Inc()
		metrics.EngineRPCTotal.With(method, "error").Inc()
		engineHealthFlipped(false, err.Error())
		return zero, err
	}
	metrics.EngineRPCTotal.With(method, "ok").Inc()
	engineHealthFlipped(true, "")
	return rep, nil
}

// send 单方法传输 + 幂等重试循环。退避等待感知 ctx 取消。
func send[T any](ctx context.Context, c *EngineClient, retryable bool, method, path string, body []byte) (T, error) {
	var zero T
	for attempt := 0; ; attempt++ {
		rep, err := attemptSend[T](ctx, c, method, path, body)
		if err == nil {
			return rep, nil
		}
		if !retryable || !isRetryable(err) || attempt >= maxAttempts-1 {
			return zero, err
		}
		timer := time.NewTimer(c.backoff(attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return zero, ctx.Err()
		case <-timer.C:
		}
	}
}

// attemptSend 单次尝试:POST JSON → 200 解码。任何非 200 或解码失败
// 都归为传输错误(业务失败在 200 体内,不经此处)。
func attemptSend[T any](ctx context.Context, c *EngineClient, method, path string, body []byte) (T, error) {
	var zero T
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(body))
	if err != nil {
		return zero, &transportError{method: method, cause: err}
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return zero, &transportError{method: method, cause: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return zero, &transportError{
			method:    method,
			status:    resp.StatusCode,
			retryable: gatewayRetryable(resp.StatusCode),
			cause:     fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b))),
		}
	}
	var rep T
	if err := json.NewDecoder(resp.Body).Decode(&rep); err != nil {
		return zero, &transportError{method: method, cause: fmt.Errorf("响应解码失败: %w", err)}
	}
	return rep, nil
}

// transportError 传输层失败(连接失败、非 200、解码失败)。
// 与业务错误(200 + Ok=false)严格区分:前者触发健康告警,后者不会。
type transportError struct {
	method    string
	status    int
	retryable bool
	cause     error
}

func (e *transportError) Error() string {
	if e.status != 0 {
		return fmt.Sprintf("引擎返回异常状态(%s): %v", e.method, e.cause)
	}
	return fmt.Sprintf("引擎不可达(%s): %v", e.method, e.cause)
}

func (e *transportError) Unwrap() error { return e.cause }

// gatewayRetryable 网关类 5xx(代理后面的引擎滚动重启窗口)值得重试;
// 500/401/400 属配置或版本问题,重试无益。
func gatewayRetryable(status int) bool {
	return status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

// isRetryable 仅幂等方法会走到这里:连接层失败与网关类 5xx 才重试。
func isRetryable(err error) bool {
	var te *transportError
	if !errors.As(err, &te) {
		return false
	}
	if te.status == 0 {
		return true
	}
	return te.retryable
}
