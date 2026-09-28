package mongocheck

// RuleWhere 禁止 $where（把脚本推到服务端执行）
const RuleWhere = "MongoForbidWhere"

var whereRule = forbidWhere{}

func init() { Register(whereRule) }

type forbidWhere struct{}

func (forbidWhere) Name() string        { return RuleWhere }
func (forbidWhere) Desc() string        { return "禁止 $where（把脚本推到服务端执行）" }
func (forbidWhere) Category() Category  { return CategorySecurity }
func (forbidWhere) Modes() []Mode       { return []Mode{ModeWrite} }
func (forbidWhere) DefaultLevel() Level { return LevelError }

// 规则名沿用 MongoForbidWhere（开关名不能改：规则集里已经存了这个键），
// 判定覆盖整个「把脚本推到服务端执行」的家族——$function / $accumulator 是
// 4.4+ 起的等价能力，只拦 $where 等于留了个后门。
var serverScripts = []string{"$where", "$function", "$accumulator"}

func (r forbidWhere) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.ForbidWhere {
		return nil
	}
	for _, key := range serverScripts {
		if cmd.Has(key) {
			return []Finding{{
				Rule:    RuleWhere,
				Level:   cfg.Level(r),
				Message: "禁止使用 " + key + "（在服务端执行脚本）",
			}}
		}
	}
	return nil
}
