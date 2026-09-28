package mongocheck

// RuleAdminCommand 禁止账号与权限管理命令
const RuleAdminCommand = "MongoForbidAdminCommand"

var adminCommandRule = forbidAdminCommand{}

func init() { Register(adminCommandRule) }

type forbidAdminCommand struct{}

func (forbidAdminCommand) Name() string { return RuleAdminCommand }
func (forbidAdminCommand) Desc() string {
	return "禁止 createUser / dropUser / grantRolesToUser / createRole 等账号与权限管理命令"
}
func (forbidAdminCommand) Category() Category  { return CategorySecurity }
func (forbidAdminCommand) Modes() []Mode       { return []Mode{ModeWrite} }
func (forbidAdminCommand) DefaultLevel() Level { return LevelError }

// 命令清单见 command.go 的 adminCommands：分类也要用它，否则这些命令会被当成只读命令
func (r forbidAdminCommand) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.ForbidAdminCommand {
		return nil
	}
	if _, ok := adminCommands[cmd.Name]; !ok {
		return nil
	}
	return []Finding{{
		Rule:    RuleAdminCommand,
		Level:   cfg.Level(r),
		Message: "禁止通过工单执行账号/权限管理命令，请由 DBA 在数据库侧操作",
	}}
}
