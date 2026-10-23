package server

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	enginev1 "engine/gen/engine/v1"
	"engine/internal/rediscmd"

	"github.com/redis/go-redis/v9"
)

// redisTimeout 建立连接与探活的超时。
const redisTimeout = 15 * time.Second

// redisOptions 按 DataSource 构造客户端选项。口令为空表示未开认证（Redis 允许无密码）。
func redisOptions(ds *enginev1.DataSource) *redis.Options {
	opt := &redis.Options{
		Addr:         net.JoinHostPort(ds.Ip, strconv.Itoa(int(ds.Port))),
		Password:     ds.Password,
		DB:           0,
		DialTimeout:  redisTimeout,
		ReadTimeout:  redisTimeout,
		WriteTimeout: redisTimeout,
	}
	if ds.Username != "" {
		opt.Username = ds.Username
	}
	return opt
}

// connectRedisDB 建连并选中指定逻辑库（go-redis 在建连时按 Options.DB 发 SELECT）。
// 口令是明文：主程序在发起 RPC 前已经把口令列解密好了。
func connectRedisDB(ds *enginev1.DataSource, db int) (*redis.Client, error) {
	if ds == nil || ds.Ip == "" {
		return nil, fmt.Errorf("数据源 IP 为空")
	}
	opt := redisOptions(ds)
	if db > 0 {
		opt.DB = db
	}
	client := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), redisTimeout)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

// redisParseDB 把查询页传来的库名（dbN）解析成库号，非法值返回 0。
func redisParseDB(name string) int {
	if len(name) < 3 || name[:2] != "db" {
		return 0
	}
	n, err := strconv.Atoi(name[2:])
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// execRedis 执行 Redis 变更工单：硬保底 → 连接 → 执行 → 回滚。
//
// 与 Mongo 同属命令式引擎：前镜像抓取与回滚在 internal/rediscmd 里完成，
// 结果映射回 ExecReply，与 SQL / Mongo 同一形状。
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
