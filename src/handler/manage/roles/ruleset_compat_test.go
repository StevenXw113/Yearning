package roles

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"Yearning-go/src/engine"
	"Yearning-go/src/model"

	"engine/ruleset"
)

func TestNormalizeMongoRuleSetFromDefinition(t *testing.T) {
	def := ruleset.Definition{
		SchemaVersion: ruleset.SchemaVersion,
		Dialect:       ruleset.DialectMongoDB,
		Scope:         ruleset.ScopeDataSource,
		Settings:      ruleset.Settings{MaxAffectRows: 100},
		Rules: []ruleset.Rule{
			{ID: "mongo.multi_write", Enabled: true, Level: ruleset.LevelError},
			{ID: "mongo.large_in_list", Enabled: true, Level: ruleset.LevelWarn, Params: json.RawMessage(`{"max_items":50}`)},
		},
	}
	data, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	rule := &model.CoreRules{Definition: data}
	if err := normalizeMongoRuleSet(rule); err != nil {
		t.Fatalf("normalizeMongoRuleSet() error = %v", err)
	}
	var old engine.AuditRole
	if err := rule.AuditRole.UnmarshalToJSON(&old); err != nil {
		t.Fatalf("兼容层 JSON 非法: %v", err)
	}
	if !old.MongoForbidMultiWrite || old.MongoLargeInList != 50 || old.MongoMaxAffectRows != 100 {
		t.Fatalf("v2 -> 兼容层转换错误: %#v", old)
	}
	if rule.Dialect != string(ruleset.DialectMongoDB) || rule.Scope != string(ruleset.ScopeDataSource) {
		t.Fatalf("规则集元数据错误: dialect=%q scope=%q", rule.Dialect, rule.Scope)
	}
}

func TestNormalizeMongoRuleSetFromLegacy(t *testing.T) {
	rule := &model.CoreRules{AuditRole: model.JSON(`{
		"MongoForbidMultiWrite": true,
		"MongoForbidAdminCommand": true,
		"MongoLargeInList": 50,
		"MongoMaxAffectRows": 100
	}`)}
	if err := normalizeMongoRuleSet(rule); err != nil {
		t.Fatalf("normalizeMongoRuleSet() error = %v", err)
	}
	def, err := ruleset.Decode(rule.Definition)
	if err != nil {
		t.Fatalf("规则集 Definition 非法: %v", err)
	}
	for _, id := range []string{"mongo.multi_write", "mongo.collection.drop", "mongo.collection.rename", "mongo.account.privilege", "mongo.large_in_list"} {
		rule, ok := def.Rule(id)
		if !ok || !rule.Enabled {
			t.Fatalf("旧规则转换后缺少 %s: %#v", id, def.Rules)
		}
	}
}

func TestNormalizeMongoRuleSetRejectsMixedDialects(t *testing.T) {
	rule := &model.CoreRules{AuditRole: model.JSON(`{
		"MongoForbidMultiWrite": true,
		"RedisDeleteKey": true,
		"RuleLevel": {"DDLForbidTruncate":"error"}
	}`)}
	if err := normalizeMongoRuleSet(rule); err == nil || !strings.Contains(err.Error(), "非 MongoDB") {
		t.Fatalf("expected mixed-dialect rejection, got %v", err)
	}
}

func TestRestoreMongoRuleSetFromHistory(t *testing.T) {
	current := &model.CoreRules{Dialect: "mongodb", Definition: []byte(`{"schema_version":2}`)}
	restored, err := restoreRuleSetFromHistory(current, model.JSON(`{"MongoForbidMultiWrite":true}`))
	if err != nil {
		t.Fatalf("restoreRuleSetFromHistory() error = %v", err)
	}
	if restored.Dialect != "mongodb" || restored.Scope != "data_source" || len(restored.Definition) == 0 {
		t.Fatalf("MongoDB rollback did not rebuild v2 fields: %#v", restored)
	}
	def, err := ruleset.Decode(restored.Definition)
	if err != nil {
		t.Fatal(err)
	}
	if rule, ok := def.Rule("mongo.multi_write"); !ok || !rule.Enabled {
		t.Fatalf("MongoDB rollback lost stable rule ID: %#v", def.Rules)
	}
}

