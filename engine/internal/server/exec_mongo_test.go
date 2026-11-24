package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"engine/api"
	mongoe "engine/mongo"
	"engine/ruleset"
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

// An enabled change rule must stop the order before connecting to MongoDB.
func TestExecMongoRechecksMultiWrite(t *testing.T) {
	var e Engine
	req := &api.ExecRequest{
		Order:  &api.Order{Sql: `{"delete":"orders","deletes":[{"q":{"tenant":1},"limit":0}]}`},
		Rules:  &api.AuditRole{MongoForbidMultiWrite: true},
		Source: &api.DataSource{Kind: "mongodb"},
	}
	rep, err := e.execMongo(context.Background(), req)
	if err != nil || rep == nil || rep.Ok || !strings.Contains(rep.Error, "deletes[0].limit") {
		t.Fatalf("expected execution-time rule rejection, got %+v, %v", rep, err)
	}
	req.Rules.RuleLevels = map[string]string{"MongoForbidMultiWrite": "warn"}
	warns, err := recheckMongoRules(req, ruleset.MongoDefinitionFromAPI(req.Rules))
	if err != nil || len(warns) != 1 || !strings.Contains(warns[0], "deletes[0].limit") {
		t.Fatalf("expected execution-time warning, got %v, %v", warns, err)
	}
}

func TestExecMongoDBAApprovalDoesNotMaskNormalError(t *testing.T) {
	def := ruleset.Definition{
		SchemaVersion: ruleset.SchemaVersion,
		Dialect:       ruleset.DialectMongoDB,
		Dialects:      []string{string(ruleset.DialectMongoDB)},
		Scope:         ruleset.ScopeDataSource,
		Rules: []ruleset.Rule{
			{ID: "mongo.collection.drop", Enabled: true, Level: ruleset.LevelError},
			{
				ID:      "mongo.collection_prefix",
				Enabled: true,
				Level:   ruleset.LevelError,
				Params:  json.RawMessage(`{"prefix":"app_"}`),
			},
		},
	}
	req := &api.ExecRequest{
		Order: &api.Order{Sql: `{"drop":"legacy"}`},
		DBAApproval: &api.DBAApproval{
			Approved: true,
			Approver: "dba",
			Reason:   "approved after impact review",
		},
	}
	_, err := recheckMongoRules(req, def)
	if err == nil || !strings.Contains(err.Error(), "legacy") || strings.Contains(err.Error(), "DBA 特批未完成") {
		t.Fatalf("expected normal prefix rejection, got %v", err)
	}

	req.DBAApproval = nil
	decision, err := mongoe.New(nil).AuditDefinitionWithApproval(nil, mongoe.Input{
		Command: req.Order.Sql,
		Mode:    mongoe.ModeWrite,
	}, def, req.DBAApproval)
	if err != nil || decision.Error != nil {
		t.Fatalf("audit failed: decision=%+v error=%v", decision, err)
	}
	if !decision.RequiresDBAApproval {
		t.Fatalf("expected DBA approval requirement, got %+v", decision)
	}
}
