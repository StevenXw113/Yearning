package engine

import (
	"testing"

	"engine/api"
)

func TestAuditRoleFromAPI(t *testing.T) {
	in := &api.AuditRole{
		MongoForbidMultiWrite:   true,
		MongoForbidAdminCommand: true,
		MongoLargeInList:        10,
		MongoMaxAffectRows:      20,
	}
	got := AuditRoleFromAPI(in)
	if got == nil {
		t.Fatal("AuditRoleFromAPI() = nil")
	}
	if !got.MongoForbidMultiWrite || !got.MongoForbidAdminCommand {
		t.Fatalf("Mongo 规则映射丢失: %#v", got)
	}
	if got.MongoLargeInList != 10 || got.MongoMaxAffectRows != 20 {
		t.Fatalf("Mongo 参数映射错误: large=%d affect=%d", got.MongoLargeInList, got.MongoMaxAffectRows)
	}
	if AuditRoleFromAPI(nil) != nil {
		t.Fatal("nil 输入应返回 nil")
	}
}
