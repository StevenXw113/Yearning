package server

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"engine/api"
	"engine/internal/server/emetrics"
)

// maxMsgSize 单条请求/响应上限:默认 HTTP 中间件 1MB 装不下长 INSERT 工单
// (大 VALUES 批次),放宽到 64MB;再大的工单本就该拆分提交。
const maxMsgSize = 64 << 20

// bearerPrefix Bearer 方案前缀(大小写不敏感解析,见 bearerToken)。
const bearerPrefix = "bearer "

// NewHandler 构建引擎 HTTP 处理器:业务路由 + Bearer 令牌认证 + 计时埋点。
// 替代原 gRPC 骨架(EngineService/健康检查/认证与统计拦截器)。
//
// 认证语义与原 interceptor.go 一致:token 为空 = 不启用认证(纯内网部署
// 现状兼容);/healthz 豁免认证(容器探针不会携带令牌)。业务错误
// (Ok=false/Error 非空)走 200 + 业务体,HTTP 状态码只表达协议层失败。
func NewHandler(eng *Engine, token string) http.Handler {
	h := &handler{eng: eng}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/check", timed("Check", h.check))
	mux.HandleFunc("POST /v1/merge-alter-tables", timed("MergeAlterTables", h.mergeAlterTables))
	mux.HandleFunc("POST /v1/exec", timed("Exec", h.exec))
	mux.HandleFunc("POST /v1/query", timed("Query", h.query))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	if token == "" {
		return mux
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			mux.ServeHTTP(w, r)
			return
		}
		if subtle.ConstantTimeCompare([]byte(bearerToken(r)), []byte(token)) != 1 {
			writeError(w, http.StatusUnauthorized,
				"无效或缺失的引擎访问令牌(需 Header authorization: Bearer <token>)")
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// handler 业务方法的薄壳:解码请求 → 调用实现 → 写回响应。
type handler struct{ eng *Engine }

func (h *handler) check(w http.ResponseWriter, r *http.Request) {
	var req api.CheckRequest
	if !decode(w, r, &req) {
		return
	}
	rep, err := h.eng.Check(r.Context(), &req)
	reply(w, rep, err)
}

func (h *handler) mergeAlterTables(w http.ResponseWriter, r *http.Request) {
	var req api.MergeAlterTablesRequest
	if !decode(w, r, &req) {
		return
	}
	rep, err := h.eng.MergeAlterTables(r.Context(), &req)
	reply(w, rep, err)
}

func (h *handler) exec(w http.ResponseWriter, r *http.Request) {
	var req api.ExecRequest
	if !decode(w, r, &req) {
		return
	}
	rep, err := h.eng.Exec(r.Context(), &req)
	reply(w, rep, err)
}

func (h *handler) query(w http.ResponseWriter, r *http.Request) {
	var req api.QueryRequest
	if !decode(w, r, &req) {
		return
	}
	rep, err := h.eng.Query(r.Context(), &req)
	reply(w, rep, err)
}

// decode 请求体解码:64MB 上限 + JSON。失败即 400,不再进入业务逻辑。
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxMsgSize)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return false
	}
	return true
}

// reply 业务响应:实现层返回 error 属内部失败(500);业务失败(审核不通过、
// 执行出错)由实现层以 rep.Ok=false / rep.Error 表达,照常 200 返回。
func reply(w http.ResponseWriter, rep any, err error) {
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(rep)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// timed 计时埋点:统计业务处理本身的耗时(认证通过之后,与原拦截器一致)。
type statusWriter struct {
	http.ResponseWriter
	code int
}

func (s *statusWriter) WriteHeader(c int) {
	s.code = c
	s.ResponseWriter.WriteHeader(c)
}

func timed(method string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		next(sw, r)
		emetrics.ObserveRPC(method, time.Since(start), sw.code < http.StatusInternalServerError)
	}
}

// bearerToken 从请求 Header 提取 Bearer 令牌;缺失或方案不符返回空。
func bearerToken(r *http.Request) string {
	v := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(v) > len(bearerPrefix) && strings.EqualFold(v[:len(bearerPrefix)], bearerPrefix) {
		return strings.TrimSpace(v[len(bearerPrefix):])
	}
	return ""
}
