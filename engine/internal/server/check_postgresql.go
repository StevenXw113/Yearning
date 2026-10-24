package server

import (
	"context"
	"strings"

	enginev1 "engine/gen/engine/v1"
	"engine/internal/mysqlparse"

	storepb "engine/internal/bytebase/generated-go/store"
)

// isPostgreSQLSource 判断审核目标是否为 PostgreSQL 数据源（kind 由主程序按 db_type 映射）。
func isPostgreSQLSource(src *enginev1.DataSource) bool {
	return src != nil && strings.EqualFold(src.GetKind(), "postgres")
}

// checkPostgreSQL 执行 PostgreSQL 静态审核：拆分 → 逐条 auditSQLStatement。
// 方言固定 PostgreSQL，与 MySQL 各走各的入口；PostgreSQL 侧不做影响行数估算
// （AST 类型不同，留 0）。
func checkPostgreSQL(ctx context.Context, req *enginev1.CheckRequest) *enginev1.CheckReply {
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
		rec, _ := auditSQLStatement(ctx, engine, text, req.Schema, req.Rule, plan)
		recs = append(recs, rec)
	}
	return &enginev1.CheckReply{Ok: true, Records: recs}
}
