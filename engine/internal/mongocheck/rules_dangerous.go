package mongocheck

// RuleDangerous 禁止 dropDatabase / eval / mapReduce 等危险命令
const RuleDangerous = "MongoForbidDangerous"

var dangerousRule = forbidDangerous{}

func init() { Register(dangerousRule) }

type forbidDangerous struct{}

func (forbidDangerous) Name() string        { return RuleDangerous }
func (forbidDangerous) Desc() string        { return "禁止 dropDatabase / eval / mapReduce 等危险命令" }
func (forbidDangerous) Category() Category  { return CategorySecurity }
func (forbidDangerous) Modes() []Mode       { return []Mode{ModeWrite} }
func (forbidDangerous) DefaultLevel() Level { return LevelError }

// 命令清单见 command.go 的 dangerousCommands：分类也要用它，
// 否则这些命令会被当成只读命令
func (r forbidDangerous) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.ForbidDangerous {
		return nil
	}
	msg, bad := dangerousCommands[cmd.Name]
	if !bad {
		return nil
	}
	return []Finding{{Rule: RuleDangerous, Level: cfg.Level(r), Message: msg}}
}
