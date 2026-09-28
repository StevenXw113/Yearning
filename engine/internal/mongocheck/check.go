// Package mongocheck 实现 MongoDB 命令的静态审核。
//
// 上游 bytebase 不提供 Mongo 审核规则（common.EngineSupportSQLReview(MONGODB)
// 返回 false，advisor 目录里也没有 mongodb 方言），所以这里自研。
//
// 与 SQL 侧的根本差别：SQL 审核建立在语法树之上，而 Mongo 命令就是一段
// extended JSON，没有可审核的 AST。这里退一步，用「命令名 + 若干关键字段 +
// 操作符形态」做约束——够挡住最常见也最危险的操作（全集合改删、服务端脚本、
// 删库、无索引形态的查询）。
//
// 结构：规则注册表（rule.go）+ 共享解析层（command.go）+ 规则开关（config.go），
// 一条规则一个文件、在 init() 里注册。新增规则见 README。
//
// 只用标准库解析：本包不引入 mongo-driver，避免引擎依赖变重。
package mongocheck

import "errors"

// Check 审核一条 MongoDB 命令（extended JSON 文本）。
//
// 返回变更类型与命中的规则；第三个返回值非 nil 表示命令本身不可用：
// 不是合法 JSON、不是变更命令（变更审核下），或是查询审核下出现的写命令。
func Check(cmdJSON string, cfg Config) (int, []Finding, error) {
	cmd, err := Parse(cmdJSON)
	if err != nil {
		return KindQuery, nil, err
	}

	if cfg.queryMode() {
		// 查询页出现写命令：直接拒绝。这不是可配置的规则——
		// 查询页能改数据等于绕过整个工单审批链路，不该依赖谁把开关配对。
		if cmd.Kind != KindQuery {
			return cmd.Kind, nil, errors.New(notReadOnly(cmd.Name))
		}
	} else if cmd.Kind == KindQuery {
		return cmd.Kind, nil, errors.New("不是变更命令（insert / update / delete / createIndexes 等），无需提交工单")
	}

	var out []Finding
	for _, r := range All() {
		if !applies(r, cfg.mode()) {
			continue
		}
		out = append(out, r.Check(cfg, cmd)...)
	}
	return cmd.Kind, out, nil
}

// notReadOnly 查询页的拒绝文案：说清为什么不行、以及该去哪儿做
func notReadOnly(name string) string {
	return "查询页只能执行只读命令（find / aggregate / count / distinct 等），" +
		name + " 属于变更命令，请到工单申请页提交"
}
