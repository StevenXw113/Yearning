package ruleset

import (
	"strings"
	"testing"
)

func TestMongoDBAApprovalRulesAreEnforced(t *testing.T) {
	def := Definition{
		SchemaVersion: SchemaVersion,
		Dialect:       DialectMongoDB,
		Dialects:      []string{string(DialectMongoDB)},
		Scope:         ScopeDataSource,
		Rules: []Rule{
			{ID: "mongo.multi_write", Enabled: true, Level: LevelError},
		},
	}
	if err := ValidateMongoDefinition(def); err == nil || !strings.Contains(err.Error(), "requires DBA approval") {
		t.Fatalf("ValidateMongoDefinition() error = %v, want DBA approval rejection", err)
	}

	EnforceMongoDBAApprovalRules(&def)
	if err := ValidateMongoDefinition(def); err != nil {
		t.Fatalf("ValidateMongoDefinition() error = %v", err)
	}
	for _, id := range []string{"mongo.collection.drop", "mongo.collection.rename", "mongo.account.privilege"} {
		if !def.Enabled(id) {
			t.Fatalf("DBA approval rule %s is not enabled: %#v", id, def.Rules)
		}
	}
}
