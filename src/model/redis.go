package model

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"Yearning-go/src/lib/enc"

	"github.com/redis/go-redis/v9"
)

// DBTypeRedis 数据源类型：0 mysql（默认）、1 PostgreSQL、2 mongodb、3 redis、4 doris
const DBTypeRedis = 3

// RedisTimeout 建立连接与探活的超时
const RedisTimeout = 15 * time.Second

// 连接参数取云厂商文档推荐值（腾讯云 Redis 连接示例 / 华为云 DCS 客户端建议）。
// 不设 MinIdleConns：客户端按会话/工单建、用完即关，预建空闲连接只会堆在服务端。
// ponytail: 读写 2s 是文档值，大 key 假性超时就把 redisIOTimeout 调大。
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

// RedisDefaultDatabases Redis 默认的逻辑库数量（redis.conf 的 databases，默认 16）
const RedisDefaultDatabases = 16

// RedisScanLimit 列键名时单次抽样的上限。
// 查询页只需展示一批键名，取够就停，避免在大实例上长时间游标遍历。
const RedisScanLimit = 200

// RedisConn 统一单机与集群客户端：*redis.Client、*redis.ClusterClient 都实现该接口
type RedisConn = redis.UniversalClient

// RedisOptions 按数据源配置构造客户端选项。
// 口令为空表示实例未开认证——redis 允许无密码，这里不做非空校验（Mongo 不同）。
// 认证支持 Redis 6+ 的 ACL 用户名：留空即旧版的「只有密码」。
func RedisOptions(s *CoreDataSource, password string) *redis.Options {
	opt := &redis.Options{
		Addr:            net.JoinHostPort(s.IP, strconv.Itoa(s.Port)),
		Password:        password,
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
	if s.Username != "" {
		opt.Username = s.Username
	}
	return opt
}

// RedisClusterOptions 集群选项：数据源的 ip:port 只作种子节点，
// go-redis 会经 CLUSTER SLOTS 自动发现其余节点。
func RedisClusterOptions(s *CoreDataSource, password string) *redis.ClusterOptions {
	opt := &redis.ClusterOptions{
		Addrs:           []string{net.JoinHostPort(s.IP, strconv.Itoa(s.Port))},
		Password:        password,
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
	if s.Username != "" {
		opt.Username = s.Username
	}
	return opt
}

// RedisClusterEnabled 解析 CLUSTER INFO：集群模式返回 cluster_enabled:1。
func RedisClusterEnabled(info string) bool {
	return strings.Contains(info, "cluster_enabled:1")
}

// IsRedisCluster 判断客户端是否为集群客户端。
func IsRedisCluster(c RedisConn) bool {
	_, ok := c.(*redis.ClusterClient)
	return ok
}

// ConnectRedis 按数据源配置建立客户端（口令列先解密再建连）。
// Redis 不经 gorm，调用方负责 Close。
func (s *CoreDataSource) ConnectRedis() (RedisConn, error) {
	return s.ConnectRedisWith(enc.Decrypt(C.General.SecretKey, s.Password))
}

// ConnectRedisWith 用明文口令建连并 Ping 探活，同时自动识别部署形态：
//   - 单机 / Proxy 集群（Twemproxy、Codis、云厂商代理）：对客户端就一个普通入口，用 Client；
//   - Redis Cluster：CLUSTER INFO 返回 cluster_enabled:1，改用 ClusterClient（自带 MOVED/ASK 重定向）。
//
// 数据源管理页的「检测」已经把口令解好，这里再解一次会变成空口令——与 ConnectMongoWith 同理。
func (s *CoreDataSource) ConnectRedisWith(password string) (RedisConn, error) {
	client := redis.NewClient(RedisOptions(s, password))
	ctx, cancel := context.WithTimeout(context.Background(), RedisTimeout)
	defer cancel()
	// 连接是懒建立的，Ping 一次才能把「地址/口令不对」在入口处暴露出来
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	// 探测是否直连集群：代理模式（腾讯云/华为云 Proxy）同样报 cluster_enabled:1，
	// 但 CLUSTER SLOTS 给的是内网节点地址，客户端连不上；只有槽位地址与配置地址一致才切集群客户端。
	if info, err := client.ClusterInfo(ctx).Result(); err == nil && RedisClusterEnabled(info) && slotHostMatch(client, s.IP) {
		_ = client.Close()
		cluster := redis.NewClusterClient(RedisClusterOptions(s, password))
		if err := cluster.Ping(ctx).Err(); err != nil {
			_ = cluster.Close()
			return nil, err
		}
		return cluster, nil
	}
	return client, nil
}

// slotHostMatch CLUSTER SLOTS 的节点地址是否与配置地址同主机：
// 直连模式返回分片地址（可连），代理模式返回实例内网地址（客户端不可达）。
func slotHostMatch(client *redis.Client, host string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), RedisTimeout)
	defer cancel()
	slots, err := client.ClusterSlots(ctx).Result()
	return err == nil && slotsMatchHost(slots, host)
}

