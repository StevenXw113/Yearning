package server

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"engine/api"
	"engine/internal/rediscmd"

	"github.com/redis/go-redis/v9"
)

// redisTimeout 建立连接与探活的超时。
const redisTimeout = 15 * time.Second

// 连接参数与主程序 src/model/redis.go 一致（云厂商文档推荐值）；
// 客户端进程级复用后空闲连接会长期留着，MaxIdleConns 同样收敛到 20。
const (
	redisDialTimeout = 2 * time.Second
	// 读放大到 5s（大 key 读取），写保持 2s，与主程序 src/model/redis.go 一致
	redisReadTimeout  = 5 * time.Second
	redisWriteTimeout = 2 * time.Second
	redisPoolTimeout  = 3 * time.Second
	redisPoolSize     = 200
	redisMaxIdleConns = 20
	redisMaxRetries   = 3
	redisMaxRedirects = 3
	redisConnMaxIdle  = 30 * time.Minute
)

// redisUniversalOptions 单机/代理与集群共用一份选项（UniversalOptions 能构造两种客户端）。
// 口令为空表示未开认证（Redis 允许无密码）。
func redisUniversalOptions(ds *api.DataSource, db int) *redis.UniversalOptions {
	return &redis.UniversalOptions{
		Addrs:           []string{net.JoinHostPort(ds.Ip, strconv.Itoa(int(ds.Port)))},
		Username:        ds.Username,
		Password:        ds.Password,
		DB:              db,
		DialTimeout:     redisDialTimeout,
		ReadTimeout:     redisReadTimeout,
		WriteTimeout:    redisWriteTimeout,
		PoolTimeout:     redisPoolTimeout,
		PoolSize:        redisPoolSize,
		MaxIdleConns:    redisMaxIdleConns,
		MaxRetries:      redisMaxRetries,
		ConnMaxIdleTime: redisConnMaxIdle,
		MaxRedirects:    redisMaxRedirects,
	}
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

	probe := redis.NewClient(redisUniversalOptions(ds, 0).Simple())
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
		cluster := redis.NewClusterClient(redisUniversalOptions(ds, 0).Cluster())
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

	client := redis.NewClient(redisUniversalOptions(ds, db).Simple())
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

// redisClients 进程级客户端缓存，键 = 连接参数（含库号）。
// Exec 每单新建即弃会让同一数据源反复支付 TCP+auth 握手（一次执行最多两次：
// 探活 + 换库），与 dbpool.go 的 SQL 连接池同因同治。
//
// ponytail: 不做淘汰。条目 = 数据源×库，量级几十；口令变更走新指纹自然分流，
// 旧条目只剩池对象本身，空闲连接由 ConnMaxIdleTime 收掉。
var redisClients sync.Map

// redisClient 取（或建）进程级共享客户端；调用方不要 Close。
func redisClient(ds *api.DataSource, db int) (redis.UniversalClient, error) {
	if ds == nil {
		return nil, fmt.Errorf("数据源为空")
	}
	key := redisClientKey(ds, db)
	if v, ok := redisClients.Load(key); ok {
		return v.(redis.UniversalClient), nil
	}
	client, err := connectRedisDB(ds, db)
	if err != nil {
		return nil, err
	}
	actual, loaded := redisClients.LoadOrStore(key, client)
	if loaded {
		// 并发下另一个执行先建好了：用它的，关掉自己这份
		_ = client.Close()
		return actual.(redis.UniversalClient), nil
	}
	return client, nil
}

func redisClientKey(ds *api.DataSource, db int) string {
	sum := sha256.Sum256([]byte(ds.Password))
	return fmt.Sprintf("%s|%d|%s|%x|%d", ds.Ip, ds.Port, ds.Username, sum, db)
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
func redisParseDB(name string) (int, error) { return api.ParseRedisDB(name) }

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
	client, err := redisClient(req.Source, dbNo)
	if err != nil {
		return &api.ExecReply{Ok: false, Error: "连接 Redis 失败: " + err.Error()}, nil
	}

	out, err := rediscmd.Execute(ctx, client, req.Order.Sql)
	if err != nil {
		return &api.ExecReply{Ok: false, Error: err.Error(),
			Records: []*api.Record{{Sql: req.Order.Sql, Status: "执行失败", Level: 1, Error: err.Error()}}}, nil
	}

	rec := &api.Record{Sql: req.Order.Sql, Status: "已执行", AffectRows: uint32(out.Affected)}
	return &api.ExecReply{Ok: true, Records: []*api.Record{rec}, Rollback: out.Rollback}, nil
}
