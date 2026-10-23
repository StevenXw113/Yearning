package server

import (
	"context"
	"strings"

	enginev1 "engine/gen/engine/v1"
	"engine/internal/mongodb"
	"engine/internal/rediscmd"

	"go.mongodb.org/mongo-driver/bson"
)

// execCommand 执行命令式引擎（MongoDB / Redis）的变更工单。
//
// SQL 走 Exec 里的拆分执行路径（sql.Open + 逐条执行 + binlog 回滚）；
// 这里按 kind 分派到 mongodb / rediscmd：硬保底、前镜像、执行、回滚都在引擎侧完成，
// 结果映射回 ExecReply，与 SQL 同一形状——主程序侧的落库逻辑（Records/Rollback）
// 因此不需要知道数据源类型。
func (e *Engine) execCommand(ctx context.Context, req *enginev1.ExecRequest) (*enginev1.ExecReply, error) {
	switch strings.ToLower(req.Source.GetKind()) {
	case "mongodb":
		return e.execMongo(ctx, req)
	case "redis":
		return e.execRedis(ctx, req)
	default:
		return &enginev1.ExecReply{Ok: false, Error: "不支持的引擎类型: " + req.Source.GetKind()}, nil
	}
}

func (e *Engine) execMongo(ctx context.Context, req *enginev1.ExecRequest) (*enginev1.ExecReply, error) {
	var cmd bson.D
	if err := bson.UnmarshalExtJSON([]byte(req.Order.Sql), false, &cmd); err != nil {
		return &enginev1.ExecReply{Ok: false, Error: "工单内容不是合法的 MongoDB 命令 JSON：" + err.Error()}, nil
	}
	// 执行前再过一遍硬保底（与提交时同一套）：提交到审批之间工单内容可能被改过
	if _, err := mongodb.Validate(cmd); err != nil {
		return &enginev1.ExecReply{Ok: false, Error: err.Error()}, nil
	}
	maxAffectRows := 0
	if req.Rules != nil {
		maxAffectRows = int(req.Rules.MongoMaxAffectRows)
	}

	client, err := connectMongo(req.Source)
	if err != nil {
		return &enginev1.ExecReply{Ok: false, Error: "连接 MongoDB 失败: " + err.Error()}, nil
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	// 注意（回归点）：createIndexes 的实时构建进度此前由主程序写 osc_info，
	// 引擎没有主程序的元数据库，这里不再挂 WatchIndexBuild 进度回传；
	// 索引构建完成与否仍由执行结果体现。见 design.md 语义核对第 4 条。
	out, err := mongodb.Execute(ctx, client, req.Order.DataBase, cmd, maxAffectRows)
	if err != nil {
		return &enginev1.ExecReply{Ok: false, Error: err.Error(),
			Records: []*enginev1.Record{{Sql: req.Order.Sql, Status: "执行失败", Level: 1, Error: err.Error()}}}, nil
	}

	rec := &enginev1.Record{Sql: req.Order.Sql, Status: "已执行", AffectRows: uint32(out.Affected)}
	if out.Warning != "" {
		rec.Error = out.Warning
	}
	return &enginev1.ExecReply{Ok: true, Records: []*enginev1.Record{rec}, Rollback: out.Rollback}, nil
}

func (e *Engine) execRedis(ctx context.Context, req *enginev1.ExecRequest) (*enginev1.ExecReply, error) {
	// 执行前过硬保底（与提交时同一套）
	if _, _, err := rediscmd.Validate(req.Order.Sql); err != nil {
		return &enginev1.ExecReply{Ok: false, Error: err.Error()}, nil
	}
	client, err := connectRedisDB(req.Source, redisParseDB(req.Order.DataBase))
	if err != nil {
		return &enginev1.ExecReply{Ok: false, Error: "连接 Redis 失败: " + err.Error()}, nil
	}
	defer func() { _ = client.Close() }()

	out, err := rediscmd.Execute(ctx, client, req.Order.Sql)
	if err != nil {
		return &enginev1.ExecReply{Ok: false, Error: err.Error(),
			Records: []*enginev1.Record{{Sql: req.Order.Sql, Status: "执行失败", Level: 1, Error: err.Error()}}}, nil
	}

	rec := &enginev1.Record{Sql: req.Order.Sql, Status: "已执行", AffectRows: uint32(out.Affected)}
	return &enginev1.ExecReply{Ok: true, Records: []*enginev1.Record{rec}, Rollback: out.Rollback}, nil
}
