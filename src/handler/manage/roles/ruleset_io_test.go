package roles

import (
	"encoding/json"
	"strings"
	"testing"

	"Yearning-go/src/lib/factory"
	"Yearning-go/src/model"

	"engine/ruleset"
)

func TestRuleSetExportFromRule(t *testing.T) {
	mongoDefinition := ruleset.Definition{
		SchemaVersion: ruleset.SchemaVersion,
		Dialect:       ruleset.DialectMongoDB,
		Dialects:      []string{string(ruleset.DialectMongoDB)},
		Scope:         ruleset.ScopeDataSource,
		Rules: []ruleset.Rule{
			{ID: "mongo.multi_write", Enabled: true, Level: ruleset.LevelError},
		},
	}
	definition, err := json.Marshal(mongoDefinition)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name        string
		rule        *model.CoreRules
		wantDialect string
		wantDefn    bool
		wantCompat  []string
	}{
		{"mongo v2", &model.CoreRules{Definition: definition, Dialect: string(ruleset.DialectMongoDB), Scope: string(ruleset.ScopeDataSource)}, string(ruleset.DialectMongoDB), true, nil},
		{"legacy mongo", &model.CoreRules{AuditRole: model.JSON(`{"MongoForbidMultiWrite":true}`), Scope: string(ruleset.ScopeDataSource)}, string(ruleset.DialectMongoDB), true, nil},
		{"legacy sql", &model.CoreRules{AuditRole: model.JSON(`{"DDLForbidTruncate":true}`), Scope: string(ruleset.ScopeDataSource)}, "sql", false, []string{string(ruleset.DialectMySQL), string(ruleset.DialectPostgres)}},
		{"redis", &model.CoreRules{AuditRole: model.JSON(`{"RedisForbidDebug":true}`), Dialect: string(ruleset.DialectRedis), Scope: string(ruleset.ScopeDataSource)}, string(ruleset.DialectRedis), false, nil},
		{"doris", &model.CoreRules{AuditRole: model.JSON(`{"DorisRequireDistribution":true}`), Dialect: string(ruleset.DialectDoris), Scope: string(ruleset.ScopeDataSource)}, string(ruleset.DialectDoris), false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := ruleSetExportFromRule(tc.rule)
			if err != nil {
				t.Fatalf("ruleSetExportFromRule() error = %v", err)
			}
			if payload.Dialect != tc.wantDialect {
				t.Fatalf("dialect = %q, want %q", payload.Dialect, tc.wantDialect)
			}
			if (payload.Definition != nil) != tc.wantDefn {
				t.Fatalf("definition present = %v, want %v", payload.Definition != nil, tc.wantDefn)
			}
			if strings.Join(payload.CompatibleDialects, ",") != strings.Join(tc.wantCompat, ",") {
				t.Fatalf("compatible dialects = %v, want %v", payload.CompatibleDialects, tc.wantCompat)
			}
		})
	}
}

func TestRuleSetExportRejectsNonMongoDefinition(t *testing.T) {
	rule := &model.CoreRules{
		Dialect:    string(ruleset.DialectMySQL),
		Definition: model.JSON(`{"schema_version":2}`),
		Scope:      string(ruleset.ScopeDataSource),
	}
	if _, err := ruleSetExportFromRule(rule); err == nil || !strings.Contains(err.Error(), "仅 MongoDB 支持 v2 规则集") {
		t.Fatalf("ruleSetExportFromRule() error = %v, want non-Mongo definition rejection", err)
	}
}

func TestValidateImportDialectAndCompatibility(t *testing.T) {
	for _, dialect := range []string{string(ruleset.DialectMySQL), string(ruleset.DialectPostgres), string(ruleset.DialectMongoDB), string(ruleset.DialectRedis), string(ruleset.DialectDoris)} {
		if err := validateImportDialect(dialect); err != nil {
			t.Fatalf("validateImportDialect(%q) error = %v", dialect, err)
		}
	}
	if err := validateImportDialect("sql"); err == nil {
		t.Fatal("validateImportDialect(sql) should require a concrete target dialect")
	}

	compat := []struct {
		target   string
		inferred factory.LegacyRuleDialect
		want     bool
	}{
		{string(ruleset.DialectMySQL), factory.LegacyRuleDialectSQL, true},
		{string(ruleset.DialectPostgres), factory.LegacyRuleDialectSQL, true},
		{string(ruleset.DialectMySQL), factory.LegacyRuleDialectMongo, false},
		{string(ruleset.DialectMongoDB), factory.LegacyRuleDialectMongo, true},
		{string(ruleset.DialectRedis), factory.LegacyRuleDialectRedis, true},
		{string(ruleset.DialectDoris), factory.LegacyRuleDialectDoris, true},
		{string(ruleset.DialectRedis), factory.LegacyRuleDialectDoris, false},
	}
	for _, tc := range compat {
		if got := importDialectCompatible(tc.target, tc.inferred); got != tc.want {
			t.Fatalf("importDialectCompatible(%q, %q) = %v, want %v", tc.target, tc.inferred, got, tc.want)
		}
	}
}
