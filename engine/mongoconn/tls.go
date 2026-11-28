package mongoconn

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"strings"
)

// Certificates are PEM contents supplied with the datasource, not server paths.
func (c Config) tlsConfig() (*tls.Config, error) {
	config := &tls.Config{MinVersion: tls.VersionTLS12}
	if strings.TrimSpace(c.CA) != "" {
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM([]byte(c.CA)) {
			return nil, fmt.Errorf("MongoDB CA 证书必须为有效的 PEM 证书")
		}
		config.RootCAs = roots
	}
	cert, key := strings.TrimSpace(c.Certificate), strings.TrimSpace(c.Key)
	if (cert == "") != (key == "") {
		return nil, fmt.Errorf("MongoDB 客户端证书与私钥必须同时填写")
	}
	if cert != "" {
		pair, err := tls.X509KeyPair([]byte(cert), []byte(key))
		if err != nil {
			return nil, fmt.Errorf("MongoDB 客户端证书或私钥无效或不匹配")
		}
		config.Certificates = []tls.Certificate{pair}
	}
	return config, nil
}
