package customrules

import (
	"regexp"
	"strings"
)

func init() {
	Register(forbidDrop{})
}

var reMultiSpace = regexp.MustCompile(`\s+`)

// forbidDrop 禁止 DROP DATABASE / DROP TABLE。
//
// 语义来自 Yearning 的 AuditRole：DDLEnableDropDatabase / DDLEnableDropTable
// 表示“允许删除”，关闭即禁止。Bytebase 规则集没有对应的静态规则
// （DROP 的拦截在 Bytebase 里由执行/审批链承担），故放在自定义规则层。
//
// 这是新增自定义规则的模板：实现 Name/Check，在 init 中 Register 即可。
//   - 只读 Context.SQL / Context.Schema 与 Config，不要 import internal/bytebase；
//   - 返回 nil 表示未命中；命中项会被 server 转成统一的 Record。
type forbidDrop struct{}

func (forbidDrop) Name() string { return "forbid-drop" }

func (forbidDrop) Check(cfg Config, c Context) []Finding {
	norm := reMultiSpace.ReplaceAllString(strings.ToLower(strings.TrimSpace(c.SQL)), " ")
	switch {
	case !cfg.AllowDropDatabase && hasPrefixWord(norm, "drop database", "drop schema"):
		return []Finding{{Title: "forbid-drop", Content: "禁止执行 DROP DATABASE 操作", Level: LevelError}}
	case !cfg.AllowDropTable && hasPrefixWord(norm, "drop table"):
		return []Finding{{Title: "forbid-drop", Content: "禁止执行 DROP TABLE 操作", Level: LevelError}}
	}
	return nil
}

// hasPrefixWord 判断 s 是否以给定短语开头（短语后须紧跟空白或 "("，避免误伤前缀相同的标识符）。
func hasPrefixWord(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p+" ") || strings.HasPrefix(s, p+"(") {
			return true
		}
	}
	return false
}
