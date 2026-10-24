package server

import (
	"context"
	"database/sql"
	"fmt"

	enginev1 "engine/gen/engine/v1"
	"engine/internal/mysqlparse"

	"github.com/bytebase/omni/mysql/ast"

	storepb "engine/internal/bytebase/generated-go/store"
	mysqlparser "engine/internal/bytebase/plugin/parser/mysql"
)

// checkSQL 执行 MySQL 静态审核：拆分 → 逐条 auditSQLStatement → 影响行数估算。
// 方言固定 MySQL；估算与行数上限是 MySQL 专属增强，挂在共享审核结果之上。
func checkSQL(ctx context.Context, req *enginev1.CheckRequest) *enginev1.CheckReply {
	const engine = storepb.Engine_MYSQL

	stmts, err := mysqlparse.SplitEngine(engine, req.Sql)
	if err != nil {
		return &enginev1.CheckReply{Ok: false, Error: "SQL 拆分失败: " + err.Error()}
	}
	if len(stmts) == 0 {
		if perr := mysqlparse.CheckEngine(engine, req.Sql); perr != nil {
			return &enginev1.CheckReply{Ok: false, Error: "SQL 语法错误: " + perr.Error()}
		}
		return &enginev1.CheckReply{Ok: true}
	}

	plan := reviewRules(req.Rule, engine)
	recs := make([]*enginev1.Record, 0, len(stmts))
	var db *sql.DB // 懒连接：仅 UPDATE/DELETE 估算影响行数时使用
	defer func() {
		if db != nil {
			db.Close()
		}
	}()
	for _, text := range stmts {
		rec, parsed := auditSQLStatement(ctx, engine, text, req.Schema, req.Rule, plan)
		// 影响行数估算：MySQL 的 AST 支持，直接做（语法解析失败时 parsed 为空，跳过）
		if len(parsed) == 0 {
			recs = append(recs, rec)
			continue
		}
		if node, ok := mysqlparser.GetOmniNode(parsed[0].AST); ok {
			rec.AffectRows = estimateRows(ctx, &db, req.Source, req.Schema, node, text)
			// 审核期用估算值比对规则集上限（执行期还会用真实影响行数再比对一次）；
			// 估算只覆盖 DML，DDL 工单的 MaxDDLAffectRows 由执行期兜底
			if over, limit := affectRowOverLimit(req.Rule, "dml", rec.AffectRows); over {
				rec.Error = fmt.Sprintf("预估影响行数 %d 超过规则集上限 %d", rec.AffectRows, limit)
				rec.Status, rec.Level = "审核不通过", 1
			}
		}
		recs = append(recs, rec)
	}
	// 整单级校验：多条 DDL 需规则集显式允许（见 appendMultiDDLCheck）
	recs = appendMultiDDLCheck(recs, stmts, req.Rule)
	return &enginev1.CheckReply{Ok: true, Records: recs}
}

// estimateRows 估算单条语句影响行数，best-effort：连不上库或无法判断时记 0，不影响审核结果。
// INSERT/REPLACE 按 VALUES 元组数；UPDATE/DELETE 用 EXPLAIN FORMAT=JSON 估算（与 Bytebase 同策略）。
func estimateRows(ctx context.Context, dbp **sql.DB, src *enginev1.DataSource, schema string, node ast.Node, stmtText string) uint32 {
	switch s := node.(type) {
	case *ast.InsertStmt:
		if s.Select == nil { // INSERT ... SELECT 行数取决于数据，无法静态估算
			return uint32(len(s.Values))
		}
	case *ast.UpdateStmt, *ast.DeleteStmt:
		return explainRows(ctx, dbp, src, schema, node, stmtText)
	}
	return 0
}

// explainRows 对单条 UPDATE/DELETE 执行 EXPLAIN FORMAT=JSON 估算影响行数。
// node 是该语句的 AST：上游 3.23.0 起解析函数需要它来判断 DML 目标表
// （旧版只吃 plan 字符串）。
func explainRows(ctx context.Context, dbp **sql.DB, src *enginev1.DataSource, schema string, node ast.Node, stmtText string) uint32 {
	db := *dbp
	if db == nil {
		if src == nil || src.Ip == "" {
			return 0
		}
		dsn, err := mysqlDSN(src, schema)
		if err != nil {
			return 0
		}
		d, err := sql.Open("mysql", dsn)
		if err != nil {
			return 0
		}
		if err := d.PingContext(ctx); err != nil {
			d.Close()
			return 0
		}
		*dbp, db = d, d
	}
	var plan string
	q := "EXPLAIN FORMAT=JSON " + trimStmtSemicolon(stmtText)
	if err := db.QueryRowContext(ctx, q).Scan(&plan); err != nil {
		return 0
	}
	n, err := mysqlparser.EstimateAffectedRowsFromExplainJSON(node, plan)
	if err != nil || n < 0 {
		return 0
	}
	return uint32(n)
}

// trimStmtSemicolon 去掉语句结尾分号与空白，避免拼进 EXPLAIN 前缀后语法错误。
func trimStmtSemicolon(s string) string {
	out := s
	for len(out) > 0 {
		last := out[len(out)-1]
		if last == ';' || last == ' ' || last == '\t' || last == '\r' || last == '\n' {
			out = out[:len(out)-1]
			continue
		}
		break
	}
	return out
}
