package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	enginev1 "engine/gen/engine/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// 生成自签 CA / 服务端证书 / 客户端证书（测试专用，内存生成落临时目录）。
func genTLSArtifacts(t *testing.T, dir string) (serverCert, serverKey, clientCert, clientKey, caPath string) {
	t.Helper()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER, _ := x509.CreateCertificate(rand.Reader, caTpl, caTpl, &caKey.PublicKey, caKey)

	issue := func(cn string, isServer bool) (string, string) {
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		tpl := &x509.Certificate{
			SerialNumber: big.NewInt(time.Now().UnixNano()),
			Subject:      pkix.Name{CommonName: cn},
			IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().Add(time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		}
		if isServer {
			tpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
			tpl.DNSNames = []string{"127.0.0.1"}
		} else {
			tpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
		}
		der, _ := x509.CreateCertificate(rand.Reader, tpl, mustCert(t, caDER), &key.PublicKey, caKey)
		certPath := filepath.Join(dir, cn+".crt")
		keyPath := filepath.Join(dir, cn+".key")
		writePEM(t, certPath, "CERTIFICATE", der)
		writePEM(t, keyPath, "PRIVATE KEY", mustMarshalPK8(t, key))
		return certPath, keyPath
	}

	caPath = filepath.Join(dir, "ca.crt")
	writePEM(t, caPath, "CERTIFICATE", caDER)
	serverCert, serverKey = issue("server", true)
	clientCert, clientKey = issue("client", false)
	return
}

func mustCert(t *testing.T, der []byte) *x509.Certificate {
	t.Helper()
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func mustMarshalPK8(t *testing.T, k *ecdsa.PrivateKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func writePEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	_ = pem.Encode(f, &pem.Block{Type: blockType, Bytes: der})
}

// TestMTLSEndToEnd mTLS 端到端：引擎要求并校验客户端证书——
// 带证书的客户端可用，不带/错 CA 的客户端被拒。
func TestMTLSEndToEnd(t *testing.T) {
	dir := t.TempDir()
	serverCert, serverKey, clientCert, clientKey, caPath := genTLSArtifacts(t, dir)

	tlsCfg, err := LoadServerTLS(serverCert, serverKey, caPath)
	if err != nil {
		t.Fatalf("LoadServerTLS: %v", err)
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsCfg)))
	enginev1.RegisterEngineServiceServer(s, NewEngine())
	go func() { _ = s.Serve(lis) }()
	defer s.Stop()

	clientPool := x509.NewCertPool()
	clientPool.AppendCertsFromPEM(mustRead(t, caPath))

	// 带客户端证书：可用
	cert, err := tls.LoadX509KeyPair(clientCert, clientKey)
	if err != nil {
		t.Fatal(err)
	}
	good, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(
		credentials.NewTLS(&tls.Config{RootCAs: clientPool, Certificates: []tls.Certificate{cert}, ServerName: "127.0.0.1"})))
	if err != nil {
		t.Fatal(err)
	}
	defer good.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := enginev1.NewEngineServiceClient(good).Check(ctx, &enginev1.CheckRequest{Sql: "SELECT 1"}); err != nil {
		t.Fatalf("持有效客户端证书应可用: %v", err)
	}

	// 不带客户端证书：握手被拒
	anon, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(
		credentials.NewTLS(&tls.Config{RootCAs: clientPool, ServerName: "127.0.0.1"})))
	if err != nil {
		t.Fatal(err)
	}
	defer anon.Close()
	if _, err := enginev1.NewEngineServiceClient(anon).Check(ctx, &enginev1.CheckRequest{Sql: "SELECT 1"}); err == nil {
		t.Fatal("无客户端证书应被 mTLS 拒绝")
	}
}

// TestPlainStillWorks 引擎未配 TLS 时明文客户端照常可用（内网现状兼容）。
func TestPlainStillWorks(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := grpc.NewServer()
	enginev1.RegisterEngineServiceServer(s, NewEngine())
	go func() { _ = s.Serve(lis) }()
	defer s.Stop()
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := enginev1.NewEngineServiceClient(conn).Check(ctx, &enginev1.CheckRequest{Sql: "SELECT 1"}); err != nil {
		t.Fatalf("明文模式不应受影响: %v", err)
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
