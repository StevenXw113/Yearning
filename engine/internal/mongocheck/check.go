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
// 审核与执行共用 Extended JSON 解析器。
package mongocheck

import (
	"errors"

	"engine/mongoguard"
)

// Check 审核一条 MongoDB 命令（extended JSON 文本）。
//
// 返回变更类型与命中的规则；第三个返回值非 nil 表示命令本身不可用：
// 不是合法 JSON、不是变更命令（变更审核下），或是查询审核下出现的写命令。
func Check(cmdJSON string, cfg Config) (int, []Finding, error) {
	doc, err := mongoguard.Parse(cmdJSON)
	if err != nil {
		return KindQuery, nil, err
	}
	if cfg.queryMode() {
		doc, err = mongoguard.ValidateQuery(doc)
		if err != nil {
			return KindQuery, nil, err
		}
	}
	cmd := commandFromBSON(doc)
	if !cfg.queryMode() && cmd.Kind == KindQuery {
		return cmd.Kind, nil, errors.New("不是变更命令（insert / update / delete / createIndexes 等），无需提交工单")
	}

	var out []Finding
	dbaRuleID, dbaMessage, dbaRequired := DBAApprovalRequirement(cmd)
	for _, r := range All() {
		if !applies(r, cfg.mode()) {
			continue
		}
		if dbaRequired && dbaBaselineRuleSuppressed(cmd, r.Name()) {
			continue
		}
		out = append(out, r.Check(cfg, cmd)...)
	}
	if dbaRequired {
		out = append(out, Finding{
			Rule:                dbaLegacyRule(cmd.Name),
			RuleID:              dbaRuleID,
			Level:               LevelError,
			Message:             dbaMessage,
			RequiresDBAApproval: true,
		})
	}
	for i := range out {
		if out[i].RuleID == "" {
			out[i].RuleID = RuleID(out[i].Rule)
		}
	}
	return cmd.Kind, out, nil
}

// dbaLegacyRule 返回硬底线对应的旧规则名，便于存量前端按 Rule 字段分组展示。
func dbaLegacyRule(command string) string {
	if _, ok := adminCommands[command]; ok {
		return RuleAdminCommand
	}
	return RuleDropCollection
}
