package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"engine/internal/server"
	"engine/internal/server/emetrics"
)

func main() {
	addr := flag.String("addr", ":13307", "HTTP 监听地址，如 0.0.0.0:13307")
	// metricsAddr Prometheus 指标端点：独立 HTTP 口，0 = 关闭。仅暴露聚合计数
	// 与进程状态，无业务数据；建议绑内网口。
	metricsAddr := flag.String("metrics-addr", "", "Prometheus /metrics 监听地址（如 :13308），空 = 关闭")
	// authToken 共享访问令牌：非空时所有业务方法要求 Header
	// authorization: Bearer <token>（健康检查 /healthz 豁免）。
	// 引擎是「拿到数据源凭据即可执行 SQL」的代理，跨不完全可信网络部署必须启用。
	authToken := flag.String("auth-token", "", "共享访问令牌（也可用环境变量 ENGINE_AUTH_TOKEN 注入）；空 = 不启用认证")
	// TLS / mTLS：配 cert+key 即启用 TLS（口令与行数据不再明文过网）；
	// 再配 client-ca 则同时要求客户端证书（mTLS，双向身份）。
	// 与共享令牌可叠加：mTLS 管传输与身份，令牌管应用层授权。
	tlsCert := flag.String("tls-cert", "", "服务端证书 PEM 路径（与 -tls-key 同配启用 TLS）")
	tlsKey := flag.String("tls-key", "", "服务端私钥 PEM 路径")
	tlsClientCA := flag.String("tls-client-ca", "", "客户端证书 CA PEM 路径（启用 mTLS，要求客户端证书）")
	flag.Parse()

	token := strings.TrimSpace(*authToken)
	if token == "" {
		token = strings.TrimSpace(os.Getenv("ENGINE_AUTH_TOKEN"))
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           server.NewHandler(server.NewEngine(), token),
		ReadHeaderTimeout: 5 * time.Second,
	}
	if *tlsCert != "" && *tlsKey != "" {
		tlsCfg, err := server.LoadServerTLS(*tlsCert, *tlsKey, *tlsClientCA)
		if err != nil {
			log.Fatalf("TLS 配置加载失败: %v", err)
		}
		log.Printf("engine TLS: enabled (client cert verification: %v)", *tlsClientCA != "")
		srv.TLSConfig = tlsCfg
	}
	if token != "" {
		log.Printf("engine auth: enabled (shared token)")
	} else {
		log.Printf("engine auth: DISABLED (no token configured; safe only on fully trusted networks)")
	}

	if *metricsAddr != "" {
		go serveMetrics(*metricsAddr)
	}

	go func() {
		log.Printf("engine http server listening on %s", *addr)
		var err error
		if srv.TLSConfig != nil {
			err = srv.ListenAndServeTLS("", "")
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down engine")
	// 等价于原 grpc GracefulStop:等待在途请求完成;TLS 证书由 TLSConfig 携带。
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

// serveMetrics 独立 HTTP 口暴露 /metrics（Prometheus text exposition）。
func serveMetrics(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(emetrics.Render()))
	})
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("engine metrics listening on %s/metrics", addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Printf("metrics server stopped: %v", err)
	}
}
