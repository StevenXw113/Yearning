// Package mongodb 提供 MongoDB 变更命令的识别、安全校验与执行（含回滚前镜像快照）。
//
// 与 SQL 侧的根本差别：SQL 审核建立在语法树（引擎内的 Bytebase 解析器）之上，
// 而 Mongo 命令就是一段 JSON，没有可审核的 AST。这里退一步，用
// 「命令名 + 若干关键字段」做约束——够挡住最常见也最危险的操作（全集合改删、
// 服务端脚本、删库），且不需要引入一个 Mongo 语法解析器。
package mongodb

import (
	"errors"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
)

// 变更类型，取值与工单的 Type 对齐（vars.DDL=0 / vars.DML=1）
const (
	KindQuery = -1 // 只读命令，不走工单
	KindDDL   = 0  // 改集合/索引结构
	KindDML   = 1  // 改数据
)

// dmlCommands 改数据的命令
//
// 不含 bulkWrite：它是驱动层方法、不是服务端命令，runCommand 发不出去。
var dmlCommands = map[string]struct{}{
	"insert": {}, "update": {}, "delete": {}, "findandmodify": {},
}

// adminCommands 账号与权限管理命令：归结构变更，否则会被当成只读命令
// （查询页的只读白名单另有一道门，见 query.go）。
var adminCommands = map[string]struct{}{
	"createuser": {}, "updateuser": {}, "dropuser": {}, "dropallusersfromdatabase": {},
	"createrole": {}, "updaterole": {}, "droprole": {}, "dropallrolesfromdatabase": {},
	"grantrolestouser": {}, "revokerolesfromuser": {},
	"grantprivilegestorole": {}, "revokeprivilegesfromrole": {},
	"invalidateusercache": {},
}

// ddlCommands 改集合/索引结构的命令
var ddlCommands = map[string]struct{}{
	"create": {}, "drop": {}, "createindexes": {}, "dropindexes": {},
	"collmod": {}, "renamecollection": {}, "converttocapped": {},
}

// forbiddenCommands 一律拒绝的命令
var forbiddenCommands = map[string]string{
	"dropdatabase": "禁止删除数据库",
	"eval":         "禁止执行服务端脚本",
	"mapreduce":    "禁止使用 mapReduce（可在服务端执行脚本）",
}

// CommandName 取命令名：Mongo 命令名大小写不敏感，且第一个字段就是命令名
func CommandName(cmd bson.D) string {
	if len(cmd) == 0 {
		return ""
	}
	return strings.ToLower(cmd[0].Key)
}

// CommandCollection 取命令作用的集合。
// Mongo 变更命令的形态是 {<命令名>: "<集合名>", ...}——集合名在第一个字段的值上。
func CommandCollection(cmd bson.D) string {
	if len(cmd) == 0 {
		return ""
	}
	if name, ok := cmd[0].Value.(string); ok {
		return name
	}
	return ""
}

// Classify 判断命令属于哪一类；不认识的按只读处理（查询路径会直接执行）。
func Classify(cmd bson.D) int {
	name := CommandName(cmd)
	if _, ok := dmlCommands[name]; ok {
		return KindDML
	}
	if _, ok := ddlCommands[name]; ok {
		return KindDDL
	}
	if _, ok := adminCommands[name]; ok {
		return KindDDL
	}
	return KindQuery
}

// Validate 提交 / 执行前的硬保底校验：空 filter、删库与服务端脚本、$where 一律拦下。
//
// 这一层**不读规则集**，也不受配置影响——它是执行底线（与抓前镜像的条数上限同类）。
// 可配置的审核（开关 + error/warn/observe 级别）统一由引擎完成，见
// engine/internal/mongocheck；两处的判定语义保持一致。
func Validate(cmd bson.D) (int, error) {
	name := CommandName(cmd)
	kind := Classify(cmd)

	if msg, bad := forbiddenCommands[name]; bad {
		return KindDDL, errors.New(msg)
	}
	if kind == KindQuery {
		return kind, errors.New("不是变更命令（insert / update / delete / createIndexes 等），无需提交工单")
	}
	switch name {
	case "update", "delete", "findandmodify":
		// 全集合改删是 Mongo 最容易造成事故的操作：filter 必须显式且非空
		if emptyFilter(cmd) {
			return kind, errors.New("update / delete / findAndModify 必须带非空 filter，否则会作用于整个集合")
		}
	}
	// 服务端脚本家族：$function / $accumulator 与 $where 等价（4.4+ 起提供），
	// 只拦 $where 等于留了个后门。引擎侧 MongoForbidWhere 判的是同一组（保持同步）。
	for _, key := range []string{"$where", "$function", "$accumulator"} {
		if hasKeyRecursive(cmd, key) {
			return kind, errors.New("禁止使用 " + key + "（在服务端执行脚本）")
		}
	}
	return kind, nil
}

