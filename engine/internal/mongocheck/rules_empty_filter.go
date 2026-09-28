package mongocheck

// RuleEmptyFilter 禁止无 filter 的 update / delete / findAndModify
const RuleEmptyFilter = "MongoForbidEmptyFilter"

var emptyFilterRule = forbidEmptyFilter{}

func init() { Register(emptyFilterRule) }

type forbidEmptyFilter struct{}

func (forbidEmptyFilter) Name() string { return RuleEmptyFilter }
func (forbidEmptyFilter) Desc() string {
	return "禁止无 filter 的 update / delete / findAndModify（否则作用于整个集合）"
}
func (forbidEmptyFilter) Category() Category  { return CategorySecurity }
func (forbidEmptyFilter) Modes() []Mode       { return []Mode{ModeWrite} }
func (forbidEmptyFilter) DefaultLevel() Level { return LevelError }

func (r forbidEmptyFilter) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.ForbidEmptyFilter {
		return nil
	}
	switch cmd.Name {
	case "update", "delete", "findandmodify":
		if cmd.EmptyFilter() {
			return []Finding{{
				Rule:    RuleEmptyFilter,
				Level:   cfg.Level(r),
				Message: "update / delete / findAndModify 必须带非空 filter，否则会作用于整个集合",
			}}
		}
	}
	return nil
}
