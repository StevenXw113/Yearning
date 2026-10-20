// Package mysqlparse 基于 Bytebase 解析器提供 SQL 语句拆分与语法校验。
//
// 名字仍叫 mysqlparse 是历史原因：最早只支持 MySQL。现在按 engine 参数分发，
// 同时支持 MySQL 与 PostgreSQL（pg 是 SQL 方言，同走 parser 的按方言拆分）。
package mysqlparse

import (
	"strings"

	storepb "engine/internal/bytebase/generated-go/store"
	"engine/internal/bytebase/plugin/parser/base"

	// 注册 Bytebase MySQL / PostgreSQL 解析与拆分实现。
	_ "engine/internal/bytebase/plugin/parser/mysql"
	_ "engine/internal/bytebase/plugin/parser/pg"
)

// Split 拆分 MySQL SQL（历史入口，等价 SplitEngine(storepb.Engine_MYSQL, sql)）。
func Split(sql string) ([]string, error) {
	return SplitEngine(storepb.Engine_MYSQL, sql)
}

// SplitEngine 将整段 SQL 按方言拆分为独立语句（去掉首尾空白与结尾分号）。
func SplitEngine(engine storepb.Engine, sql string) ([]string, error) {
	if strings.TrimSpace(sql) == "" {
		return nil, nil
	}
	list, err := base.SplitMultiSQL(engine, sql)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(list))
	for _, s := range list {
		if s.Empty {
			continue
		}
		if text := trim(s.Text); text != "" {
			out = append(out, text)
		}
	}
	return out, nil
}

// Check 校验单条 MySQL SQL 语法（历史入口）。
func Check(sql string) error {
	return CheckEngine(storepb.Engine_MYSQL, sql)
}

// CheckEngine 校验单条 SQL 语法，返回 Bytebase 解析器报告的语法错误。
func CheckEngine(engine storepb.Engine, sql string) error {
	if strings.TrimSpace(sql) == "" {
		return nil
	}
	_, err := base.ParseStatements(engine, sql)
	return err
}

func trim(s string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), ";"))
}
