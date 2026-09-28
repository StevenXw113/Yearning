package server

import (
	"regexp"
	"strings"

	enginev1 "engine/gen/engine/v1"
	"engine/internal/mongocheck"

	storepb "engine/internal/bytebase/generated-go/store"
)

// 规则级别（规则集 RuleLevels[字段名] 的取值）：
//
//	error   命中即拦截（默认，未配置时按此处理）
//	warn    命中只提示，不影响提交
//	observe 命中只记录（信息级），不影响提交
//
// warn / observe 存在的意义是灰度：新规则先跑一段时间看命中率与误报，再收紧为 error。
const (
	ruleLevelError   = "error"
	ruleLevelWarn    = "warn"
	ruleLevelObserve = "observe"
)

// reviewPlan 按级别分组的审核规则：check.go 依次执行「错误 → 警告 → 观察」，
// 只有错误组命中才判「审核不通过」。
type reviewPlan struct {
	error   []*storepb.SQLReviewRule
	warning []*storepb.SQLReviewRule
	observe []*storepb.SQLReviewRule
}

// all 按执行顺序返回全部规则（不分级别），供覆盖度检查等使用。
func (p reviewPlan) all() []*storepb.SQLReviewRule {
	out := make([]*storepb.SQLReviewRule, 0, len(p.error)+len(p.warning)+len(p.observe))
	out = append(out, p.error...)
	out = append(out, p.warning...)
	return append(out, p.observe...)
}

// ruleLevel 取规则集里某条规则配置的级别；未配置或无法识别时按 error 处理（保持历史行为）。
func ruleLevel(r *enginev1.AuditRole, key string) string {
	switch strings.ToLower(strings.TrimSpace(r.GetRuleLevels()[key])) {
	case "warn", "warning":
		return ruleLevelWarn
	case "observe", "info":
		return ruleLevelObserve
	default:
		return ruleLevelError
	}
}

