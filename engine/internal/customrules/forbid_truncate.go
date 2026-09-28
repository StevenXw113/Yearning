package customrules

import "strings"

func init() {
	Register(forbidTruncate{})
}

// forbidTruncate 禁止 TRUNCATE —— **新增自定义规则的完整示例**。
//
// 一条自研规则从引擎到页面共四处改动，本文件是第 1 处；其余三处见
// internal/customrules/README.md「如何新增一条规则」：
//
//  1. 本文件：实现 Rule，并在 init 里 Register（规则逻辑）
//  2. rule.go：在 Config 里加开关字段（自研层只认自己的 Config，不依赖上游 proto）
//  3. internal/server/check.go：把 enginev1.AuditRole 的开关映射进 Config
//  4. proto + src/engine/{engine.go,convert.go} + front rules.ts/i18n：把开关透传到审核规则页
//
// 开关语义：DDLForbidTruncate = true 表示禁止 TRUNCATE；默认 false（规则不生效），
// 所以新增自定义规则不会改变线上既有审核行为，想启用时在「审核规则」页勾上即可。
type forbidTruncate struct{}

func (forbidTruncate) Name() string { return "forbid-truncate" }

func (forbidTruncate) Switches() []string { return []string{"DDLForbidTruncate"} }

func (forbidTruncate) Desc() string {
	return "禁止 TRUNCATE（不可回滚，如需清空数据请改用 DELETE 并走审批）"
}

func (forbidTruncate) Check(cfg Config, c Context) []Finding {
	if !cfg.ForbidTruncate {
		return nil
	}
	// 与 forbid-drop 一致：先小写 + 压缩空白，再看是否以关键字开头，
	// 避免把 truncate_log 这类同前缀标识符误伤。
	norm := reMultiSpace.ReplaceAllString(strings.ToLower(strings.TrimSpace(c.SQL)), " ")
	if hasPrefixWord(norm, "truncate") {
		return []Finding{{
			Title:   "forbid-truncate",
			Content: "禁止执行 TRUNCATE 操作（不可回滚，如需清空数据请改用 DELETE 并走审批）",
			Level:   LevelError,
		}}
	}
	return nil
}
