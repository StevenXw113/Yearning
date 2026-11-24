package ruleset

import (
	"encoding/json"
	"fmt"
	"strings"

	"engine/api"
)

// MongoDefinitionFromAPI converts the legacy wire AuditRole into a v2 MongoDB data-source rule set.
// The result is intended to be persisted as CoreRules.Definition; old field names are not public IDs.
func MongoDefinitionFromAPI(old *api.AuditRole) Definition {
	out := Definition{
		SchemaVersion: SchemaVersion,
		Dialect:       DialectMongoDB,
		Dialects:      []string{string(DialectMongoDB)},
		Scope:         ScopeDataSource,
	}
	if old == nil {
		return out
	}
	out.Settings.MaxAffectRows = int(old.MongoMaxAffectRows)

	manifest := MongoManifest()
	for _, meta := range manifest.Rules {
		enabled, params, ok := legacyMongoValue(old, meta.LegacyID)
		if !ok || !enabled {
			continue
		}
		rule := Rule{ID: meta.ID, Enabled: true, Level: legacyMongoLevel(old.RuleLevels, meta), Params: params}
		out.Rules = append(out.Rules, rule)
	}
	return out
}

// MongoDefinitionToAPI converts a v2 MongoDB data-source rule set back to the legacy wire contract.
// Unknown rule IDs are ignored only after ValidateMongoDefinition has accepted the definition; callers
// must validate imported definitions before using this conversion.
func MongoDefinitionToAPI(def Definition) *api.AuditRole {
	out := &api.AuditRole{RuleLevels: map[string]string{}}
	out.MongoMaxAffectRows = int32(def.Settings.MaxAffectRows)

	manifest := MongoManifest()
	byID := make(map[string]ManifestRule, len(manifest.Rules))
	for _, meta := range manifest.Rules {
		byID[meta.ID] = meta
	}
	for _, rule := range def.Rules {
		meta, ok := byID[rule.ID]
		if !ok || !rule.Enabled {
			continue
		}
		applyMongoRule(out, meta, rule)
		if rule.Level != "" {
			out.RuleLevels[meta.LegacyID] = string(rule.Level)
		}
	}
	return out
}

// ValidateMongoDefinition validates a MongoDB data-source rule set before import or execution.
// It fails closed on unsupported dialects, non-data-source scopes, unknown IDs and malformed parameters.
func ValidateMongoDefinition(def Definition) error {
	if def.Dialect != DialectMongoDB {
		return fmt.Errorf("mongodb rule set dialect must be %q, got %q", DialectMongoDB, def.Dialect)
	}
	if def.Scope != ScopeDataSource {
		return fmt.Errorf("mongodb rule set scope must be %q, got %q", ScopeDataSource, def.Scope)
	}
	if len(def.Dialects) > 1 {
		return fmt.Errorf("mongodb data-source rule set cannot mix dialects: %v", def.Dialects)
	}
	for _, dialect := range def.Dialects {
		if dialect != string(DialectMongoDB) {
			return fmt.Errorf("invalid dialect %q in mongodb data-source rule set", dialect)
		}
	}
	if err := def.Validate(MongoKnownRules()); err != nil {
		return err
	}
	manifest := MongoManifest()
	byID := make(map[string]ManifestRule, len(manifest.Rules))
	for _, meta := range manifest.Rules {
		byID[meta.ID] = meta
	}
	for _, rule := range def.Rules {
		meta, ok := byID[rule.ID]
		if !ok {
			return fmt.Errorf("unknown rule %q", rule.ID)
		}
		if err := validateMongoParams(meta, rule.Params); err != nil {
			return err
		}
	}
	return validateMongoDBAApprovalRules(def)
}

// EnforceMongoDBAApprovalRules enables the MongoDB rules that require DBA approval.
// dropCollection, renameCollection, and account/privilege commands are hard security
// boundaries and cannot be disabled by a rule-set payload.
func EnforceMongoDBAApprovalRules(def *Definition) {
	if def == nil {
		return
	}
	required := mongoDBAApprovalRuleIDs()
	for _, id := range required {
		found := false
		for i := range def.Rules {
			if def.Rules[i].ID != id {
				continue
			}
			def.Rules[i].Enabled = true
			found = true
			break
		}
		if found {
			continue
		}
		var level Level
		for _, meta := range MongoManifest().Rules {
			if meta.ID == id {
				level = meta.DefaultLevel
				break
			}
		}
		def.Rules = append(def.Rules, Rule{ID: id, Enabled: true, Level: level})
	}
}

