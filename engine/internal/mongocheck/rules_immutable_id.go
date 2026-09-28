package mongocheck

import "strings"

// RuleImmutableID 禁止修改 _id
const RuleImmutableID = "MongoForbidImmutableId"

var immutableIDRule = forbidImmutableID{}

func init() { Register(immutableIDRule) }

type forbidImmutableID struct{}

func (forbidImmutableID) Name() string { return RuleImmutableID }
func (forbidImmutableID) Desc() string {
	return "禁止修改 _id（不可变字段：$set / $unset / $rename / 替换文档）"
}
func (forbidImmutableID) Category() Category  { return CategorySecurity }
func (forbidImmutableID) Modes() []Mode       { return []Mode{ModeWrite} }
func (forbidImmutableID) DefaultLevel() Level { return LevelError }

// 服务端本来也会拒绝改 _id，这条规则的价值在于**提前**：申请页就能看到问题，
// 而不是审批通过、执行时才失败（与 SQL 侧「禁止无 where 的 update」同理，
// 拦的是「不该这么写」，不只是「写了会报错」）。
//
// ponytail: 不解析聚合管道写法（u 为 [{...}]），$set 管道改 _id 同样会被服务端拒绝；
// 真要覆盖时在这里加一层数组遍历即可。
func (r forbidImmutableID) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.ForbidImmutableID {
		return nil
	}
	for _, doc := range updateDocs(cmd) {
		if where, bad := touchesID(doc); bad {
			return []Finding{{
				Rule:    RuleImmutableID,
				Level:   cfg.Level(r),
				Message: "禁止修改 _id（不可变字段，命中的是" + where + "）",
			}}
		}
	}
	return nil
}

// updateDocs 取出命令里的更新文档：update 的 updates[].u / 顶层 u，findAndModify 的 update
func updateDocs(cmd *Command) []map[string]interface{} {
	pick := func(v interface{}) map[string]interface{} {
		d, _ := v.(map[string]interface{})
		return d
	}
	var out []map[string]interface{}
	switch cmd.Name {
	case "update":
		if clauses, ok := cmd.ClauseDocs("updates"); ok {
			for _, c := range clauses {
				if u, ok := Lookup(c, "u"); ok {
					if d := pick(u); d != nil {
						out = append(out, d)
					}
				}
			}
			return out
		}
		if u, ok := cmd.Lookup("u"); ok {
			if d := pick(u); d != nil {
				out = append(out, d)
			}
		}
	case "findandmodify":
		if u, ok := cmd.Lookup("update"); ok {
			if d := pick(u); d != nil {
				out = append(out, d)
			}
		}
	}
	return out
}

// touchesID 判断一份更新文档是否动了 _id，返回命中位置的说明。
// 字段名用精确匹配：_id 与 _ID 在 Mongo 里是两个字段，忽略大小写会误报。
func touchesID(doc map[string]interface{}) (string, bool) {
	operatorDoc := false
	for k := range doc {
		if strings.HasPrefix(k, "$") {
			operatorDoc = true
			break
		}
	}
	if !operatorDoc {
		// 整体替换文档：带 _id 就会与现有值比对，对不上直接报错
		if _, ok := doc["_id"]; ok {
			return "替换文档里的 _id", true
		}
		return "", false
	}
	for _, op := range []string{"$set", "$unset", "$rename"} {
		v, ok := doc[op]
		if !ok {
			continue
		}
		fields, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if _, has := fields["_id"]; has {
			// $rename 的键是源字段：改走 _id 也是改不可变字段
			return op + " 的目标字段 _id", true
		}
		if op == "$rename" {
			for _, to := range fields {
				if s, ok := to.(string); ok && s == "_id" {
					return "$rename 的目标字段 _id", true
				}
			}
		}
	}
	return "", false
}
