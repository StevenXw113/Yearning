package calls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
)

// commonTLSConfig 构造客户端 TLS 配置：CA 必配（校验引擎服务端身份）；
// cert+key 可选（引擎配 -tls-client-ca 即 mTLS 时必配）。TLS12 下限。
func commonTLSConfig(caFile, certFile, keyFile string) (*tls.Config, error) {
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("读取引擎 CA 失败: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("引擎 CA 文件不含有效 PEM 证书: %s", caFile)
	}
	cfg := &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	if strings.TrimSpace(certFile) != "" && strings.TrimSpace(keyFile) != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("加载客户端证书失败: %w", err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg, nil
}
