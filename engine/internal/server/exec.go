package server

import (
	"context"
	"strings"

	enginev1 "engine/gen/engine/v1"
)

// Exec 按数据源方言分派执行。五个引擎各自独立实现、互不混用：
//   - mongodb  → execMongo（exec_mongo.go，主程序驱动 + 前镜像回滚）
//   - redis    → execRedis（exec_redis.go）
//   - postgres → execPG（exec_pg.go，pgx + 连接参数 search_path，无 binlog 回滚）
//   - doris    → execDoris（exec_doris.go，mysql 协议直连，无事务无回滚）
//   - mysql / 空 kind → execSQL（exec_mysql.go；不填 kind 的历史调用方按 MySQL）
//
// 其余未知 kind 显式拒绝（fail-closed）：宁可报错也不能落到错误方言上执行。
func (e *Engine) Exec(ctx context.Context, req *enginev1.ExecRequest) (*enginev1.ExecReply, error) {
	if req == nil || req.Order == nil {
		return &enginev1.ExecReply{Ok: false, Error: "工单为空"}, nil
	}
	if strings.TrimSpace(req.Order.Sql) == "" {
		return &enginev1.ExecReply{Ok: false, Error: "工单 SQL 为空"}, nil
	}

	switch k := strings.ToLower(strings.TrimSpace(req.Source.GetKind())); k {
	case "", "mysql":
		return e.execSQL(ctx, req)
	case "mongodb":
		return e.execMongo(ctx, req)
	case "redis":
		return e.execRedis(ctx, req)
	case "postgres":
		return e.execPG(ctx, req)
	case "doris":
		return e.execDoris(ctx, req)
	default:
		return &enginev1.ExecReply{Ok: false, Error: "引擎暂不支持该数据源类型: " + k + "，请升级引擎或检查数据源配置"}, nil
	}
}

// orderType 工单类型 → dml/ddl。SQL 引擎（MySQL / pg）判断是否包事务时用。
func orderType(t int32) string {
	if t == 1 {
		return "dml"
	}
	return "ddl"
}

// affectRowOverLimit 判断真实影响行数是否超过规则集上限（0 = 不限制）。
// DML 工单用 MaxAffectRows，DDL 工单用 MaxDdlAffectRows，与开关文案语义一致；
// 三条 SQL 执行路径（mysql / pg / doris）共用，执行期逐条语句比对。
func affectRowOverLimit(rules *enginev1.AuditRole, tp string, rows uint32) (bool, uint32) {
	var limit uint32
	if rules != nil {
		if tp == "dml" {
			limit = rules.GetMaxAffectRows()
		} else {
			limit = rules.GetMaxDdlAffectRows()
		}
	}
	return limit > 0 && rows > limit, limit
}

// attachRollbackWarning 把回滚抓取/生成的异常挂到最后一条执行记录上。
// 状态保持「已执行/执行失败」不变、只补说明——与 Mongo 执行器的 Warning
// 呈现方式一致（rec.Error 随执行明细落库展示）。静默吞掉会让 backup=1 的
// 工单在审批人不知情的情况下没有回滚语句。
func attachRollbackWarning(recs []*enginev1.Record, warn string) {
	if warn == "" || len(recs) == 0 {
		return
	}
	last := recs[len(recs)-1]
	if last.Error == "" {
		last.Error = warn
		return
	}
	last.Error += "\n" + warn
}
