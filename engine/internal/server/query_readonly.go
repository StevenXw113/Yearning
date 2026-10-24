package server

import "strings"

// 查询页只读校验（引擎侧的纵深防御）：主程序本地执行是第一道白名单，
// 引擎在预检时再拦一次——它已经逐条解析了语句，顺手判只读是零成本，
// 且新引擎接入查询页时自动获得同一防线。
//
// 词法级判定（不依赖各方言 AST）：首关键字白名单 + WITH 语句的顶层写关键字
// 扫描。扫描器跳过引号（含双写转义与字符串反斜杠转义）与注释，字符串里的
// "delete" 不会误判，注释里藏关键字也骗不过。

// queryReadOnlyPrefixes 查询页允许的语句首关键字。WITH 单独处理：
// PostgreSQL 支持 WITH ... INSERT/UPDATE/DELETE，不能直接放行。
var queryReadOnlyPrefixes = map[string]bool{
	"select": true, "show": true, "desc": true, "describe": true,
	"explain": true, "table": true, "values": true, "with": true,
}

// queryWriteKeywords WITH 语句顶层禁现的写/管理关键字。
var queryWriteKeywords = []string{
	"insert", "update", "delete", "replace", "merge", "create", "drop",
	"alter", "truncate", "rename", "grant", "revoke", "set", "call",
	"copy", "execute", "lock", "vacuum", "comment", "use", "prepare", "refresh",
}

// isReadOnlyQuery 判定一条语句是否只读。
func isReadOnlyQuery(text string) bool {
	first := firstTopLevelKeyword(text)
	if first == "" || !queryReadOnlyPrefixes[first] {
		return false
	}
	if first != "with" {
		return true
	}
	for _, kw := range queryWriteKeywords {
		if keywordInCode(text, kw) {
			return false
		}
	}
	return true
}

// firstTopLevelKeyword 返回顶层（深度 0、引号与注释外）的第一个词，小写。
func firstTopLevelKeyword(s string) string {
	out := ""
	scanSQL(s, true, func(i int) bool {
		if !isWordByte(s[i]) || (i > 0 && isWordByte(s[i-1])) {
			return false // 还没到词首
		}
		j := i
		for j < len(s) && isWordByte(s[j]) {
			j++
		}
		out = strings.ToLower(s[i:j])
		return true
	})
	return out
}

// topLevelKeywordAt 判断 keyword 是否作为独立的顶层关键字出现（大小写不敏感，
// 括号内不算）。供 mergeSafe 判 ALTER 子句级关键字。
func topLevelKeywordAt(s, keyword string) bool {
	found := false
	scanSQL(s, true, func(i int) bool {
		if keywordAt(s, i, keyword) {
			found = true
			return true
		}
		return false
	})
	return found
}

// keywordInCode 判断 keyword 是否出现在代码区的任意位置（引号与注释外，
// 括号内也算）。CTE 体在括号里，WITH 后接写操作必须连括号一起扫。
func keywordInCode(s, keyword string) bool {
	found := false
	scanSQL(s, false, func(i int) bool {
		if keywordAt(s, i, keyword) {
			found = true
			return true
		}
		return false
	})
	return found
}

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

// scanSQL 自左向右扫描，跳过引号与注释（-- / # / 区块），对代码区的字节位置
// 调用 visit；visit 返回 true 时停止。parenSkip 为 true 时只回调深度 0 的位置
// （顶层子句级），false 时括号内也回调。
func scanSQL(s string, parenSkip bool, visit func(i int) bool) {
	depth, quote := 0, byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == '\\' && quote != '`' && i+1 < len(s) {
				i++
				continue
			}
			if c == quote {
				if i+1 < len(s) && s[i+1] == quote { // 引号双写是转义
					i++
					continue
				}
				quote = 0
			}
			continue
		}
		switch {
		case c == '\'' || c == '"' || c == '`':
			quote = c
		case c == '-' && i+1 < len(s) && s[i+1] == '-' && (i+2 >= len(s) || isSpaceByte(s[i+2])):
			for ; i < len(s) && s[i] != '\n'; i++ {
			}
		case c == '#':
			for ; i < len(s) && s[i] != '\n'; i++ {
			}
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			if end := strings.Index(s[i+2:], "*/"); end >= 0 {
				i += 2 + end + 1
			} else {
				i = len(s) // 未闭合注释吞掉剩余
			}
		case c == '(':
			depth++
		case c == ')':
			if depth > 0 {
				depth--
			}
		default:
			if (!parenSkip || depth == 0) && visit(i) {
				return
			}
		}
	}
}
