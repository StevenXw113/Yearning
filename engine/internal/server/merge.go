package server

import (
	"context"
	"regexp"
	"strings"

	enginev1 "engine/gen/engine/v1"
	"engine/internal/mysqlparse"
)

// reAlterTable 匹配 "ALTER TABLE <表名>" 前缀，表名支持反引号或裸标识符。
var reAlterTable = regexp.MustCompile(`(?i)^ALTER\s+TABLE\s+(?P<name>` + "`[^`]+`|[A-Za-z0-9_$.]+" + `)\s+(?P<body>.+)$`)

// MergeAlterTables 将针对同一张表的多条 ALTER TABLE 合并为一条。
// - 若所有语句针对同一张表，则输出一条合并后的 ALTER TABLE；
// - 若涉及多张表或含非 ALTER 语句，则保留原样并以分号连接。
func (e *Engine) MergeAlterTables(_ context.Context, req *enginev1.MergeAlterTablesRequest) (*enginev1.MergeAlterTablesReply, error) {
	if req == nil || strings.TrimSpace(req.Sqls) == "" {
		return &enginev1.MergeAlterTablesReply{Ok: false, Error: "SQL 不能为空"}, nil
	}
	stmts, err := mysqlparse.Split(req.Sqls)
	if err != nil {
		return &enginev1.MergeAlterTablesReply{Ok: false, Error: err.Error()}, nil
	}
	if len(stmts) == 0 {
		return &enginev1.MergeAlterTablesReply{Ok: false, Error: "未解析到有效 SQL"}, nil
	}

	// 归一化每条：表名 + 动作体。
	type alter struct {
		table string
		body  string
	}
	var alts []alter
	allAlter := true
	for _, st := range stmts {
		m := reAlterTable.FindStringSubmatch(st.Text)
		if m == nil {
			allAlter = false
			break
		}
		alts = append(alts, alter{table: normalizeIdent(m[1]), body: strings.TrimSpace(m[2])})
	}

	if !allAlter {
		// 含非 ALTER TABLE 或跨类型语句：无法安全合并，原样返回。
		parts := make([]string, 0, len(stmts))
		for _, st := range stmts {
			parts = append(parts, st.Text)
		}
		return &enginev1.MergeAlterTablesReply{Ok: true, Sql: strings.Join(parts, "; ")}, nil
	}

	// 校验是否同一张表。
	first := alts[0].table
	sameTable := true
	for _, a := range alts {
		if a.table != first {
			sameTable = false
			break
		}
	}
	if !sameTable {
		parts := make([]string, 0, len(stmts))
		for _, st := range stmts {
			parts = append(parts, st.Text)
		}
		return &enginev1.MergeAlterTablesReply{Ok: true, Sql: strings.Join(parts, "; ")}, nil
	}

	// 同一张表：合并动作子句。
	bodies := make([]string, 0, len(alts))
	for _, a := range alts {
		bodies = append(bodies, a.body)
	}
	merged := "ALTER TABLE " + alts[0].table + " " + strings.Join(bodies, ", ")
	return &enginev1.MergeAlterTablesReply{Ok: true, Sql: merged}, nil
}

// normalizeIdent 统一标识符（去掉反引号）。
func normalizeIdent(s string) string {
	return strings.Trim(s, "`")
}