// slotsMatchHost 槽位节点里是否有与 host 同主机的地址
func slotsMatchHost(slots []redis.ClusterSlot, host string) bool {
	for _, s := range slots {
		for _, n := range s.Nodes {
			if h, _, err := net.SplitHostPort(n.Addr); err == nil && strings.EqualFold(h, host) {
				return true
			}
		}
	}
	return false
}

// ConnectRedisDB 建连并选中指定逻辑库（工单/查询页按 dbN 指定目标库时用）。
// go-redis 是在连接建立时按 Options.DB 发 SELECT 的，所以换库要另建客户端，
// 不能在一个客户端上手动 SELECT（连接池里各连接的库会不一致）。
// 集群模式只有 db0，选其它库直接报错。
func (s *CoreDataSource) ConnectRedisDB(db int) (RedisConn, error) {
	if db <= 0 {
		return s.ConnectRedis()
	}
	probe, err := s.ConnectRedis()
	if err != nil {
		return nil, err
	}
	if IsRedisCluster(probe) {
		_ = probe.Close()
		return nil, fmt.Errorf("Redis Cluster 只有 db0，不支持 db%d", db)
	}
	_ = probe.Close()

	opt := RedisOptions(s, enc.Decrypt(C.General.SecretKey, s.Password))
	opt.DB = db
	client := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), RedisTimeout)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

// RedisDatabases 返回可选的逻辑库名（db0..dbN），供查询页的树使用。
// 集群只有 db0；单机/代理以 CONFIG GET databases 为准，托管 Redis 常禁用 CONFIG，读不到就按默认 16 个。
func (s *CoreDataSource) RedisDatabases() ([]string, error) {
	client, err := s.ConnectRedis()
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()

	if IsRedisCluster(client) {
		return []string{"db0"}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), RedisTimeout)
	defer cancel()

	n := RedisDefaultDatabases
	// CONFIG 可能被托管服务禁用，失败不当错误、退回默认值
	if m, cerr := client.ConfigGet(ctx, "databases").Result(); cerr == nil {
		if raw, ok := m["databases"]; ok {
			if parsed, perr := strconv.Atoi(raw); perr == nil && parsed > 0 {
				n = parsed
			}
		}
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, "db"+strconv.Itoa(i))
	}
	return out, nil
}

// RedisScanKeys 用 SCAN 抽样键名。
//
// 刻意不用 KEYS：它会一次性遍历整个键空间并在大实例上阻塞服务端，
// 这是 Redis 最常见的生产事故来源之一（也是审核规则里要拦的对象）。
func (s *CoreDataSource) RedisScanKeys(db int, pattern string, limit int) ([]string, error) {
	if pattern == "" {
		pattern = "*"
	}
	if limit <= 0 || limit > RedisScanLimit {
		limit = RedisScanLimit
	}
	client, err := s.ConnectRedisDB(db)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), RedisTimeout)
	defer cancel()

	var (
		cursor uint64
		keys   []string
	)
	for len(keys) < limit {
		batch, next, serr := client.Scan(ctx, cursor, pattern, int64(limit)).Result()
		if serr != nil {
			return nil, serr
		}
		keys = append(keys, batch...)
		cursor = next
		if cursor == 0 {
			break
		}
	}
	if len(keys) > limit {
		keys = keys[:limit]
	}
	sort.Strings(keys)
	return keys, nil
}

// RedisParseDB 把查询页传来的库名（dbN）解析成库号，非法值返回 0。
// 库名只作为选择器使用，不参与任何命令拼接。
func RedisParseDB(name string) int {
	if len(name) < 3 || name[:2] != "db" {
		return 0
	}
	n, err := strconv.Atoi(name[2:])
	if err != nil || n < 0 {
		return 0
	}
	return n
}
