package mongoconn

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testCertificates(t *testing.T, expired bool) (string, string, string) {
	t.Helper()
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	root := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	rootDER, err := x509.CreateCertificate(rand.Reader, root, root, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: []string{"dds.internal"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}}
	if expired {
		leaf.NotAfter = time.Now().Add(-time.Minute)
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, root, &key.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})),
		string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
}

func TestTLSConfiguration(t *testing.T) {
	ca, cert, key := testCertificates(t, false)
	cases := []struct {
		name    string
		config  Config
		invalid bool
	}{
		{"system roots", Config{TLS: true}, false},
		{"custom CA", Config{TLS: true, CA: ca}, false},
		{"client certificate", Config{TLS: true, CA: ca, Certificate: cert, Key: key}, false},
		{"invalid CA", Config{TLS: true, CA: "private-invalid-ca"}, true},
		{"certificate without key", Config{TLS: true, Certificate: cert}, true},
		{"key without certificate", Config{TLS: true, Key: key}, true},
		{"invalid key pair", Config{TLS: true, Certificate: cert, Key: "private-invalid-key"}, true},
		{"TLS disabled preserves legacy settings", Config{CA: "unused", Key: "unused"}, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.config
			c.Hosts = "localhost"
			opts, err := c.Options()
			if (err != nil) != tt.invalid {
				t.Fatalf("error = %v, want error=%v", err, tt.invalid)
			}
			if err != nil {
				if strings.Contains(err.Error(), "private-invalid") || strings.Contains(err.Error(), "-----BEGIN") {
					t.Fatal("certificate material leaked")
				}
				return
			}
			if !c.TLS {
				if opts.TLSConfig != nil {
					t.Fatal("legacy connection unexpectedly uses TLS")
				}
				return
			}
			if opts.TLSConfig == nil || opts.TLSConfig.MinVersion < tls.VersionTLS12 || opts.TLSConfig.InsecureSkipVerify {
				t.Fatal("TLS verification is not strict")
			}
			if c.Certificate != "" && len(opts.TLSConfig.Certificates) != 1 {
				t.Fatal("client certificate missing")
			}
		})
	}
}

func TestTLSHandshakeVerification(t *testing.T) {
	ca, cert, key := testCertificates(t, false)
	wrongCA, _, _ := testCertificates(t, false)
	expiredCA, expiredCert, expiredKey := testCertificates(t, true)
	cases := []struct {
		name, ca, cert, key, serverName string
		clientCert, wantOK              bool
	}{
		{"trusted hostname", ca, cert, key, "dds.internal", false, true},
		{"hostname mismatch", ca, cert, key, "127.0.0.1", false, false},
		{"untrusted CA", wrongCA, cert, key, "dds.internal", false, false},
		{"expired certificate", expiredCA, expiredCert, expiredKey, "dds.internal", false, false},
		{"mutual TLS", ca, cert, key, "dds.internal", true, true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			pair, err := tls.X509KeyPair([]byte(tt.cert), []byte(tt.key))
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
			server.Config.ErrorLog = log.New(io.Discard, "", 0)
			server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{pair}}
			c := Config{Hosts: "localhost", TLS: true, CA: tt.ca}
			if tt.clientCert {
				roots := x509.NewCertPool()
				roots.AppendCertsFromPEM([]byte(ca))
				server.TLS.ClientAuth = tls.RequireAndVerifyClientCert
				server.TLS.ClientCAs = roots
				c.Certificate, c.Key = cert, key
			}
			server.StartTLS()
			defer server.Close()
			opts, err := c.Options()
			if err != nil {
				t.Fatal(err)
			}
			opts.TLSConfig.ServerName = tt.serverName
			client := &http.Client{Transport: &http.Transport{TLSClientConfig: opts.TLSConfig}, Timeout: 3 * time.Second}
			defer client.CloseIdleConnections()
			resp, err := client.Get(server.URL)
			if (err == nil) != tt.wantOK {
				t.Fatalf("TLS handshake error = %v, want success=%v", err, tt.wantOK)
			}
			if resp != nil {
				resp.Body.Close()
			}
		})
	}
}
