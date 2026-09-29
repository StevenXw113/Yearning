package mongocheck

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// 变更类型，取值与工单 Type 对齐（主程序侧 vars.DDL=0 / vars.DML=1）
const (
	KindQuery = -1 // 只读命令，不走工单
	KindDDL   = 0  // 改集合/索引结构
	KindDML   = 1  // 改数据
)

// dmlCommands 改数据的命令
//
// 不含 bulkWrite：那是驱动层方法，不是服务端命令，runCommand 发不出去——
// 留在表里只会让这类工单「能过审但执行不了」。
var dmlCommands = map[string]struct{}{
	"insert": {}, "update": {}, "delete": {}, "findandmodify": {},
}

// ddlCommands 改集合/索引结构的命令
var ddlCommands = map[string]struct{}{
	"create": {}, "drop": {}, "createindexes": {}, "dropindexes": {},
	"collmod": {}, "renamecollection": {}, "converttocapped": {},
}

// adminCommands 账号与权限管理命令。
//
// 归到结构变更而不是查询：它们不是只读命令，落进「不是变更命令」那条分支
// 既拦不住（查询页/申请页的判定依据都是分类）也说不清理由。
// 是否允许执行由 MongoForbidAdminCommand 决定（规则集里没配就不拦，与其他规则一致）。
var adminCommands = map[string]struct{}{
	"createuser": {}, "updateuser": {}, "dropuser": {}, "dropallusersfromdatabase": {},
	"createrole": {}, "updaterole": {}, "droprole": {}, "dropallrolesfromdatabase": {},
	"grantrolestouser": {}, "revokerolesfromuser": {},
	"grantprivilegestorole": {}, "revokeprivilegesfromrole": {},
	"invalidateusercache": {},
}

// dangerousCommands 危险命令：命令名 → 拦截说明（由 MongoForbidDangerous 规则使用）。
//
// 放在这里而不是规则文件里，是因为分类也要用它：这些命令名不在 dml/ddl 表里，
// 若不特殊处理会被当成「只读命令」，于是申请页会回一句「不是变更命令」——
// 既拦不住也说不清理由。Kind 由分类表决定，与「该规则是否开启」无关。
var dangerousCommands = map[string]string{
	"dropdatabase": "禁止 dropDatabase（删除整个数据库）",
	"eval":         "禁止 eval（服务端执行脚本）",
	"mapreduce":    "禁止 mapReduce（可在服务端执行脚本）",
}

// Command 一条解析后的 MongoDB 命令
type Command struct {
	// Name 命令名：JSON 里的第一个键（Mongo 按第一个字段识别命令），已转小写
	Name string
	// Doc 整个命令文档
	Doc map[string]interface{}
	// Coll 目标集合：命令名的取值（若是字符串），如 {"update":"users"} → users
	Coll string
	// Kind 变更类型（KindQuery / KindDDL / KindDML）
	Kind int
}

// Parse 解析一条命令。命令名是 JSON 里的第一个键——map 遍历无序，
// 所以单独用 token 流再读一次。
func Parse(cmdJSON string) (*Command, error) {
	body := strings.TrimSpace(cmdJSON)
	if body == "" {
		return nil, errors.New("MongoDB 命令不能为空")
	}
	dec := json.NewDecoder(strings.NewReader(body))
	dec.UseNumber()
	var doc map[string]interface{}
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("命令不是合法的 JSON：%v", err)
	}

	name := ""
	toks := json.NewDecoder(strings.NewReader(body))
	if t, err := toks.Token(); err == nil {
		if d, ok := t.(json.Delim); ok && d == '{' {
			if k, err := toks.Token(); err == nil {
				if s, ok := k.(string); ok {
					name = strings.ToLower(s)
				}
			}
		}
	}
	if name == "" {
		return nil, errors.New(`无法识别命令名，示例：{"update":"users","updates":[...]}`)
	}

	cmd := &Command{Name: name, Doc: doc, Kind: classify(name)}
	if coll, ok := Lookup(doc, name); ok {
		if s, ok := coll.(string); ok {
			cmd.Coll = s
		}
	}
	return cmd, nil
}

// classify 按命令名分类：先看 dml/ddl 表，管理命令与危险命令都按结构变更处理
// （见 adminCommands / dangerousCommands 的注释）
func classify(name string) int {
	if _, ok := dmlCommands[name]; ok {
		return KindDML
	}
	if _, ok := ddlCommands[name]; ok {
		return KindDDL
	}
	if _, ok := adminCommands[name]; ok {
		return KindDDL
	}
	if _, ok := dangerousCommands[name]; ok {
		return KindDDL
	}
	return KindQuery
}

