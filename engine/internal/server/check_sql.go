package server

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	enginev1 "engine/gen/engine/v1"
	"engine/internal/mysqlparse"

	"github.com/bytebase/omni/mysql/ast"

	storepb "engine/internal/bytebase/generated-go/store"
	"engine/internal/bytebase/plugin/parser/base"
	mysqlparser "engine/internal/bytebase/plugin/parser/mysql"
)

// checkSQL 执行 MySQL 静态审核：拆分语句 → 自研规则 → Bytebase MySQL advisor。
// 方言固定 MySQL，不再用 engine 变量 + 条件分支与其他方言混在一起。
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
		rec := &enginev1.Record{Sql: text, Schema: req.Schema, Status: "审核通过", Level: 0}
		if f, ok := runCustomRules(req.Rule, text, req.Schema); ok {
			rec.Error = fmt.Sprintf("[%s] %s", f.Title, f.Content)
			rec.Status, rec.Level = customStatus(f.Level), uint32(f.Level)
			recs = append(recs, rec)
			continue
		}
		parsed, perr := base.ParseStatements(engine, text)
		switch {
		case perr != nil || len(parsed) == 0:
			rec.Error, rec.Status, rec.Level = "SQL 语法错误: "+errText(perr), "语法错误", 1
		default:
			advice, rerr := review(ctx, engine, parsed, req.Schema, plan.error)
			switch {
			case rerr != nil:
				rec.Error, rec.Status, rec.Level = "SQL 语法错误: "+rerr.Error(), "语法错误", 1
			case advice != nil:
				rec.Error, rec.Status, rec.Level = adviceText(advice), "审核不通过", 1
			default:
				rec.Error, rec.Status, rec.Level = downgraded(ctx, engine, parsed, req.Schema, plan)
			}
			// 影响行数估算：MySQL 的 AST 支持，直接做（不再用 if engine==MYSQL 区分）
			if node, ok := mysqlparser.GetOmniNode(parsed[0].AST); ok {
				rec.AffectRows = estimateRows(ctx, &db, req.Source, req.Schema, node, text)
			}
		}
		recs = append(recs, rec)
	}
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
	q := "EXPLAIN FORMAT=JSON " + strings.TrimRight(strings.TrimSpace(stmtText), ";")
	if err := db.QueryRowContext(ctx, q).Scan(&plan); err != nil {
		return 0
	}
	n, err := mysqlparser.EstimateAffectedRowsFromExplainJSON(node, plan)
	if err != nil || n < 0 {
		return 0
	}
	return uint32(n)
}
