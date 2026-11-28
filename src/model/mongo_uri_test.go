package model

import (
	"strings"
	"testing"
)

// 回归:无认证 MongoDB 数据源(空用户名)曾生成 mongodb://:@host 的非法 URI,
// mongo 驱动直接报 "username required if URI contains user info",连接测试永远失败。
func TestMongoURINoAuth(t *testing.T) {
	s := &CoreDataSource{IP: "127.0.0.1", Port: 27017}
	uri, err := MongoURI(s, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(uri, "@") {
		t.Fatalf("空用户名不应携带 user info: %s", uri)
	}
	if !strings.Contains(uri, "mongodb://127.0.0.1:27017/") {
		t.Fatalf("URI 形态错误: %s", uri)
	}
}

func TestMongoURIWithAuth(t *testing.T) {
	s := &CoreDataSource{IP: "127.0.0.1", Port: 27017, Username: "yrn", AuthDatabase: "admin"}
	uri, err := MongoURI(s, "pw")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(uri, "yrn:pw@") {
		t.Fatalf("有用户名时应携带凭据: %s", uri)
	}
	if !strings.Contains(uri, "authSource=admin") {
		t.Fatalf("应带 authSource: %s", uri)
	}
}

func TestMongoReplicaConfig(t *testing.T) {
	source := &CoreDataSource{IP: "a:27017,b:27018", Username: "user", AuthDatabase: "accounts", MongoReplicaSet: "rs-test"}
	opts, err := source.MongoConfig("p@ss^word%").Options()
	if err != nil {
		t.Fatal(err)
	}
	if len(opts.Hosts) != 2 || opts.ReplicaSet == nil || *opts.ReplicaSet != source.MongoReplicaSet || opts.Auth.AuthSource != source.AuthDatabase || opts.Auth.Password != "p@ss^word%" {
		t.Fatal("datasource connection settings were lost")
	}
}

func TestMongoTLSConfig(t *testing.T) {
	source := &CoreDataSource{IP: "dds.internal", Username: "user", MongoTLS: true,
		MongoAuthMechanism: "SCRAM-SHA-256", CAFile: "ca-pem", Cert: "cert-pem", KeyFile: "key-pem"}
	c := source.MongoConfig("plaintext")
	if !c.TLS || c.AuthMechanism != "SCRAM-SHA-256" || c.CA != source.CAFile || c.Certificate != source.Cert || c.Key != source.KeyFile || c.Password != "plaintext" {
		t.Fatal("MongoDB TLS/auth configuration lost")
	}
	if _, err := c.Options(); err == nil {
		t.Fatal("invalid CA accepted")
	}
}