func mongoDBAApprovalRuleIDs() []string {
	var out []string
	for _, meta := range MongoManifest().Rules {
		if meta.RequiresDBAApproval {
			out = append(out, meta.ID)
		}
	}
	return out
}

func validateMongoDBAApprovalRules(def Definition) error {
	for _, id := range mongoDBAApprovalRuleIDs() {
		if !def.Enabled(id) {
			return fmt.Errorf("rule %q must be enabled because it requires DBA approval", id)
		}
	}
	return nil
}

func legacyMongoValue(old *api.AuditRole, legacyID string) (bool, json.RawMessage, bool) {
	switch legacyID {
	case "MongoForbidEmptyFilter":
		return old.MongoForbidEmptyFilter, nil, true
	case "MongoForbidDangerous":
		return old.MongoForbidDangerous, nil, true
	case "MongoForbidWhere":
		return old.MongoForbidWhere, nil, true
	case "MongoForbidDropCollection":
		return old.MongoForbidDropCollection, nil, true
	case "MongoForbidSystemCollection":
		return old.MongoForbidSystemCollection, nil, true
	case "MongoForbidAdminCommand":
		return old.MongoForbidAdminCommand, nil, true
	case "MongoForbidImmutableId":
		return old.MongoForbidImmutableId, nil, true
	case "MongoForbidUpsert":
		return old.MongoForbidUpsert, nil, true
	case "MongoForbidMultiWrite":
		return old.MongoForbidMultiWrite, nil, true
	case "MongoForbidCappedConvert":
		return old.MongoForbidCappedConvert, nil, true
	case "MongoForbidCollMod":
		return old.MongoForbidCollMod, nil, true
	case "MongoIndexKeyLimit":
		if old.MongoIndexKeyLimit == 0 {
			return false, nil, true
		}
		return true, mongoParams("max_keys", old.MongoIndexKeyLimit), true
	case "MongoIndexNameSpec":
		if old.MongoIndexNameSpec == "" {
			return false, nil, true
		}
		return true, mongoParams("pattern", old.MongoIndexNameSpec), true
	case "MongoCollectionPrefix":
		if old.MongoCollectionPrefix == "" {
			return false, nil, true
		}
		return true, mongoParams("prefix", old.MongoCollectionPrefix), true
	case "MongoMaxCollectionNameLen":
		if old.MongoMaxCollectionNameLen == 0 {
			return false, nil, true
		}
		return true, mongoParams("max_length", old.MongoMaxCollectionNameLen), true
	case "MongoRegexUnanchored":
		return old.MongoRegexUnanchored, nil, true
	case "MongoNegationOperator":
		return old.MongoNegationOperator, nil, true
	case "MongoOrClause":
		return old.MongoOrClause, nil, true
	case "MongoLargeInList":
		if old.MongoLargeInList == 0 {
			return false, nil, true
		}
		return true, mongoParams("max_items", old.MongoLargeInList), true
	case "MongoQueryForbidNoFilter":
		return old.MongoQueryForbidNoFilter, nil, true
	case "MongoQueryForbidLookup":
		return old.MongoQueryForbidLookup, nil, true
	case "MongoQueryForbidNoLimit":
		return old.MongoQueryForbidNoLimit, nil, true
	default:
		return false, nil, false
	}
}

func legacyMongoLevel(levels map[string]string, meta ManifestRule) Level {
	if value, ok := levels[meta.LegacyID]; ok {
		level := Level(strings.ToLower(strings.TrimSpace(value)))
		if level != "" {
			return level
		}
	}
	return meta.DefaultLevel
}

