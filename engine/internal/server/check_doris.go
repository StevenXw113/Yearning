package server

import (
	"context"
	"fmt"
	"strings"

	enginev1 "engine/gen/engine/v1"
	"engine/internal/dorischeck"
	"engine/internal/mysqlparse"

	storepb "engine/internal/bytebase/generated-go/store"
	"engine/internal/bytebase/plugin/parser/base"
)

// isDorisSource 判断审核目标是否为 Doris 数据源（kind 由主程序按 db_type 映射）。
func isDorisSource(src *enginev1.DataSource) bool {
	return src != nil && strings.EqualFold(src.GetKind(), "doris")
}

// checkDoris 审核 Doris 命令。混合路径：
//   - 建表 DDL：Doris 自有语法，MySQL 解析器审不了，走自研 internal/dorischeck
//     （文本级规则：必须 DISTRIBUTED BY、BUCKETS 上限、副本数、KEY 模型）。
//   - DML/查询：Doris 兼容 MySQL 语法，走 MySQL 解析器 + advisor（不做影响行数估算）。
func checkDoris(ctx context.Context, req *enginev1.CheckRequest) *enginev1.CheckReply {
	if dorischeck.Parse(req.Sql) != nil {
		return checkDorisDDL(req)
	}
	return checkDorisDML(ctx, req)
}

// checkDorisDDL 用 dorischeck 审核建表语句。
func checkDorisDDL(req *enginev1.CheckRequest) *enginev1.CheckReply {
	findings := dorischeck.Check(req.Sql, dorisRulesFrom(req.Rule))
	rec := &enginev1.Record{
		Sql:    strings.TrimSpace(req.Sql),
		Schema: req.Schema,
		Status: "审核通过",
		Level:  0,
	}
	if len(findings) > 0 {
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
	return &enginev1.CheckReply{Ok: true, Records: []*enginev1.Record{rec}}
}

// checkDorisDML 用 MySQL 解析器 + advisor 审核 Doris 的 DML/查询。
// Doris 不做影响行数估算（EXPLAIN 语义与 MySQL 不同，留 0）。
func checkDorisDML(ctx context.Context, req *enginev1.CheckRequest) *enginev1.CheckReply {
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
			// Doris 不做影响行数估算（EXPLAIN 语义与 MySQL 不同，留 0）
		}
		recs = append(recs, rec)
	}
	return &enginev1.CheckReply{Ok: true, Records: recs}
}
