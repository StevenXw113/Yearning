package server

import (
	"strings"

	enginev1 "engine/gen/engine/v1"
	"engine/internal/redischeck"
)

// isRedisSource 判断审核目标是否为 Redis 数据源（kind 由主程序按 db_type 映射）。
func isRedisSource(src *enginev1.DataSource) bool {
	return src != nil && strings.EqualFold(src.GetKind(), "redis")
}

// checkRedis 审核 Redis 命令。
//
// 上游 bytebase 有 SQL 方言规则但没有 Redis 规则
// （common.EngineSupportSQLReview(REDIS) 返回 false，advisor 目录里也没有 redis 方言），
// 所以走 internal/redischeck：按 RESP 参数规则切词后用「命令名 + 参数形态」判定。
// req.Mode 决定审哪一种：空/write = 变更命令，query = 只读命令（写命令直接拒绝）。
// 记录字段与 SQL 侧对齐：Level 1 拦截 / 2 警告 / 3 观察，前端只拦 1。
func checkRedis(req *enginev1.CheckRequest) *enginev1.CheckReply {
	_, findings, err := redischeck.Check(req.Sql, redisRulesFrom(req.Rule, req.GetMode()))
	if err != nil {
		return &enginev1.CheckReply{Ok: false, Error: err.Error()}
	}
	rec := &enginev1.Record{
		Sql:    strings.TrimSpace(req.Sql),
		Schema: req.Schema,
		Status: "审核通过",
		Level:  0,
	}
	if len(findings) > 0 {
		rec.Error = redischeck.Messages(findings)
		rec.Level = uint32(redischeck.Severity(findings))
		rec.Status = "审核不通过"
		if redischeck.Blocked(findings) == nil {
			// warn / observe 只提示、不拦提交（与 SQL 侧的降级语义一致）
			if rec.Level == 2 {
				rec.Status = "警告"
			} else {
				rec.Status = "观察"
			}
		}
	}
	return &enginev1.CheckReply{Ok: true, Records: []*enginev1.Record{rec}}
}
