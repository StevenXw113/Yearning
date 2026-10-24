package server

import (
	"context"
	"fmt"
	"strings"

	enginev1 "engine/gen/engine/v1"
	"engine/internal/dorischeck"

	storepb "engine/internal/bytebase/generated-go/store"
	"engine/internal/bytebase/plugin/parser/base"
)

// isDorisSource 判断审核目标是否为 Doris 数据源（kind 由主程序按 db_type 映射）。
func isDorisSource(src *enginev1.DataSource) bool {
	return src != nil && strings.EqualFold(src.GetKind(), "doris")
}

// checkDoris 审核 Doris 工单。切分与 exec_doris 保持一致（splitDorisStatements），
// 逐条语句路由：
//   - 建表 DDL：Doris 自有语法，MySQL 解析器审不了，走自研 internal/dorischeck
//     （文本级规则：必须 DISTRIBUTED BY、BUCKETS 上限、副本数、KEY 模型）。
//   - DML/查询：Doris 兼容 MySQL 语法，走 MySQL 解析器 + advisor（不做影响行数估算）。
//
// 此前按「整单首句是否 CREATE TABLE」路由：混合工单（DML + DDL）要么被 MySQL
// 拆分器整单误报语法错误，要么 DML 部分整段漏审，且单条 Record 与 exec 侧
// 逐条执行明细对不上。
func checkDoris(ctx context.Context, req *enginev1.CheckRequest) *enginev1.CheckReply {
	stmts := splitDorisStatements(req.Sql)
	if len(stmts) == 0 {
		return &enginev1.CheckReply{Ok: true}
	}

	plan := reviewRules(req.Rule, storepb.Engine_MYSQL)
	recs := make([]*enginev1.Record, 0, len(stmts))
	for _, text := range stmts {
		if dorischeck.Parse(text) != nil {
			recs = append(recs, checkDorisDDLStmt(text, req))
			continue
		}
		recs = append(recs, checkDorisDMLStmt(ctx, text, req, plan))
	}
	return &enginev1.CheckReply{Ok: true, Records: recs}
}

// checkDorisDDLStmt 用 dorischeck 审核单条建表语句，一条语句一条记录。
func checkDorisDDLStmt(text string, req *enginev1.CheckRequest) *enginev1.Record {
	rec := &enginev1.Record{
		Sql:    text,
		Schema: req.Schema,
		Status: "审核通过",
		Level:  0,
	}
	if findings := dorischeck.Check(text, dorisRulesFrom(req.Rule)); len(findings) > 0 {
		rec.Error = dorischeck.Messages(findings)
		rec.Level = uint32(dorischeck.Severity(findings))
		rec.Status = "审核不通过"
		if dorischeck.Blocked(findings) == nil {
			// warn / observe 只提示、不拦提交（与 SQL 侧降级语义一致）
			if rec.Level == 2 {
				rec.Status = "警告"
			} else {
				rec.Status = "观察"
			}
		}
	}
	return rec
}

// checkDorisDMLStmt 用 MySQL 解析器 + advisor 审核单条 DML/查询语句。
// Doris 不做影响行数估算（EXPLAIN 语义与 MySQL 不同，留 0）。
func checkDorisDMLStmt(ctx context.Context, text string, req *enginev1.CheckRequest, plan reviewPlan) *enginev1.Record {
	const engine = storepb.Engine_MYSQL

	rec := &enginev1.Record{Sql: text, Schema: req.Schema, Status: "审核通过", Level: 0}
	if f, ok := runCustomRules(req.Rule, text, req.Schema); ok {
		rec.Error = fmt.Sprintf("[%s] %s", f.Title, f.Content)
		rec.Status, rec.Level = customStatus(f.Level), uint32(f.Level)
		return rec
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
		// Doris 不做影响行数估算（EXPLAIN 语义与 MySQL 不同，留 0）
	}
	return rec
}
