package model

import (
	"strings"
	"testing"
)

// 回归:无认证 MongoDB 数据源(空用户名)曾生成 mongodb://:@host 的非法 URI,
// mongo 驱动直接报 "username required if URI contains user info",连接测试永远失败。
func TestMongoURINoAuth(t *testing.T) {
	s := &CoreDataSource{IP: "127.0.0.1", Port: 27017}
	uri := MongoURI(s, "")
	if strings.Contains(uri, "@") {
		t.Fatalf("空用户名不应携带 user info: %s", uri)
	}
	if !strings.Contains(uri, "mongodb://127.0.0.1:27017/") {
		t.Fatalf("URI 形态错误: %s", uri)
	}
}

func TestMongoURIWithAuth(t *testing.T) {
	s := &CoreDataSource{IP: "127.0.0.1", Port: 27017, Username: "yrn", AuthDatabase: "admin"}
	uri := MongoURI(s, "pw")
	if !strings.Contains(uri, "yrn:pw@") {
		t.Fatalf("有用户名时应携带凭据: %s", uri)
	}
	if !strings.Contains(uri, "authSource=admin") {
		t.Fatalf("应带 authSource: %s", uri)
	}
}
