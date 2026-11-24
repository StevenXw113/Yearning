package mongocheck

// legacyRuleIDs 是旧 AuditRole 字段名到 v2 稳定规则 ID 的映射。
// v2 对外只暴露 mongo.* 命名空间 ID；旧字段名只作为兼容层存在。
var legacyRuleIDs = map[string][]string{
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

// RuleIDs 返回旧规则名对应的全部 v2 稳定 ID。
func RuleIDs(legacy string) []string {
	ids, ok := legacyRuleIDs[legacy]
	if !ok {
		return nil
	}
	return append([]string(nil), ids...)
}

// RuleID 返回旧规则名的主 v2 ID。
func RuleID(legacy string) string {
	ids := RuleIDs(legacy)
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

// KnownLegacyRules 返回已声明稳定 ID 的旧规则名，供规则清单与回归测试使用。
func KnownLegacyRules() []string {
	out := make([]string, 0, len(legacyRuleIDs))
	for name := range legacyRuleIDs {
		out = append(out, name)
	}
	return out
}
