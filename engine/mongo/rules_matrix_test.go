package mongo

import (
	"encoding/json"
	"testing"

	"engine/api"
	"engine/ruleset"
)

// All commands are audited with a nil client, including destructive examples.
func TestMongoRuleManifestMatrix(t *testing.T) {
	type example struct {
		id, bad, good, params string
		badFilter, goodFilter string
	}
	examples := []example{
		{id: "mongo.empty_filter", bad: `{"update":"users","updates":[{"q":{},"u":{"$set":{"x":1}}}]}`, good: `{"update":"users","updates":[{"q":{"_id":1},"u":{"$set":{"x":1}}}]}`},
		{id: "mongo.dangerous_command", bad: `{"dropDatabase":1}`, good: `{"create":"users"}`},
		{id: "mongo.where", bad: `{"update":"users","updates":[{"q":{"$where":"true"},"u":{"$set":{"x":1}}}]}`, good: `{"update":"users","updates":[{"q":{"_id":1},"u":{"$set":{"x":1}}}]}`},
		{id: "mongo.collection.drop", bad: `{"drop":"users"}`, good: `{"create":"users"}`},
		{id: "mongo.collection.rename", bad: `{"renameCollection":"test.users","to":"test.archive"}`, good: `{"create":"users"}`},
		{id: "mongo.system_collection", bad: `{"insert":"system.users","documents":[{"x":1}]}`, good: `{"insert":"users","documents":[{"x":1}]}`},
		{id: "mongo.account.privilege", bad: `{"createUser":"reader","pwd":"test-only","roles":[]}`, good: `{"create":"users"}`},
		{id: "mongo.immutable_id", bad: `{"update":"users","updates":[{"q":{"_id":1},"u":{"$set":{"_id":2}}}]}`, good: `{"update":"users","updates":[{"q":{"_id":1},"u":{"$set":{"x":2}}}]}`},
		{id: "mongo.upsert", bad: `{"update":"users","updates":[{"q":{"_id":1},"u":{"$set":{"x":1}},"upsert":true}]}`, good: `{"update":"users","updates":[{"q":{"_id":1},"u":{"$set":{"x":1}},"upsert":false}]}`},
		{id: "mongo.multi_write", bad: `{"delete":"users","deletes":[{"q":{"x":1},"limit":0}]}`, good: `{"delete":"users","deletes":[{"q":{"x":1},"limit":1}]}`},
		{id: "mongo.capped_convert", bad: `{"convertToCapped":"users","size":4096}`, good: `{"create":"users"}`},
		{id: "mongo.coll_mod", bad: `{"collMod":"users","validator":{"x":{"$type":"string"}}}`, good: `{"create":"users"}`},
		{id: "mongo.index_key_limit", params: `{"max_keys":2}`, bad: `{"createIndexes":"users","indexes":[{"key":{"a":1,"b":1,"c":1},"name":"idx_abc"}]}`, good: `{"createIndexes":"users","indexes":[{"key":{"a":1,"b":1},"name":"idx_ab"}]}`},
		{id: "mongo.index_name_spec", params: `{"pattern":"^idx_"}`, bad: `{"createIndexes":"users","indexes":[{"key":{"a":1},"name":"a_1"}]}`, good: `{"createIndexes":"users","indexes":[{"key":{"a":1},"name":"idx_a"}]}`},
		{id: "mongo.collection_prefix", params: `{"prefix":"app_"}`, bad: `{"create":"users"}`, good: `{"create":"app_users"}`},
		{id: "mongo.max_collection_name_len", params: `{"max_length":8}`, bad: `{"create":"123456789"}`, good: `{"create":"12345678"}`},
		{id: "mongo.regex_unanchored", badFilter: `{"name":{"$regex":"abc"}}`, goodFilter: `{"name":{"$regex":"^abc"}}`},
		{id: "mongo.negation_operator", badFilter: `{"x":{"$ne":1}}`, goodFilter: `{"x":{"$eq":1}}`},
		{id: "mongo.or_clause", badFilter: `{"$or":[{"x":1},{"x":2}]}`, goodFilter: `{"$and":[{"x":1},{"y":2}]}`},
		{id: "mongo.large_in_list", params: `{"max_items":2}`, badFilter: `{"x":{"$in":[1,2,3]}}`, goodFilter: `{"x":{"$in":[1,2]}}`},
		{id: "mongo.query.no_filter", bad: `{"find":"users","filter":{}}`, good: `{"find":"users","filter":{"_id":1}}`},
		{id: "mongo.query.no_lookup", bad: `{"aggregate":"users","pipeline":[{"$lookup":{"from":"orders","localField":"_id","foreignField":"user","as":"orders"}}],"cursor":{}}`, good: `{"aggregate":"users","pipeline":[{"$match":{"_id":1}}],"cursor":{}}`},
		{id: "mongo.query.no_limit", bad: `{"find":"users","filter":{"x":1},"sort":{"x":1}}`, good: `{"find":"users","filter":{"x":1},"sort":{"x":1},"limit":1}`},
	}
	byID := make(map[string]example, len(examples))
	for _, ex := range examples {
		if _, exists := byID[ex.id]; exists {
			t.Fatalf("duplicate test example %s", ex.id)
		}
		byID[ex.id] = ex
	}
	manifest := ruleset.MongoManifest()
	if len(byID) != len(manifest.Rules) {
		t.Fatalf("examples=%d manifest=%d", len(byID), len(manifest.Rules))
	}
	for _, meta := range manifest.Rules {
		ex, exists := byID[meta.ID]
		if !exists {
			t.Fatalf("missing rule example %s", meta.ID)
		}
		t.Run(meta.ID, func(t *testing.T) {
			for _, mode := range meta.Modes {
				t.Run(mode, func(t *testing.T) {
					bad, good := ex.bad, ex.good
					if ex.badFilter != "" {
						wrap := func(filter string) string {
							if Mode(mode) == ModeQuery {
								return `{"find":"users","filter":` + filter + `}`
							}
							return `{"update":"users","updates":[{"q":` + filter + `,"u":{"$set":{"x":1}}}]}`
						}
						bad, good = wrap(ex.badFilter), wrap(ex.goodFilter)
					}
					definition := func(enabled bool, level ruleset.Level) ruleset.Definition {
						return ruleset.Definition{
							SchemaVersion: ruleset.SchemaVersion, Dialect: ruleset.DialectMongoDB, Scope: ruleset.ScopeDataSource,
							Rules: []ruleset.Rule{{ID: meta.ID, Enabled: enabled, Level: level, Params: json.RawMessage(ex.params)}},
						}
					}
					check := func(t *testing.T, command string, def ruleset.Definition, hit bool, wantLevel Level, legacy bool) {
						t.Helper()
						var decision Decision
						var err error
						input := Input{Command: command, Database: "test", Mode: Mode(mode)}
						if legacy {
							decision, err = New(nil).Audit(nil, input, ruleset.MongoDefinitionToAPI(def))
						} else {
							decision, err = New(nil).AuditDefinition(nil, input, def)
						}
						if err != nil || decision.Error != nil {
							t.Fatalf("audit: decision=%+v error=%v", decision, err)
						}
						if hit {
							if len(decision.Findings) != 1 {
								t.Fatalf("expected exactly one finding: %+v", decision)
							}
							finding := decision.Findings[0]
							if finding.RuleID != meta.ID || finding.Rule != meta.LegacyID || finding.Level != wantLevel || finding.Message == "" || finding.RequiresDBAApproval != meta.RequiresDBAApproval {
								t.Fatalf("unexpected rule finding: %+v", finding)
							}
						} else if len(decision.Findings) != 0 {
							t.Fatalf("safe or disabled rule produced findings: %+v", decision)
						}
						if decision.Allowed != (!hit || wantLevel != LevelError) {
							t.Fatalf("unexpected allow/block decision: %+v", decision)
						}
					}
					for _, level := range []ruleset.Level{ruleset.LevelError, ruleset.LevelWarn, ruleset.LevelObserve} {
						t.Run(string(level), func(t *testing.T) {
							wantLevel := Level(level)
							if meta.RequiresDBAApproval {
								wantLevel = LevelError
							}
							for _, legacy := range []bool{false, true} {
								check(t, bad, definition(true, level), true, wantLevel, legacy)
							}
							// Exercise the persisted/exported JSON contract before auditing again.
							raw, err := json.Marshal(definition(true, level))
							if err != nil {
								t.Fatal(err)
							}
							decoded, err := ruleset.Decode(raw)
							if err != nil {
								t.Fatal(err)
							}
							check(t, bad, decoded, true, wantLevel, false)
						})
					}
					t.Run("safe", func(t *testing.T) { check(t, good, definition(true, ruleset.LevelError), false, LevelError, false) })
					t.Run("disabled", func(t *testing.T) {
						for _, legacy := range []bool{false, true} {
							check(t, bad, definition(false, ruleset.LevelError), meta.RequiresDBAApproval, LevelError, legacy)
						}
					})
					t.Run("legacy_default_level", func(t *testing.T) {
						rules := ruleset.MongoDefinitionToAPI(definition(true, meta.DefaultLevel))
						rules.RuleLevels = nil
						decision, err := New(nil).Audit(nil, Input{Command: bad, Mode: Mode(mode)}, rules)
						if err != nil || decision.Error != nil || len(decision.Findings) != 1 || decision.Findings[0].Level != Level(meta.DefaultLevel) {
							t.Fatalf("default level does not match manifest: decision=%+v error=%v", decision, err)
						}
					})
				})
			}
		})
	}
}

func TestAuditApprovedBaselineKeepsEvidence(t *testing.T) {
	approval := &api.DBAApproval{Approved: true, Approver: "dba", Reason: "reviewed"}
	decision, err := New(nil).AuditDefinitionWithApproval(nil, Input{Command: `{"drop":"users"}`, Mode: ModeWrite}, ruleset.MongoDefinitionFromAPI(nil), approval)
	if err != nil || decision.Error != nil || !decision.Allowed || !decision.RequiresDBAApproval || len(decision.Findings) != 1 || !decision.Findings[0].RequiresDBAApproval {
		t.Fatalf("completed approval must allow execution and retain evidence: decision=%+v error=%v", decision, err)
	}
}
