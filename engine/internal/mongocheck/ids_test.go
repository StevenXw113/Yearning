package mongocheck

import "testing"

func TestRuleIDsUseMongoNamespace(t *testing.T) {
	want := map[string][]string{
		RuleEmptyFilter:          {"mongo.empty_filter"},
		RuleDangerous:            {"mongo.dangerous_command"},
		RuleWhere:                {"mongo.where"},
		RuleDropCollection:       {"mongo.collection.drop", "mongo.collection.rename"},
		RuleSystemCollection:     {"mongo.system_collection"},
		RuleAdminCommand:         {"mongo.account.privilege"},
		RuleImmutableID:          {"mongo.immutable_id"},
		RuleForbidUpsert:         {"mongo.upsert"},
		RuleForbidMultiWrite:     {"mongo.multi_write"},
		RuleCappedConvert:        {"mongo.capped_convert"},
		RuleCollMod:              {"mongo.coll_mod"},
		RuleIndexKeyLimit:        {"mongo.index_key_limit"},
		RuleIndexNameSpec:        {"mongo.index_name_spec"},
		RuleCollectionPrefix:     {"mongo.collection_prefix"},
		RuleMaxCollectionNameLen: {"mongo.max_collection_name_len"},
		RuleRegexUnanchored:      {"mongo.regex_unanchored"},
		RuleNegationOperator:     {"mongo.negation_operator"},
		RuleOrClause:             {"mongo.or_clause"},
		RuleLargeInList:          {"mongo.large_in_list"},
		RuleQueryNoFilter:        {"mongo.query.no_filter"},
		RuleQueryForbidLookup:    {"mongo.query.no_lookup"},
		RuleQueryNoLimit:         {"mongo.query.no_limit"},
	}
	for legacy, ids := range want {
		got := RuleIDs(legacy)
		if len(got) != len(ids) {
			t.Fatalf("%s: RuleIDs() = %v, want %v", legacy, got, ids)
		}
		for i := range ids {
			if got[i] != ids[i] {
				t.Fatalf("%s: RuleIDs()[%d] = %q, want %q", legacy, i, got[i], ids[i])
			}
		}
	}
	if got := RuleIDs("MongoUnknown"); got != nil {
		t.Fatalf("unknown rule should return nil, got %v", got)
	}
}

func TestDBAApprovalRequirement(t *testing.T) {
	for _, tc := range []struct {
		name   string
		ruleID string
	}{
		{name: "drop", ruleID: "mongo.collection.drop"},
		{name: "renameCollection", ruleID: "mongo.collection.rename"},
		{name: "createUser", ruleID: "mongo.account.privilege"},
		{name: "grantRolesToUser", ruleID: "mongo.account.privilege"},
	} {
		ruleID, message, required := DBAApprovalRequirement(&Command{Name: tc.name})
		if !required || ruleID != tc.ruleID || message == "" {
			t.Fatalf("%s: DBAApprovalRequirement() = %q, %q, %v; want %q and required", tc.name, ruleID, message, required, tc.ruleID)
		}
	}
	ruleID, message, required := DBAApprovalRequirement(&Command{Name: "update"})
	if required || ruleID != "" || message != "" {
		t.Fatalf("update should not require DBA approval, got %q, %q, %v", ruleID, message, required)
	}
}

func TestDBAApprovalIsSafetyBaseline(t *testing.T) {
	_, findings, err := Check(`{"drop":"orders"}`, Config{Mode: ModeWrite})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || !findings[0].RequiresDBAApproval || findings[0].RuleID != "mongo.collection.drop" {
		t.Fatalf("drop must produce one DBA-approval finding, got %+v", findings)
	}
}
