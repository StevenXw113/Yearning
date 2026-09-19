package server

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	enginev1 "engine/gen/engine/v1"
	"engine/internal/customrules"
	"engine/internal/mysqlparse"

	"github.com/bytebase/omni/mysql/ast"

	storepb "engine/internal/bytebase/generated-go/store"
	bbadvisor "engine/internal/bytebase/plugin/advisor"
	_ "engine/internal/bytebase/plugin/advisor/mysql" // 注册 MySQL 审核规则实现（80+ 条）
	"engine/internal/bytebase/plugin/parser/base"
	mysqlparser "engine/internal/bytebase/plugin/parser/mysql"

	_ "github.com/go-sql-driver/mysql"
)

// Check 执行 SQL 静态审核：先用 Bytebase MySQL 解析器拆分语句，
// 再逐条交给 Bytebase MySQL advisor 按审核规则检查。
func (e *Engine) Check(ctx context.Context, req *enginev1.CheckRequest) (*enginev1.CheckReply, error) {
	if req == nil || strings.TrimSpace(req.Sql) == "" {
		return &enginev1.CheckReply{Ok: false, Error: "SQL 不能为空"}, nil
	}
	stmts, err := mysqlparse.Split(req.Sql)
	if err != nil {
		return &enginev1.CheckReply{Ok: false, Error: "SQL 拆分失败: " + err.Error()}, nil
	}
	if len(stmts) == 0 {
		if perr := mysqlparse.Check(req.Sql); perr != nil {
			return &enginev1.CheckReply{Ok: false, Error: "SQL 语法错误: " + perr.Error()}, nil
		}
		return &enginev1.CheckReply{Ok: true}, nil
	}

	plan := reviewRules(req.Rule)
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
			rec.Status = "审核不通过"
			rec.Level = uint32(f.Level)
			recs = append(recs, rec)
			continue
		}
		parsed, perr := base.ParseStatements(storepb.Engine_MYSQL, text)
		switch {
		case perr != nil || len(parsed) == 0:
			rec.Error, rec.Status, rec.Level = "SQL 语法错误: "+errText(perr), "语法错误", 1
		default:
			advice, rerr := review(ctx, parsed, req.Schema, plan.error)
			switch {
			case rerr != nil:
				rec.Error, rec.Status, rec.Level = "SQL 语法错误: "+rerr.Error(), "语法错误", 1
			case advice != nil:
				rec.Error, rec.Status, rec.Level = adviceText(advice), "审核不通过", 1
			default:
				// 错误级没命中，再看警告/观察：这两档都不拦提交，只体现在检测结果里。
				rec.Error, rec.Status, rec.Level = downgraded(ctx, parsed, req.Schema, plan)
			}
			if node, ok := mysqlparser.GetOmniNode(parsed[0].AST); ok {
				rec.AffectRows = estimateRows(ctx, &db, req.Source, req.Schema, node, text)
			}
		}
		recs = append(recs, rec)
	}
	return &enginev1.CheckReply{Ok: true, Records: recs}, nil
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
		return explainRows(ctx, dbp, src, schema, stmtText)
	}
	return 0
}

// explainRows 对单条 UPDATE/DELETE 执行 EXPLAIN FORMAT=JSON 估算影响行数。
func explainRows(ctx context.Context, dbp **sql.DB, src *enginev1.DataSource, schema, stmtText string) uint32 {
	db := *dbp
	if db == nil {
		if src == nil || src.Ip == "" {
			return 0
		}
		dsn, err := dsnOf(src, schema)
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
	n, ok := mysqlparser.GetEstimatedAffectedRowsFromExplainJSON(plan)
	if !ok || n < 0 {
		return 0
	}
	return uint32(n)
}

// errText 把解析错误安全转成文案。
func errText(err error) string {
	if err == nil {
		return "语句为空"
	}
	return err.Error()
}

// review 对已解析的单条 SQL 依次执行规则，返回最严重的一条建议（error 优先于 warning）。
func review(ctx context.Context, stmts []base.ParsedStatement, schema string, rules []*storepb.SQLReviewRule) (*storepb.Advice, error) {
	size := 0
	for _, s := range stmts {
		size += len(s.Text)
	}
	checkCtx := bbadvisor.Context{
		DBType:              storepb.Engine_MYSQL,
		CurrentDatabase:     schema,
		ParsedStatements:    stmts,
		StatementsTotalSize: size,
	}
	var warn *storepb.Advice
	for _, rule := range rules {
		checkCtx.Rule = rule
		advices, err := bbadvisor.Check(ctx, storepb.Engine_MYSQL, rule.GetType(), checkCtx)
		if err != nil {
			return nil, err
		}
		for _, a := range advices {
			if a.GetStatus() == storepb.Advice_ERROR {
				return a, nil
			}
			if warn == nil && a.GetStatus() == storepb.Advice_WARNING {
				warn = a
			}
		}
	}
	return warn, nil
}

// adviceText 把 Bytebase 建议渲染成检测结果里的错误文案。
func adviceText(a *storepb.Advice) string {
	return fmt.Sprintf("[%s] %s", a.GetTitle(), a.GetContent())
}

// downgraded 在错误级规则未命中时，按「警告 → 观察」继续判定，返回记录需要的
// 文案/状态/级别（Yearning 约定：0 通过 1 错误 2 警告 3 信息，前端只拦 level===1）。
// 两组都不命中时返回空文案，记录保持「审核通过」。
func downgraded(ctx context.Context, stmts []base.ParsedStatement, schema string, plan reviewPlan) (string, string, uint32) {
	for _, g := range []struct {
		rules []*storepb.SQLReviewRule
		text  string
		level uint32
	}{
		{plan.warning, "警告", 2},
		{plan.observe, "观察", 3},
	} {
		advice, err := review(ctx, stmts, schema, g.rules)
		if err != nil || advice == nil {
			continue
		}
		return adviceText(advice), g.text, g.level
	}
	return "", "", 0
}

// runCustomRules 执行自研规则层（internal/customrules），返回最严重的一条命中。
// role 为 nil（未配置审核规则）时不启用自研规则，与历史语义保持一致。
//
// 自研规则与上游规则分属两层：前者在 internal/customrules（同步不覆盖），
// 后者在 internal/bytebase（可被 sync-bytebase.sh 整体重建）。
func runCustomRules(role *enginev1.AuditRole, sql, schema string) (customrules.Finding, bool) {
	if role == nil {
		return customrules.Finding{}, false
	}
	cfg := customrules.Config{
		AllowDropDatabase: role.GetDdlEnableDropDatabase(),
		AllowDropTable:    role.GetDdlEnableDropTable(),
	}
	ctx := customrules.Context{SQL: sql, Schema: schema}

	var hit []customrules.Finding
	for _, r := range customrules.All() {
		hit = append(hit, r.Check(cfg, ctx)...)
	}
	if len(hit) == 0 {
		return customrules.Finding{}, false
	}
	best := hit[0]
	for _, f := range hit[1:] {
		if f.Level < best.Level { // 数值越小越严重
			best = f
		}
	}
	return best, true
}
