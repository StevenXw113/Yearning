package mongocheck

// RuleDropCollection 禁止 drop / renameCollection 集合
const RuleDropCollection = "MongoForbidDropCollection"

var dropCollectionRule = forbidDropCollection{}

func init() { Register(dropCollectionRule) }

type forbidDropCollection struct{}

func (forbidDropCollection) Name() string        { return RuleDropCollection }
func (forbidDropCollection) Desc() string        { return "禁止 drop / renameCollection 集合" }
func (forbidDropCollection) Category() Category  { return CategorySecurity }
func (forbidDropCollection) Modes() []Mode       { return []Mode{ModeWrite} }
func (forbidDropCollection) DefaultLevel() Level { return LevelError }

func (r forbidDropCollection) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.ForbidDropCollection {
		return nil
	}
	switch cmd.Name {
	case "drop", "renamecollection":
		return []Finding{{
			Rule:    RuleDropCollection,
			Level:   cfg.Level(r),
			Message: "规则集禁止 drop / renameCollection 集合",
		}}
	}
	return nil
}
