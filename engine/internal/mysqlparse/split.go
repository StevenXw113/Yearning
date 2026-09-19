// Package mysqlparse 基于 Bytebase MySQL 解析器（github.com/bytebase/parser/mysql）
// 提供 SQL 语句拆分与语法校验。
package mysqlparse

import (
	"strings"

	storepb "engine/internal/bytebase/generated-go/store"
	"engine/internal/bytebase/plugin/parser/base"

	// 注册 Bytebase MySQL 解析与拆分实现。
	_ "engine/internal/bytebase/plugin/parser/mysql"
)

// engine 为本模块当前支持的数据库引擎。
const engine = storepb.Engine_MYSQL

// Split 将整段 SQL 拆分为独立语句（去掉首尾空白与结尾分号）。
func Split(sql string) ([]string, error) {
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

// Check 校验单条 SQL 语法，返回 Bytebase 解析器报告的语法错误。
func Check(sql string) error {
	if strings.TrimSpace(sql) == "" {
		return nil
	}
	_, err := base.ParseStatements(engine, sql)
	return err
}

func trim(s string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), ";"))
}
