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

// 连接参数与主程序 src/model/redis.go 一致（云厂商文档推荐值）。
const (
	redisDialTimeout  = 2 * time.Second
	redisIOTimeout    = 2 * time.Second
	redisPoolTimeout  = 3 * time.Second
	redisPoolSize     = 200
	redisMaxIdleConns = 200
	redisMaxRetries   = 3
	redisMaxRedirects = 3
	redisConnMaxIdle  = 30 * time.Minute
)

// redisOptions 按 DataSource 构造客户端选项。口令为空表示未开认证（Redis 允许无密码）。
func redisOptions(ds *api.DataSource) *redis.Options {
	opt := &redis.Options{
		Addr:            net.JoinHostPort(ds.Ip, strconv.Itoa(int(ds.Port))),
		Password:        ds.Password,
		DB:              0,
		DialTimeout:     redisDialTimeout,
		ReadTimeout:     redisIOTimeout,
		WriteTimeout:    redisIOTimeout,
		PoolTimeout:     redisPoolTimeout,
		PoolSize:        redisPoolSize,
		MaxIdleConns:    redisMaxIdleConns,
		MaxRetries:      redisMaxRetries,
		ConnMaxIdleTime: redisConnMaxIdle,
	}
	if ds.Username != "" {
		opt.Username = ds.Username
	}
	return opt
}

// redisClusterOptions 集群选项：数据源 ip:port 只作种子节点，其余节点经 CLUSTER SLOTS 自动发现。
func redisClusterOptions(ds *api.DataSource) *redis.ClusterOptions {
	opt := &redis.ClusterOptions{
		Addrs:           []string{net.JoinHostPort(ds.Ip, strconv.Itoa(int(ds.Port)))},
		Password:        ds.Password,
		DialTimeout:     redisDialTimeout,
		ReadTimeout:     redisIOTimeout,
		WriteTimeout:    redisIOTimeout,
		PoolTimeout:     redisPoolTimeout,
		PoolSize:        redisPoolSize,
		MaxIdleConns:    redisMaxIdleConns,
		MaxRetries:      redisMaxRetries,
		ConnMaxIdleTime: redisConnMaxIdle,
		MaxRedirects:    redisMaxRedirects,
	}
	if ds.Username != "" {
		opt.Username = ds.Username
	}
	return opt
}

// connectRedisDB 建连并选中指定逻辑库（go-redis 在建连时按 Options.DB 发 SELECT）。
// 自动识别部署形态：单机 / Proxy 集群用普通 Client，Redis Cluster 用 ClusterClient
// （否则 MOVED 重定向会让命令失败）。口令是明文：主程序发起 RPC 前已解密。
func connectRedisDB(ds *api.DataSource, db int) (redis.UniversalClient, error) {
	if ds == nil || ds.Ip == "" {
		return nil, fmt.Errorf("数据源 IP 为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), redisTimeout)
	defer cancel()

	probe := redis.NewClient(redisOptions(ds))
	if err := probe.Ping(ctx).Err(); err != nil {
		_ = probe.Close()
		return nil, err
	}
	// 集群判定：代理模式（腾讯云/华为云 Proxy）也报 cluster_enabled:1，但 CLUSTER SLOTS
	// 给的是内网节点地址，切集群客户端会连不上；只有槽位地址与配置地址一致才算直连。
	if info, err := probe.ClusterInfo(ctx).Result(); err == nil && strings.Contains(info, "cluster_enabled:1") && slotHostMatch(probe, ds.Ip) {
		_ = probe.Close()
		if db > 0 {
			return nil, fmt.Errorf("Redis Cluster 只有 db0，不支持 db%d", db)
		}
		cluster := redis.NewClusterClient(redisClusterOptions(ds))
		if err := cluster.Ping(ctx).Err(); err != nil {
			_ = cluster.Close()
			return nil, err
		}
		return cluster, nil
	}
	if db <= 0 {
		return probe, nil
	}
	_ = probe.Close()

	opt := redisOptions(ds)
	opt.DB = db
	client := redis.NewClient(opt)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

// slotHostMatch CLUSTER SLOTS 的节点地址是否与配置地址同主机：
// 直连模式返回分片地址（可连），代理模式返回实例内网地址（客户端不可达）。
func slotHostMatch(client *redis.Client, host string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), redisTimeout)
	defer cancel()
	slots, err := client.ClusterSlots(ctx).Result()
	if err != nil {
		return false
	}
	for _, s := range slots {
		for _, n := range s.Nodes {
			if h, _, err := net.SplitHostPort(n.Addr); err == nil && strings.EqualFold(h, host) {
				return true
			}
		}
	}
	return false
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