// Lookup 从文档里取值，键名大小写不敏感（Mongo 命令名与字段名都不区分大小写）
func Lookup(doc map[string]interface{}, key string) (interface{}, bool) {
	if v, ok := doc[key]; ok {
		return v, true
	}
	for k, v := range doc {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return nil, false
}

// Lookup 成员方法版本，等价于 Lookup(cmd.Doc, key)
func (c *Command) Lookup(key string) (interface{}, bool) {
	if c == nil {
		return nil, false
	}
	return Lookup(c.Doc, key)
}

// Has 递归查找某个字段：用于 $where / $function 这类可出现在任意层级的操作符
func (c *Command) Has(key string) bool {
	if c == nil {
		return false
	}
	return hasKey(c.Doc, key)
}

// ClauseDocs 取改删命令的子句数组（updates[] / deletes[]）。
// 第二个返回值为 false 表示命令里没有该字段（旧式写法：条件直接在顶层 q）。
func (c *Command) ClauseDocs(field string) ([]map[string]interface{}, bool) {
	v, ok := c.Lookup(field)
	if !ok {
		return nil, false
	}
	arr, ok := v.([]interface{})
	if !ok {
		// 字段存在但不是数组（写法错误）：返回空列表让调用方按「条件为空」处理
		return nil, true
	}
	out := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		clause, ok := item.(map[string]interface{})
		if !ok {
			out = append(out, nil)
			continue
		}
		out = append(out, clause)
	}
	return out, true
}

// EmptyFilter 判定改删命令的条件是否为空。
//
// 这是 Mongo 最容易造成事故的一类操作（不带条件就等于作用于整个集合），
// 空 filter 规则、硬保底校验、抓前镜像三处都需要同一套语义，所以放在解析层。
func (c *Command) EmptyFilter() bool {
	field := ""
	switch c.Name {
	case "update":
		field = "updates"
	case "delete":
		field = "deletes"
	}
	if field != "" {
		if clauses, ok := c.ClauseDocs(field); ok {
			if len(clauses) == 0 {
				return true
			}
			for _, clause := range clauses {
				if clause == nil {
					return true
				}
				q, ok := Lookup(clause, "q")
				if !ok || IsEmptyDoc(q) {
					return true // 任一子句条件为空即算空
				}
			}
			return false
		}
	}
	// 旧式写法：条件在顶层 q；findAndModify 用 query
	key := "q"
	if c.Name == "findandmodify" {
		key = "query"
	}
	q, ok := c.Lookup(key)
	return !ok || IsEmptyDoc(q)
}

// IsEmptyDoc 判断条件是否为「空」：{} / nil / 空文档
func IsEmptyDoc(v interface{}) bool {
	switch m := v.(type) {
	case nil:
		return true
	case map[string]interface{}:
		return len(m) == 0
	}
	return false
}

// AsInt 把解析出来的 JSON 数字转成 int（UseNumber 解出的是 json.Number）
func AsInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i), true
		}
		if f, err := n.Float64(); err == nil {
			return int(f), true
		}
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	}
	return 0, false
}

// KeyCount 文档的键数（用于索引键数上限这类判定）
func KeyCount(v interface{}) int {
	if m, ok := v.(map[string]interface{}); ok {
		return len(m)
	}
	return 0
}

// Indexes 取 createIndexes 的索引定义列表
func (c *Command) Indexes() []map[string]interface{} {
	v, ok := c.Lookup("indexes")
	if !ok {
		return nil
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		if m, ok := item.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out
}

// Walk 递归遍历文档里的所有键值（含数组元素），fn 返回 true 时停止并返回 true。
// 只判键名用 Has 就够；要按「值」判定的规则（$regex 的值、$in 的数组长度）用这个。
func (c *Command) Walk(fn func(key string, val interface{}) bool) bool {
	return walk(c.Doc, fn)
}

func walk(v interface{}, fn func(string, interface{}) bool) bool {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, val := range t {
			if fn(k, val) || walk(val, fn) {
				return true
			}
		}
	case []interface{}:
		for _, item := range t {
			if walk(item, fn) {
				return true
			}
		}
	}
	return false
}

// hasKey 递归查找某个字段
func hasKey(v interface{}, key string) bool {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, val := range t {
			if strings.EqualFold(k, key) || hasKey(val, key) {
				return true
			}
		}
	case []interface{}:
		for _, item := range t {
			if hasKey(item, key) {
				return true
			}
		}
	}
	return false
}
