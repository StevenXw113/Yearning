package ruleset

import (
	"encoding/json"
	"testing"

	"engine/api"
)

func TestMongoManifestUsesStableNamespaceIDs(t *testing.T) {
	m := MongoManifest()
	if m.SchemaVersion != SchemaVersion || m.Dialect != DialectMongoDB || m.Scope != ScopeDataSource {
		t.Fatalf("unexpected manifest header: %+v", m)
	}
	seen := map[string]ManifestRule{}
	for _, r := range m.Rules {
		if r.ID == "" || r.LegacyID == "" || len(r.Modes) == 0 {
			t.Fatalf("incomplete manifest rule: %+v", r)
		}
		if !isMongoNamespaceID(r.ID) {
			t.Fatalf("rule id must use mongo.* namespace: %q", r.ID)
		}
		if r.ID == r.LegacyID {
			t.Fatalf("legacy id must not be v2 id: %q", r.ID)
		}
		if _, dup := seen[r.ID]; dup {
			t.Fatalf("duplicate manifest rule id: %q", r.ID)
		}
		seen[r.ID] = r
	}
	for _, id := range []string{
		"mongo.collection.drop", "mongo.collection.rename", "mongo.account.privilege",
	} {
		if !seen[id].RequiresDBAApproval {
			t.Fatalf("%s must require DBA approval", id)
		}
	}
	if got := seen["mongo.large_in_list"].ParamType; got != ParamTypeInt {
		t.Fatalf("mongo.large_in_list param type = %q, want int", got)
	}
	if got := seen["mongo.index_name_spec"].ParamType; got != ParamTypeString {
		t.Fatalf("mongo.index_name_spec param type = %q, want string", got)
	}
}

func TestMongoDefinitionRoundTrip(t *testing.T) {
	old := &api.AuditRole{
		MongoForbidEmptyFilter:    true,
		MongoForbidDropCollection: true,
		MongoForbidAdminCommand:   true,
		MongoForbidMultiWrite:     true,
		MongoIndexKeyLimit:        12,
		MongoIndexNameSpec:        `^idx_`,
		MongoLargeInList:          100,
		MongoQueryForbidNoFilter:  true,
		MongoMaxAffectRows:        1000,
		MongoRegexUnanchored:      true,
		RuleLevels: map[string]string{
			"MongoForbidMultiWrite": "warn",
			"MongoRegexUnanchored":  "observe",
		},
	}
	def := MongoDefinitionFromAPI(old)
	if err := ValidateMongoDefinition(def); err != nil {
		t.Fatal(err)
	}
	if def.SchemaVersion != SchemaVersion || def.Dialect != DialectMongoDB || def.Scope != ScopeDataSource {
		t.Fatalf("unexpected definition header: %+v", def)
	}
	if def.Settings.MaxAffectRows != 1000 {
		t.Fatalf("max_affect_rows = %d, want 1000", def.Settings.MaxAffectRows)
	}
	if !def.Enabled("mongo.multi_write") || def.LevelOf("mongo.multi_write", LevelError) != LevelWarn {
		t.Fatalf("multi_write was not converted: %+v", def)
	}
	if !def.Enabled("mongo.collection.drop") || !def.Enabled("mongo.collection.rename") {
		t.Fatalf("dropCollection should expand to drop and rename rules: %+v", def)
	}
	if !def.Enabled("mongo.query.no_filter") {
		t.Fatalf("query.no_filter was not converted: %+v", def)
	}

	got := MongoDefinitionToAPI(def)
	if !got.MongoForbidEmptyFilter || !got.MongoForbidDropCollection || !got.MongoForbidAdminCommand || !got.MongoForbidMultiWrite {
		t.Fatalf("boolean switches were not restored: %+v", got)
	}
	if got.MongoIndexKeyLimit != 12 || got.MongoIndexNameSpec != `^idx_` || got.MongoLargeInList != 100 || got.MongoMaxAffectRows != 1000 {
		t.Fatalf("parameters were not restored: %+v", got)
	}
	if got.RuleLevels["MongoForbidMultiWrite"] != "warn" || got.RuleLevels["MongoRegexUnanchored"] != "observe" {
		t.Fatalf("levels were not restored: %+v", got.RuleLevels)
	}
}

func TestValidateMongoDefinitionRejectsBadImports(t *testing.T) {
	base := MongoDefinitionFromAPI(&api.AuditRole{MongoForbidMultiWrite: true})
	base.Dialect = DialectMySQL
	if err := ValidateMongoDefinition(base); err == nil || !contains(err.Error(), "dialect") {
		t.Fatalf("wrong dialect must be rejected, got %v", err)
	}

	base = MongoDefinitionFromAPI(&api.AuditRole{MongoForbidMultiWrite: true})
	base.Scope = ScopeGlobal
	if err := ValidateMongoDefinition(base); err == nil || !contains(err.Error(), "scope") {
		t.Fatalf("data-source scope must require mongodb dialect, got %v", err)
	}

	base = MongoDefinitionFromAPI(&api.AuditRole{MongoForbidMultiWrite: true})
	base.Rules = append(base.Rules, Rule{ID: "mongo.unknown", Enabled: true, Level: LevelWarn})
	if err := ValidateMongoDefinition(base); err == nil || !contains(err.Error(), "unknown rule") {
		t.Fatalf("unknown rule must be rejected, got %v", err)
	}

	base = MongoDefinitionFromAPI(&api.AuditRole{MongoLargeInList: 10})
	base.Rules[0].Params = json.RawMessage(`{"max_items":"20"}`)
	if err := ValidateMongoDefinition(base); err == nil || !contains(err.Error(), "integer") {
		t.Fatalf("integer parameter type must be enforced, got %v", err)
	}
}

func isMongoNamespaceID(id string) bool {
	return len(id) > 6 && id[:6] == "mongo."
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsAt(s, sub))
}

func containsAt(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
