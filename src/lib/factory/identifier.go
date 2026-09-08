package factory

import (
	"regexp"
	"strings"
)

// identifierRegexp 限定 MySQL 标识符允许的字符，用于阻断把外部输入拼进 SQL 时的标识符注入。
var identifierRegexp = regexp.MustCompile(`^[\w$\-]+$`)

// IsValidIdentifier 校验库名/表名是否合法，长度遵循 MySQL 64 字符上限。
// 表名/库名无法用占位符参数化，只能先做强校验 + 转义后再拼 SQL。
func IsValidIdentifier(s string) bool {
	return s != "" && len(s) <= 64 && identifierRegexp.MatchString(s)
}

// EscapeIdentifier 在标识符进入反引号前把反引号双写，防止闭合反引号逃逸。
func EscapeIdentifier(s string) string {
	return strings.ReplaceAll(s, "`", "``")
}
