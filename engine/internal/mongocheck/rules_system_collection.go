package mongocheck

import "strings"

// RuleSystemCollection 禁止操作系统集合（system.*）
const RuleSystemCollection = "MongoForbidSystemCollection"

var systemCollectionRule = forbidSystemCollection{}

func init() { Register(systemCollectionRule) }

type forbidSystemCollection struct{}

func (forbidSystemCollection) Name() string { return RuleSystemCollection }
func (forbidSystemCollection) Desc() string {
	return "禁止对 system.* 集合（system.users / system.profile / system.js / system.views）做变更"
}
func (forbidSystemCollection) Category() Category  { return CategorySecurity }
func (forbidSystemCollection) Modes() []Mode       { return []Mode{ModeWrite} }
func (forbidSystemCollection) DefaultLevel() Level { return LevelError }

// system.* 是数据库自己的账本：system.users 存账号、system.profile 存慢日志、
// system.js 存服务端脚本。改这些集合要么直接改权限数据、要么抹掉审计线索，
// 都不该走数据变更工单。集合名大小写敏感，所以用前缀精确匹配（systemic 不误伤）。
const systemCollectionPrefix = "system."

func (r forbidSystemCollection) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.ForbidSystemCollection || cmd.Coll == "" {
		return nil
	}
	if !strings.HasPrefix(cmd.Coll, systemCollectionPrefix) {
		return nil
	}
	return []Finding{{
		Rule:    RuleSystemCollection,
		Level:   cfg.Level(r),
		Message: "禁止变更系统集合 " + cmd.Coll + "（数据库自身的账号/审计/脚本数据）",
	}}
}