func TestRuleSetHistorySnapshotRoundTrip(t *testing.T) {
	rule := &model.CoreRules{
		ID:        42,
		Desc:      "imported",
		AuditRole: model.JSON(`{"MongoForbidMultiWrite":true}`),
		Definition: model.JSON(`{
			"schema_version": 2,
			"dialect": "mongodb",
			"scope": "data_source",
			"settings": {"max_affect_rows": 100},
			"rules": [
				{"id":"mongo.multi_write","enabled":true,"level":"error"},
				{"id":"mongo.future_rule","enabled":false,"level":"warn","reason":"future"}
			]
		}`),
		Dialect: string(ruleset.DialectMongoDB),
		Scope:   string(ruleset.ScopeDataSource),
	}
	data, err := json.Marshal(ruleSetHistorySnapshotFromRule(rule))
	if err != nil {
		t.Fatalf("marshal snapshot failed: %v", err)
	}
	decoded, err := decodeRuleSetHistorySnapshot(data)
	if err != nil {
		t.Fatalf("decodeRuleSetHistorySnapshot() error = %v", err)
	}
	restored, err := restoreRuleSetFromHistory(nil, decoded)
	if err != nil {
		t.Fatalf("restoreRuleSetFromHistory() error = %v", err)
	}
	if restored.Desc != rule.Desc || restored.Dialect != rule.Dialect || restored.Scope != rule.Scope {
		t.Fatalf("snapshot metadata lost: %#v", restored)
	}
	compactJSON := func(data model.JSON) string {
		var out bytes.Buffer
		if err := json.Compact(&out, data); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	if compactJSON(restored.AuditRole) != compactJSON(rule.AuditRole) || compactJSON(restored.Definition) != compactJSON(rule.Definition) {
		t.Fatalf("snapshot body changed:\naudit=%s\ndefinition=%s", restored.AuditRole, restored.Definition)
	}
	def, err := ruleset.Decode(restored.Definition)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := def.Rule("mongo.future_rule"); !ok {
		t.Fatalf("future rule lost on rollback: %#v", def.Rules)
	}
}

func TestRuleSetHistorySnapshotRejectsCreationBeforeState(t *testing.T) {
	data, err := json.Marshal(ruleSetHistorySnapshotFromRule(nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeRuleSetHistorySnapshot(data); err == nil || !strings.Contains(err.Error(), "无法回滚") {
		t.Fatalf("decodeRuleSetHistorySnapshot() error = %v, want empty snapshot rollback rejection", err)
	}
}

func TestNormalizeMongoRuleSetEnforcesDBAApprovalRules(t *testing.T) {
	def := ruleset.Definition{
		SchemaVersion: ruleset.SchemaVersion,
		Dialect:       ruleset.DialectMongoDB,
		Dialects:      []string{string(ruleset.DialectMongoDB)},
		Scope:         ruleset.ScopeDataSource,
		Rules: []ruleset.Rule{
			{ID: "mongo.multi_write", Enabled: true, Level: ruleset.LevelError},
			{ID: "mongo.collection.drop", Enabled: false, Level: ruleset.LevelError},
		},
	}
	data, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	rule := &model.CoreRules{Definition: data}
	if err := normalizeMongoRuleSet(rule); err != nil {
		t.Fatalf("normalizeMongoRuleSet() error = %v", err)
	}
	out, err := ruleset.Decode(rule.Definition)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"mongo.collection.drop", "mongo.collection.rename", "mongo.account.privilege"} {
		if !out.Enabled(id) {
			t.Fatalf("DBA approval rule %s was not enabled: %#v", id, out.Rules)
		}
	}
}
