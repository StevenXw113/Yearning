package mongocheck

import "strconv"

const (
	RuleForbidUpsert     = "MongoForbidUpsert"
	RuleForbidMultiWrite = "MongoForbidMultiWrite"
)

func init() {
	Register(forbidUpsert{})
	Register(forbidMultiWrite{})
}

type forbidUpsert struct{}

func (forbidUpsert) Name() string { return RuleForbidUpsert }
func (forbidUpsert) Desc() string {
	return "禁止 update / findAndModify 的 upsert: true（未命中时会插入文档）"
}
func (forbidUpsert) Category() Category  { return CategorySecurity }
func (forbidUpsert) Modes() []Mode       { return []Mode{ModeWrite} }
func (forbidUpsert) DefaultLevel() Level { return LevelError }

func (r forbidUpsert) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.ForbidUpsert {
		return nil
	}
	location := ""
	switch cmd.Name {
	case "update":
		for i, clause := range writeClauses(cmd, "updates") {
			if upsert, _ := clause["upsert"].(bool); upsert {
				location = writeOptionPath(cmd, "updates", i, "upsert")
				break
			}
		}
	case "findandmodify":
		if upsert, _ := cmd.Doc["upsert"].(bool); upsert {
			location = "upsert"
		}
	}
	if location == "" {
		return nil
	}
	return []Finding{{Rule: r.Name(), Level: cfg.Level(r), Message: location + " 为 true，未匹配文档时会执行插入，请明确提交插入操作"}}
}

type forbidMultiWrite struct{}

func (forbidMultiWrite) Name() string { return RuleForbidMultiWrite }
func (forbidMultiWrite) Desc() string {
	return "禁止单个子句更新或删除多条文档（update 的 multi: true / delete 的 limit: 0）"
}
func (forbidMultiWrite) Category() Category  { return CategorySecurity }
func (forbidMultiWrite) Modes() []Mode       { return []Mode{ModeWrite} }
func (forbidMultiWrite) DefaultLevel() Level { return LevelError }

func (r forbidMultiWrite) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.ForbidMultiWrite {
		return nil
	}
	location := ""
	switch cmd.Name {
	case "update":
		for i, clause := range writeClauses(cmd, "updates") {
			if multi, _ := clause["multi"].(bool); multi {
				location = writeOptionPath(cmd, "updates", i, "multi")
				break
			}
		}
	case "delete":
		for i, clause := range writeClauses(cmd, "deletes") {
			if limit, ok := AsInt(clause["limit"]); ok && limit == 0 {
				location = writeOptionPath(cmd, "deletes", i, "limit")
				break
			}
		}
	}
	if location == "" {
		return nil
	}
	return []Finding{{Rule: r.Name(), Level: cfg.Level(r), Message: location + " 允许单个子句改删多条文档，请使用单文档操作"}}
}

func writeClauses(cmd *Command, field string) []map[string]interface{} {
	if clauses, ok := cmd.ClauseDocs(field); ok {
		return clauses
	}
	return []map[string]interface{}{cmd.Doc} // 与现有顶层 q 写法的审核兼容
}

func writeOptionPath(cmd *Command, field string, index int, option string) string {
	if _, ok := cmd.Doc[field]; ok {
		return field + "[" + strconv.Itoa(index) + "]." + option
	}
	return option
}