// emptyFilter 判断 update / delete / findAndModify 的条件是否为空
func emptyFilter(cmd bson.D) bool {
	name := CommandName(cmd)

	checkClauses := func(field string) (checked bool, empty bool) {
		v, ok := lookup(cmd, field)
		if !ok {
			return false, false
		}
		arr, ok := toArray(v)
		if !ok || len(arr) == 0 {
			return true, true
		}
		for _, item := range arr {
			clause, ok := toDoc(item)
			if !ok {
				return true, true
			}
			q, ok := lookup(clause, "q")
			if !ok || isEmptyDoc(q) {
				return true, true // 任一子句条件为空即拒绝
			}
		}
		return true, false
	}

	switch name {
	case "update":
		if checked, empty := checkClauses("updates"); checked {
			return empty
		}
		// 旧式写法：条件直接在顶层 q
		q, ok := lookup(cmd, "q")
		return !ok || isEmptyDoc(q)
	case "delete":
		if checked, empty := checkClauses("deletes"); checked {
			return empty
		}
		q, ok := lookup(cmd, "q")
		return !ok || isEmptyDoc(q)
	case "findandmodify":
		q, ok := lookup(cmd, "query")
		return !ok || isEmptyDoc(q)
	}
	return false
}

// filtersOf 取出一条改删命令里所有的查询条件（用于抓前镜像）
func filtersOf(cmd bson.D) []bson.D {
	name := CommandName(cmd)
	var out []bson.D

	collect := func(field string) bool {
		v, ok := lookup(cmd, field)
		if !ok {
			return false
		}
		arr, ok := toArray(v)
		if !ok {
			return false
		}
		for _, item := range arr {
			clause, ok := toDoc(item)
			if !ok {
				continue
			}
			if q, ok := lookup(clause, "q"); ok {
				if d, ok := toDoc(q); ok {
					out = append(out, d)
				}
			}
		}
		return true
	}

	switch name {
	case "update":
		if !collect("updates") {
			if q, ok := lookup(cmd, "q"); ok {
				if d, ok := toDoc(q); ok {
					out = append(out, d)
				}
			}
		}
	case "delete":
		if !collect("deletes") {
			if q, ok := lookup(cmd, "q"); ok {
				if d, ok := toDoc(q); ok {
					out = append(out, d)
				}
			}
		}
	case "findandmodify":
		if q, ok := lookup(cmd, "query"); ok {
			if d, ok := toDoc(q); ok {
				out = append(out, d)
			}
		}
	}
	return out
}

// ---------- bson 小工具 ----------

func lookup(d bson.D, key string) (interface{}, bool) {
	for _, e := range d {
		if strings.EqualFold(e.Key, key) {
			return e.Value, true
		}
	}
	return nil, false
}

// toDoc 把驱动给出的几种文档表示统一成 bson.D
func toDoc(v interface{}) (bson.D, bool) {
	switch x := v.(type) {
	case bson.D:
		return x, true
	case bson.M:
		return mapToDoc(x), true // 顺序丢失，但此处只做字段查找
	case map[string]interface{}:
		return mapToDoc(x), true
	}
	return nil, false
}

func toArray(v interface{}) ([]interface{}, bool) {
	switch x := v.(type) {
	case bson.A:
		return x, true
	case []interface{}:
		return x, true
	}
	return nil, false
}

func mapToDoc(m map[string]interface{}) bson.D {
	d := make(bson.D, 0, len(m))
	for k, v := range m {
		d = append(d, bson.E{Key: k, Value: v})
	}
	return d
}

// isEmptyDoc 判断条件是否为「空」：{} / nil / 空文档
func isEmptyDoc(v interface{}) bool {
	switch x := v.(type) {
	case nil:
		return true
	case bson.D:
		return len(x) == 0
	case bson.M:
		return len(x) == 0
	case map[string]interface{}:
		return len(x) == 0
	}
	return false
}

// hasKeyRecursive 递归查找某个键（用于拦 $where 这类出现在任意层级的操作符）
func hasKeyRecursive(v interface{}, key string) bool {
	switch x := v.(type) {
	case bson.D:
		for _, e := range x {
			if strings.EqualFold(e.Key, key) || hasKeyRecursive(e.Value, key) {
				return true
			}
		}
	case bson.M:
		for k, val := range x {
			if strings.EqualFold(k, key) || hasKeyRecursive(val, key) {
				return true
			}
		}
	case map[string]interface{}:
		for k, val := range x {
			if strings.EqualFold(k, key) || hasKeyRecursive(val, key) {
				return true
			}
		}
	case bson.A:
		for _, item := range x {
			if hasKeyRecursive(item, key) {
				return true
			}
		}
	case []interface{}:
		for _, item := range x {
			if hasKeyRecursive(item, key) {
				return true
			}
		}
	}
	return false
}
