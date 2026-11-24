package mongocheck

import "strings"

// DBAApprovalRequirement 是 MongoDB 工单的硬安全底线，不受普通规则开关影响。
// 返回 required=true 时，调用方必须在完成 DBA 特批前拒绝连接并执行命令。
func DBAApprovalRequirement(cmd *Command) (ruleID string, message string, required bool) {
	if cmd == nil {
		return "", "", false
	}
	switch strings.ToLower(strings.TrimSpace(cmd.Name)) {
	case "drop":
		return "mongo.collection.drop", "dropCollection 会删除集合及其索引；该类 dropCollection / renameCollection 操作必须完成 DBA 特批", true
	case "renamecollection":
		return "mongo.collection.rename", "renameCollection 会改变集合名并影响依赖方；该类 dropCollection / renameCollection 操作必须完成 DBA 特批", true
	}
	if _, ok := adminCommands[strings.ToLower(strings.TrimSpace(cmd.Name))]; ok {
		return "mongo.account.privilege", "账号与权限命令会改变访问控制面，必须完成 DBA 特批", true
	}
	return "", "", false
}

// dbaBaselineRuleSuppressed 判断普通规则是否与 DBA 硬底线重复。
func dbaBaselineRuleSuppressed(cmd *Command, ruleName string) bool {
	if cmd == nil {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(cmd.Name))
	if ruleName == RuleDropCollection && (name == "drop" || name == "renamecollection") {
		return true
	}
	_, isAdmin := adminCommands[name]
	return ruleName == RuleAdminCommand && isAdmin
}