func applyMongoRule(out *api.AuditRole, meta ManifestRule, rule Rule) {
	switch meta.ID {
	case "mongo.empty_filter":
		out.MongoForbidEmptyFilter = true
	case "mongo.dangerous_command":
		out.MongoForbidDangerous = true
	case "mongo.where":
		out.MongoForbidWhere = true
	case "mongo.collection.drop", "mongo.collection.rename":
		out.MongoForbidDropCollection = true
	case "mongo.system_collection":
		out.MongoForbidSystemCollection = true
	case "mongo.account.privilege":
		out.MongoForbidAdminCommand = true
	case "mongo.immutable_id":
		out.MongoForbidImmutableId = true
	case "mongo.upsert":
		out.MongoForbidUpsert = true
	case "mongo.multi_write":
		out.MongoForbidMultiWrite = true
	case "mongo.capped_convert":
		out.MongoForbidCappedConvert = true
	case "mongo.coll_mod":
		out.MongoForbidCollMod = true
	case "mongo.index_key_limit":
		if value, ok := mongoIntParam(rule.Params, meta.ParamKey); ok {
			out.MongoIndexKeyLimit = int32(value)
		}
	case "mongo.index_name_spec":
		if value, ok := mongoStringParam(rule.Params, meta.ParamKey); ok {
			out.MongoIndexNameSpec = value
		}
	case "mongo.collection_prefix":
		if value, ok := mongoStringParam(rule.Params, meta.ParamKey); ok {
			out.MongoCollectionPrefix = value
		}
	case "mongo.max_collection_name_len":
		if value, ok := mongoIntParam(rule.Params, meta.ParamKey); ok {
			out.MongoMaxCollectionNameLen = int32(value)
		}
	case "mongo.regex_unanchored":
		out.MongoRegexUnanchored = true
	case "mongo.negation_operator":
		out.MongoNegationOperator = true
	case "mongo.or_clause":
		out.MongoOrClause = true
	case "mongo.large_in_list":
		if value, ok := mongoIntParam(rule.Params, meta.ParamKey); ok {
			out.MongoLargeInList = int32(value)
		}
	case "mongo.query.no_filter":
		out.MongoQueryForbidNoFilter = true
	case "mongo.query.no_lookup":
		out.MongoQueryForbidLookup = true
	case "mongo.query.no_limit":
		out.MongoQueryForbidNoLimit = true
	}
}

func validateMongoParams(meta ManifestRule, raw json.RawMessage) error {
	if meta.ParamType == ParamTypeNone {
		if len(raw) != 0 && strings.TrimSpace(string(raw)) != "null" {
			return fmt.Errorf("rule %q does not accept parameters", meta.ID)
		}
		return nil
	}
	var params map[string]json.RawMessage
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("rule %q parameters must be a JSON object", meta.ID)
	}
	if len(params) != 1 {
		return fmt.Errorf("rule %q must contain exactly one parameter", meta.ID)
	}
	value, ok := params[meta.ParamKey]
	if !ok {
		return fmt.Errorf("rule %q parameter key must be %q", meta.ID, meta.ParamKey)
	}
	switch meta.ParamType {
	case ParamTypeInt:
		var number int64
		if err := json.Unmarshal(value, &number); err != nil {
			return fmt.Errorf("rule %q parameter %s must be a JSON integer", meta.ID, meta.ParamKey)
		}
	case ParamTypeString:
		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			return fmt.Errorf("rule %q parameter %s must be a JSON string", meta.ID, meta.ParamKey)
		}
	default:
		return fmt.Errorf("rule %q has unsupported parameter type %q", meta.ID, meta.ParamType)
	}
	return nil
}

func mongoIntParam(raw json.RawMessage, key string) (int, bool) {
	var params map[string]json.RawMessage
	if err := json.Unmarshal(raw, &params); err != nil {
		return 0, false
	}
	value, ok := params[key]
	if !ok {
		return 0, false
	}
	var number json.Number
	if err := json.Unmarshal(value, &number); err != nil {
		return 0, false
	}
	out, err := number.Int64()
	if err != nil || out < 0 || out > int64(int(^uint(0)>>1)) {
		return 0, false
	}
	return int(out), true
}

func mongoStringParam(raw json.RawMessage, key string) (string, bool) {
	var params map[string]json.RawMessage
	if err := json.Unmarshal(raw, &params); err != nil {
		return "", false
	}
	value, ok := params[key]
	if !ok {
		return "", false
	}
	var out string
	if err := json.Unmarshal(value, &out); err != nil {
		return "", false
	}
	return out, true
}

func mongoParams(key string, value interface{}) json.RawMessage {
	data, err := json.Marshal(map[string]interface{}{key: value})
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return data
}