// reviewRules 将 Yearning 的 AuditRole 开关映射为 Bytebase MySQL 审核规则，并按配置级别分组。
//
// 语义约定（与前端规则说明保持一致）：
//   - “强制/检查 X”类开关：开启即启用对应规则；
//   - “允许 X”类开关：关闭即启用对应的禁止规则。
//
// 未列出的开关在 Bytebase 规则集中没有对应实现（多为执行期能力或与解析器无关），
// 由执行阶段处理，不参与静态审核。
//
// 规则类型即 Bytebase 的 storepb.SQLReviewRule_Type 枚举；升级 Bytebase 后
// 枚举增删会在编译期暴露，需同步调整本映射。
func reviewRules(r *enginev1.AuditRole) reviewPlan {
	var plan reviewPlan
	if r == nil {
		return plan
	}

	// put 填级别并归组。Bytebase 侧只有 ERROR/WARNING 两档，warn 与 observe 都报 WARNING，
	// 是否拦截由分组 + check.go 决定。
	put := func(rule *storepb.SQLReviewRule, key string) {
		level := ruleLevel(r, key)
		rule.Level = storepb.SQLReviewRule_ERROR
		if level != ruleLevelError {
			rule.Level = storepb.SQLReviewRule_WARNING
		}
		switch level {
		case ruleLevelWarn:
			plan.warning = append(plan.warning, rule)
		case ruleLevelObserve:
			plan.observe = append(plan.observe, rule)
		default:
			plan.error = append(plan.error, rule)
		}
	}
	base := func(t storepb.SQLReviewRule_Type) *storepb.SQLReviewRule {
		return &storepb.SQLReviewRule{Type: t, Engine: storepb.Engine_MYSQL}
	}
	add := func(t storepb.SQLReviewRule_Type, key string) {
		put(base(t), key)
	}
	number := func(t storepb.SQLReviewRule_Type, key string, n int32) {
		rule := base(t)
		rule.Payload = &storepb.SQLReviewRule_NumberPayload{
			NumberPayload: &storepb.SQLReviewRule_NumberRulePayload{Number: n},
		}
		put(rule, key)
	}
	list := func(t storepb.SQLReviewRule_Type, key string, items ...string) {
		rule := base(t)
		rule.Payload = &storepb.SQLReviewRule_StringArrayPayload{
			StringArrayPayload: &storepb.SQLReviewRule_StringArrayRulePayload{List: items},
		}
		put(rule, key)
	}
	naming := func(t storepb.SQLReviewRule_Type, key, format string, maxLength int32) {
		rule := base(t)
		rule.Payload = &storepb.SQLReviewRule_NamingPayload{
			NamingPayload: &storepb.SQLReviewRule_NamingRulePayload{Format: format, MaxLength: maxLength},
		}
		put(rule, key)
	}
	commentRequired := func(t storepb.SQLReviewRule_Type, key string) {
		rule := base(t)
		rule.Payload = &storepb.SQLReviewRule_CommentConventionPayload{
			CommentConventionPayload: &storepb.SQLReviewRule_CommentConventionRulePayload{Required: true},
		}
		put(rule, key)
	}

	// ---- DML ----
	if r.GetDmlWhere() {
		add(storepb.SQLReviewRule_STATEMENT_WHERE_REQUIRE_UPDATE_DELETE, "DMLWhere")
	}
	if r.GetDmlOrder() {
		add(storepb.SQLReviewRule_STATEMENT_DISALLOW_ORDER_BY, "DMLOrder")
	}
	if r.GetDmlWhereExprValueIsNull() {
		add(storepb.SQLReviewRule_STATEMENT_WHERE_NO_EQUAL_NULL, "DMLWhereExprValueIsNull")
	}
	if r.GetDmlInsertColumns() || r.GetDmlInsertMustExplicitly() {
		// 两个开关合成同一条规则，级别取先开启的那个
		key := "DMLInsertColumns"
		if !r.GetDmlInsertColumns() {
			key = "DMLInsertMustExplicitly"
		}
		add(storepb.SQLReviewRule_STATEMENT_INSERT_MUST_SPECIFY_COLUMN, key)
	}
	if n := r.GetDmlMaxInsertRows(); n > 0 {
		number(storepb.SQLReviewRule_STATEMENT_INSERT_ROW_LIMIT, "DMLMaxInsertRows", n)
	}
	// DMLAllowLimitSTMT 语义为“允许 update/insert 使用 LIMIT”，关闭即禁止。
	if !r.GetDmlAllowLimitStmt() {
		add(storepb.SQLReviewRule_STATEMENT_DISALLOW_LIMIT, "DMLAllowLimitSTMT")
	}

	// ---- DDL：强制/检查类 ----
	if r.GetDdlEnablePrimaryKey() {
		add(storepb.SQLReviewRule_TABLE_REQUIRE_PK, "DDLEnablePrimaryKey")
	}
	if r.GetDdlCheckTableComment() {
		commentRequired(storepb.SQLReviewRule_TABLE_COMMENT, "DDLCheckTableComment")
	}
	if r.GetDdlCheckColumnComment() {
		commentRequired(storepb.SQLReviewRule_COLUMN_COMMENT, "DDlCheckColumnComment")
	}
	// 注：COLUMN_NO_NULL / INDEX_TOTAL_NUMBER_LIMIT 依赖 advisor.Context.FinalMetadata
	// （库内元数据快照），引擎目前只做静态审核、不提供该快照，映射上去会在规则内
	// nil 解引用 panic，被 advisor 兜住后误报成「SQL 语法错误」。等接入元数据后再启用。

	if r.GetDdlCheckColumnDefault() {
		add(storepb.SQLReviewRule_COLUMN_REQUIRE_DEFAULT, "DDLCheckColumnDefault")
	}
	if r.GetDdlEnableAutoincrementInit() {
		number(storepb.SQLReviewRule_COLUMN_AUTO_INCREMENT_INITIAL_VALUE, "DDLEnableAutoincrementInit", 1)
	}
	if r.GetDdlEnableAutoincrementUnsigned() {
		add(storepb.SQLReviewRule_COLUMN_AUTO_INCREMENT_MUST_UNSIGNED, "DDLEnableAutoincrementUnsigned")
	}
	if r.GetDdlIndexNameSpec() {
		naming(storepb.SQLReviewRule_NAMING_INDEX_IDX, "DDLIndexNameSpec", "^idx_", 0)
	}
	if r.GetCheckIdentifier() {
		add(storepb.SQLReviewRule_NAMING_IDENTIFIER_NO_KEYWORD, "CheckIdentifier")
	}
	if n := r.GetDdlMaxKeyParts(); n > 0 {
		number(storepb.SQLReviewRule_INDEX_KEY_NUMBER_LIMIT, "DDLMaxKeyParts", int32(n))
	}
	// GetDdlMaxKey 原映射到 INDEX_TOTAL_NUMBER_LIMIT，该规则同样依赖 FinalMetadata，见上方说明。
	if n := r.GetDdlMaxCharLength(); n > 0 {
		number(storepb.SQLReviewRule_COLUMN_MAXIMUM_CHARACTER_LENGTH, "DDLMaxCharLength", int32(n))
	}

	// ---- 表名规范：前缀与长度合成为一条命名规则 ----
	if r.GetDdlTablePrefix() != "" || r.GetMaxTableNameLen() > 0 {
		format, key := "", "MaxTableNameLen"
		if r.GetDdlTablePrefix() != "" {
			format = "^" + regexp.QuoteMeta(r.GetDdlTablePrefix())
			key = "DDLTablePrefix"
		}
		naming(storepb.SQLReviewRule_NAMING_TABLE, key, format, r.GetMaxTableNameLen())
	}

	// ---- 白名单/必填列 ----
	if v := strings.TrimSpace(r.GetSupportCharset()); v != "" {
		list(storepb.SQLReviewRule_SYSTEM_CHARSET_ALLOWLIST, "SupportCharset", splitList(v)...)
	}
	if v := strings.TrimSpace(r.GetSupportCollation()); v != "" {
		list(storepb.SQLReviewRule_SYSTEM_COLLATION_ALLOWLIST, "SupportCollation", splitList(v)...)
	}
	if v := strings.TrimSpace(r.GetMustHaveColumns()); v != "" {
		list(storepb.SQLReviewRule_COLUMN_REQUIRED, "MustHaveColumns", splitList(v)...)
	}

	// ---- 禁止的列类型：特殊类型与 float/double 合成为一条规则 ----
	var disallowTypes []string
	key := "DDLCheckFloatDouble"
	if !r.GetAllowSpecialType() {
		disallowTypes = append(disallowTypes, "bit", "enum", "set")
		key = "AllowSpecialType"
	}
	if r.GetDdlCheckFloatDouble() {
		disallowTypes = append(disallowTypes, "float", "double")
	}
	if len(disallowTypes) > 0 {
		list(storepb.SQLReviewRule_COLUMN_TYPE_DISALLOW_LIST, key, disallowTypes...)
	}

	// ---- DDL：允许类开关取反 ----
	if !r.GetDdlEnableForeignKey() {
		add(storepb.SQLReviewRule_TABLE_NO_FOREIGN_KEY, "DDLEnableForeignKey")
	}
	if !r.GetAllowCreatePartition() {
		add(storepb.SQLReviewRule_TABLE_DISALLOW_PARTITION, "AllowCreatePartition")
	}
	if !r.GetAllowCreateView() {
		add(storepb.SQLReviewRule_SYSTEM_VIEW_DISALLOW_CREATE, "AllowCreateView")
	}
	if !r.GetDdlAllowColumnType() {
		add(storepb.SQLReviewRule_COLUMN_DISALLOW_CHANGE_TYPE, "DDLAllowColumnType")
	}
	if !r.GetDdlAllowChangeColumnPosition() {
		add(storepb.SQLReviewRule_COLUMN_DISALLOW_CHANGING_ORDER, "DDLAllowChangeColumnPosition")
	}
	if !r.GetDdlAllowPriNotInt() {
		add(storepb.SQLReviewRule_INDEX_PK_TYPE_LIMIT, "DDLAllowPRINotInt")
	}
	if !r.GetDdlAllowMultiAlter() {
		add(storepb.SQLReviewRule_STATEMENT_MERGE_ALTER_TABLE, "DDLAllowMultiAlter")
	}

	return plan
}

// mongoRulesFrom 构造 MongoDB 审核规则配置。
//
// Mongo 规则是自研的（上游 bytebase 无 Mongo advisor），但字段与 SQL 规则同构：
// 都存在同一份 AuditRole 里，级别也共用 rule_levels（key = 字段名）。
// mode 来自 CheckRequest.mode：空/write = 变更命令审核，query = 查询页的只读命令审核。
//
// 取值由 mongocheck 按名字对应关系反射完成——这里不再逐字段映射：
// 那种写法每条规则都要补一行，漏了不报错、只是规则永远不生效。
func mongoRulesFrom(r *enginev1.AuditRole, mode string) mongocheck.Config {
	return mongocheck.ConfigFromProto(r, mongocheck.Mode(mode))
}
