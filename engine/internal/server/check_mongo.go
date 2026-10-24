package server

import (
	"strings"

	"engine/api"
	"engine/internal/mongocheck"
)

// isMongoSource 判断审核目标是否为 MongoDB 数据源（kind 由主程序按 db_type 映射）。
func isMongoSource(src *api.DataSource) bool {
	return src != nil && strings.EqualFold(src.GetKind(), "mongodb")
}

// checkMongo 审核 MongoDB 命令。
// 上游 bytebase 有 SQL 方言规则但没有 Mongo 规则，所以走 internal/mongocheck：
// 按「命令名 + 关键字段」判定，不做 AST 解析。
// req.Mode 决定审哪一种：空/write = 变更命令（申请页、工单执行前），
// query = 只读命令（查询页，写命令直接拒绝）。
// 记录字段与 SQL 侧对齐：Level 1 拦截 / 2 警告 / 3 观察，前端只拦 1。
func checkMongo(req *api.CheckRequest) *api.CheckReply {
	_, findings, err := mongocheck.Check(req.Sql, mongoRulesFrom(req.Rule, req.GetMode()))
	if err != nil {
		return &api.CheckReply{Ok: false, Error: err.Error()}
	}
	rec := findingsRecord(strings.TrimSpace(req.Sql), req.Schema,
		mongocheck.Messages(findings), mongocheck.Severity(findings), mongocheck.Blocked(findings) != nil)
	return &api.CheckReply{Ok: true, Records: []*api.Record{rec}}
}
