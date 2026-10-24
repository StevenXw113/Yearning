package server

import (
	"context"
	"strings"

	enginev1 "engine/gen/engine/v1"
	"engine/internal/dorischeck"

	storepb "engine/internal/bytebase/generated-go/store"
)

// isDorisSource 判断审核目标是否为 Doris 数据源（kind 由主程序按 db_type 映射）。
func isDorisSource(src *enginev1.DataSource) bool {
	return src != nil && strings.EqualFold(src.GetKind(), "doris")
}

// checkDoris 审核 Doris 工单。切分与 exec_doris 保持一致（splitDorisStatements），
// 逐条语句路由：
//   - 建表 DDL：Doris 自有语法，MySQL 解析器审不了，走自研 internal/dorischeck
//     （文本级规则：必须 DISTRIBUTED BY、BUCKETS 上限、副本数、KEY 模型）。
//   - DML/查询：Doris 兼容 MySQL 语法，走 MySQL 方言的 auditSQLStatement
//     （不做影响行数估算，EXPLAIN 语义与 MySQL 不同）。
//
// 此前按「整单首句是否 CREATE TABLE」路由：混合工单（DML + DDL）要么被 MySQL
// 拆分器整单误报语法错误，要么 DML 部分整段漏审，且单条 Record 与 exec 侧
// 逐条执行明细对不上。
func checkDoris(ctx context.Context, req *enginev1.CheckRequest) *enginev1.CheckReply {
	stmts := splitDorisStatements(req.Sql)
	if len(stmts) == 0 {
		return &enginev1.CheckReply{Ok: true}
	}

	const engine = storepb.Engine_MYSQL
	plan := reviewRules(req.Rule, engine)
	recs := make([]*enginev1.Record, 0, len(stmts))
	for _, text := range stmts {
		if dorischeck.Parse(text) != nil {
			// 建表 DDL：dorischeck 一条语句一条记录
			findings := dorischeck.Check(text, dorisRulesFrom(req.Rule))
			recs = append(recs, findingsRecord(text, req.Schema,
				dorischeck.Messages(findings), dorischeck.Severity(findings), dorischeck.Blocked(findings) != nil))
			continue
		}
		// DML/查询：MySQL 方言审核（不做影响行数估算）
		rec, _ := auditSQLStatement(ctx, engine, text, req.Schema, req.Rule, plan)
		recs = append(recs, rec)
	}
	return &enginev1.CheckReply{Ok: true, Records: recs}
}
