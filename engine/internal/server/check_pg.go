package server

import (
	"context"
	"fmt"
	"strings"

	enginev1 "engine/gen/engine/v1"
	"engine/internal/mysqlparse"

	storepb "engine/internal/bytebase/generated-go/store"
	"engine/internal/bytebase/plugin/parser/base"
)

// isPgSource 判断审核目标是否为 PostgreSQL 数据源（kind 由主程序按 db_type 映射）。
func isPgSource(src *enginev1.DataSource) bool {
	return src != nil && strings.EqualFold(src.GetKind(), "postgres")
}

// checkPG 执行 PostgreSQL 静态审核：拆分语句 → 自研规则 → Bytebase pg advisor。
// 方言固定 pg，与 MySQL 各走各的循环，不再共享同一段「engine 参数 + 条件分支」的逻辑。
func checkPG(ctx context.Context, req *enginev1.CheckRequest) *enginev1.CheckReply {
	const engine = storepb.Engine_POSTGRES

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
			// pg 侧不做影响行数估算（AST 类型不同，暂不估，留 0）
		}
		recs = append(recs, rec)
	}
	return &enginev1.CheckReply{Ok: true, Records: recs}
}
