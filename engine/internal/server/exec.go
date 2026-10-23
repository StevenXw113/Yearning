package server

import (
	"context"
	"strings"

	enginev1 "engine/gen/engine/v1"
)

// Exec 按数据源方言分派执行。四个引擎各自独立实现、互不混用：
//   - mongodb  → execMongo（exec_mongo.go，主程序驱动 + 前镜像回滚）
//   - redis    → execRedis（exec_redis.go）
//   - postgres → execPG（exec_pg.go，pgx + SET search_path，无 binlog 回滚）
//   - 其余     → execSQL（exec_mysql.go，MySQL；未知 kind 按 MySQL 处理以兼容旧调用方）
func (e *Engine) Exec(ctx context.Context, req *enginev1.ExecRequest) (*enginev1.ExecReply, error) {
	if req == nil || req.Order == nil {
		return &enginev1.ExecReply{Ok: false, Error: "工单为空"}, nil
	}
	if strings.TrimSpace(req.Order.Sql) == "" {
		return &enginev1.ExecReply{Ok: false, Error: "工单 SQL 为空"}, nil
	}

	switch strings.ToLower(req.Source.GetKind()) {
	case "mongodb":
		return e.execMongo(ctx, req)
	case "redis":
		return e.execRedis(ctx, req)
	case "postgres":
		return e.execPG(ctx, req)
	default:
		return e.execSQL(ctx, req)
	}
}

// orderType 工单类型 → dml/ddl。SQL 引擎（MySQL / pg）判断是否包事务时用。
func orderType(t int32) string {
	if t == 1 {
		return "dml"
	}
	return "ddl"
}
