package server

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"engine/api"
	"engine/internal/rediscmd"

	"github.com/redis/go-redis/v9"
)

// redisTimeout 建立连接与探活的超时。
const redisTimeout = 15 * time.Second

// redisOptions 按 DataSource 构造客户端选项。口令为空表示未开认证（Redis 允许无密码）。
func redisOptions(ds *api.DataSource) *redis.Options {
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
func connectRedisDB(ds *api.DataSource, db int) (*redis.Client, error) {
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

// redisParseDB 把工单/查询库名（dbN）解析成库号。空名表示默认 db0；
// 非法值报错而不是静默落 db0——在错误的库上执行变更比报错严重得多。
func redisParseDB(name string) (int, error) {
	if name == "" {
		return 0, nil
	}
	if len(name) < 3 || !strings.EqualFold(name[:2], "db") {
		return 0, fmt.Errorf("非法的 Redis 库名 %q，应为 db0..dbN", name)
	}
	n, err := strconv.Atoi(name[2:])
	if err != nil || n < 0 {
		return 0, fmt.Errorf("非法的 Redis 库名 %q，应为 db0..dbN", name)
	}
	return n, nil
}

// execRedis 执行 Redis 变更工单：硬保底 → 连接 → 执行 → 回滚。
//
// 与 Mongo 同属命令式引擎：前镜像抓取与回滚在 internal/rediscmd 里完成，
// 结果映射回 ExecReply，与 SQL / Mongo 同一形状。
func (e *Engine) execRedis(ctx context.Context, req *api.ExecRequest) (*api.ExecReply, error) {
	// 执行前过硬保底（与提交时同一套）
	if _, _, err := rediscmd.Validate(req.Order.Sql); err != nil {
		return &api.ExecReply{Ok: false, Error: err.Error()}, nil
	}
	dbNo, err := redisParseDB(req.Order.DataBase)
	if err != nil {
		return &api.ExecReply{Ok: false, Error: err.Error()}, nil
	}
	client, err := connectRedisDB(req.Source, dbNo)
	if err != nil {
		return &api.ExecReply{Ok: false, Error: "连接 Redis 失败: " + err.Error()}, nil
	}
	defer func() { _ = client.Close() }()

	out, err := rediscmd.Execute(ctx, client, req.Order.Sql)
	if err != nil {
		return &api.ExecReply{Ok: false, Error: err.Error(),
			Records: []*api.Record{{Sql: req.Order.Sql, Status: "执行失败", Level: 1, Error: err.Error()}}}, nil
	}

	rec := &api.Record{Sql: req.Order.Sql, Status: "已执行", AffectRows: uint32(out.Affected)}
	return &api.ExecReply{Ok: true, Records: []*api.Record{rec}, Rollback: out.Rollback}, nil
}
