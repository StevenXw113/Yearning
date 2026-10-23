package mongodb

import (
	"errors"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
)

// readOnlyCommands 查询页允许执行的命令白名单。
//
// 用白名单而不是黑名单：Mongo 的命令空间是开放的（服务端有没有这条命令、
// 是不是只在某个版本存在，客户端猜不全），而 mapReduce 这类命令既能执行服务端
// 脚本又能写数据，"看着像查询"却会改数据。黑名单挡不住没被想到的那一条，
// 白名单至多让某条只读命令暂时用不了——代价方向是对的。
//
// 刻意不含 listDatabases：它会把实例上所有库名暴露给只有单库权限的用户。
var readOnlyCommands = map[string]struct{}{
	// 查询
	"find": {}, "aggregate": {}, "count": {}, "distinct": {}, "explain": {},
	"getmore": {}, "killcursors": {},
	// 元数据
	"listcollections": {}, "listindexes": {}, "collstats": {}, "dbstats": {}, "indexstats": {},
	// 探活/环境（查询页连接后前端会用到，且都不改数据）
	"ping": {}, "hello": {}, "ismaster": {}, "buildinfo": {},
	"serverstatus": {}, "hostinfo": {}, "connectionstatus": {}, "whatsmyuri": {}, "getparameter": {},
}

// ValidateQuery 查询页的硬保底校验：只允许只读命令。
//
// 与 Validate 同类：**不读规则集**、不受配置影响。"查询页不能改数据"这条约束
// 不该依赖谁把开关配对——规则集配错只是少拦一条提示，而这里是绕过审批直接改库。
func ValidateQuery(cmd bson.D) error {
	name := CommandName(cmd)
	if name == "" {
		return errors.New("MongoDB 命令不能为空")
	}
	if _, ok := readOnlyCommands[name]; !ok {
		return errors.New("查询页只能执行只读命令（find / aggregate / count / distinct / listIndexes 等），" +
			name + " 属于变更命令，请到工单申请页提交")
	}
	// aggregate 在白名单里，但 $out / $merge 会把结果写回集合——白名单按命令名判定，
	// 看不见 pipeline，必须在阶段上再判一次。
	if name == "aggregate" {
		if stage, bad := writeStage(cmd); bad {
			return errors.New("aggregate 的 $out / $merge 会把结果写回集合（" + stage + "），查询页不允许执行")
		}
	}
	return nil
}

// writeStage 检查 aggregate 的 pipeline 里有没有写数据的阶段，返回命中的阶段名。
func writeStage(cmd bson.D) (string, bool) {
	v, ok := lookup(cmd, "pipeline")
	if !ok {
		return "", false
	}
	stages, ok := toArray(v)
	if !ok {
		return "", false
	}
	for _, item := range stages {
		stage, ok := toDoc(item)
		if !ok {
			continue
		}
		for _, e := range stage {
			if strings.EqualFold(e.Key, "$out") || strings.EqualFold(e.Key, "$merge") {
				return e.Key, true
			}
		}
	}
	return "", false
}
