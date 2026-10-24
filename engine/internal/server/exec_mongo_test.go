package server

import (
	"strings"
	"testing"

	"engine/api"
)

// TestMongoURI 认证库（authSource）随数据源配置：默认 admin，显式配置用配置值，
// 含特殊字符的库名走 URL 转义（否则会把连接串拆坏）。
func TestMongoURI(t *testing.T) {
	if u := mongoURI("u", "p", "127.0.0.1", 27017, ""); !strings.Contains(u, "authSource=admin") {
		t.Fatalf("未配置认证库应默认 admin: %s", u)
	}
	if u := mongoURI("u", "p", "127.0.0.1", 27017, "appdb"); !strings.Contains(u, "authSource=appdb") {
		t.Fatalf("显式认证库应生效: %s", u)
	}
	if u := mongoURI("u", "p", "127.0.0.1", 27017, "my db"); !strings.Contains(u, "authSource=my+db") {
		t.Fatalf("含空格的库名应 URL 转义: %s", u)
	}
	if u := mongoURI("u", "p@:/%", "127.0.0.1", 27017, ""); strings.Contains(u, "p@") || strings.Contains(u, "p:") {
		t.Fatalf("口令特殊字符必须转义: %s", u)
	}
}

// TestConnectMongoRejectsEmptyIP IP 为空必须在建连前报错。
func TestConnectMongoRejectsEmptyIP(t *testing.T) {
	if _, err := connectMongo(nil); err == nil || !strings.Contains(err.Error(), "IP") {
		t.Fatalf("nil 数据源应报 IP 为空: %v", err)
	}
	if _, err := connectMongo(&api.DataSource{AuthDb: "appdb"}); err == nil || !strings.Contains(err.Error(), "IP") {
		t.Fatalf("空 IP 应报错: %v", err)
	}
}
