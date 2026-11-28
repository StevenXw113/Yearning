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

func TestConnectMongoValidatesTLSBeforeNetwork(t *testing.T) {
	_, err := connectMongo(&api.DataSource{Ip: "localhost", MongoTLS: true, Ca: "invalid-ca"})
	if err == nil || !strings.Contains(err.Error(), "CA") {
		t.Fatalf("expected CA validation error, got %v", err)
	}
	_, err = connectMongo(&api.DataSource{Ip: "localhost", MongoAuthMechanism: "unsupported"})
	if err == nil || !strings.Contains(err.Error(), "认证机制") {
		t.Fatalf("expected authentication validation error, got %v", err)
	}
}

// These checks never connect: approved destructive commands are audited only.
func TestExecMongoDBAApprovalMatrix(t *testing.T) {
	commands := map[string]string{
		"drop":    `{"drop":"users"}`,
		"rename":  `{"renameCollection":"test.users","to":"test.archived"}`,
		"account": `{"createUser":"reader","pwd":"test-only","roles":[]}`,
	}
	approvals := []struct {
		name    string
		value   *api.DBAApproval
		allowed bool
	}{
		{name: "missing"},
		{name: "rejected", value: &api.DBAApproval{Approver: "dba", Reason: "reviewed"}},
		{name: "no_approver", value: &api.DBAApproval{Approved: true, Reason: "reviewed"}},
		{name: "no_reason", value: &api.DBAApproval{Approved: true, Approver: "dba"}},
		{name: "blank_approver", value: &api.DBAApproval{Approved: true, Approver: " ", Reason: "reviewed"}},
		{name: "blank_reason", value: &api.DBAApproval{Approved: true, Approver: "dba", Reason: " "}},
		{name: "approved", value: &api.DBAApproval{Approved: true, Approver: "dba", Reason: "reviewed"}, allowed: true},
	}
	for name, command := range commands {
		for _, approval := range approvals {
			t.Run(name+"/"+approval.name, func(t *testing.T) {
				req := &api.ExecRequest{Order: &api.Order{Sql: command, DataBase: "test"}, DBAApproval: approval.value}
				warns, err := recheckMongoRules(req, ruleset.MongoDefinitionFromAPI(nil))
				if approval.allowed {
					if err != nil || len(warns) != 0 {
						t.Fatalf("approved baseline must pass without unresolved warnings: warnings=%v error=%v", warns, err)
					}
				} else if err == nil || !strings.Contains(err.Error(), "DBA 特批未完成") {
					t.Fatalf("incomplete approval must block: %v", err)
				}
			})
		}
	}
}

func TestExecMongoApprovedBaselinePreservesNormalLevels(t *testing.T) {
	for _, level := range []string{"error", "warn", "observe"} {
		t.Run(level, func(t *testing.T) {
			rules := &api.AuditRole{MongoCollectionPrefix: "app_", RuleLevels: map[string]string{"MongoCollectionPrefix": level}}
			req := &api.ExecRequest{
				Order:       &api.Order{Sql: `{"drop":"legacy"}`},
				DBAApproval: &api.DBAApproval{Approved: true, Approver: "dba", Reason: "reviewed"},
			}
			warns, err := recheckMongoRules(req, ruleset.MongoDefinitionFromAPI(rules))
			if level == "error" {
				if err == nil || !strings.Contains(err.Error(), "legacy") || strings.Contains(err.Error(), "DBA 特批未完成") {
					t.Fatalf("normal rule must still block: %v", err)
				}
			} else if err != nil || len(warns) != 1 || !strings.Contains(warns[0], "legacy") || strings.Contains(warns[0], "DBA") {
				t.Fatalf("normal nonblocking finding must survive approval: warnings=%v error=%v", warns, err)
			}
		})
	}
}
