package server

import (
	"context"
	"strings"

	"engine/api"
	"engine/internal/mysqlparse"

	storepb "engine/internal/bytebase/generated-go/store"
)

// isPostgreSQLSource 判断审核目标是否为 PostgreSQL 数据源（kind 由主程序按 db_type 映射）。
func isPostgreSQLSource(src *api.DataSource) bool {
	return src != nil && strings.EqualFold(src.GetKind(), "postgres")
}

// checkPostgreSQL 执行 PostgreSQL 静态审核：拆分 → 逐条 auditSQLStatement。
// 方言固定 PostgreSQL，与 MySQL 各走各的入口；PostgreSQL 侧不做影响行数估算
// （AST 类型不同，留 0）。
func checkPostgreSQL(ctx context.Context, req *api.CheckRequest) *api.CheckReply {
	const engine = storepb.Engine_POSTGRES

	stmts, err := mysqlparse.SplitEngine(engine, req.Sql)
	if err != nil {
		return &api.CheckReply{Ok: false, Error: "SQL 拆分失败: " + err.Error()}
	}
	if len(stmts) == 0 {
		if perr := mysqlparse.CheckEngine(engine, req.Sql); perr != nil {
			return &api.CheckReply{Ok: false, Error: "SQL 语法错误: " + perr.Error()}
		}
		return &api.CheckReply{Ok: true}
	}

	plan := reviewRules(req.Rule, engine)
	recs := make([]*api.Record, 0, len(stmts))
	for _, text := range stmts {
		rec, _ := auditSQLStatement(ctx, engine, text, req.Schema, req.Rule, plan)
		recs = append(recs, rec)
	}
	// 整单级校验：多条 DDL 需规则集显式允许（PostgreSQL 的 DDL 可事务化，但
	// 「一个工单一条 DDL」是产品一致性策略，两方言行为保持一致）
	recs = appendMultiDDLCheck(recs, stmts, req.Rule)
	return &api.CheckReply{Ok: true, Records: recs}
}
