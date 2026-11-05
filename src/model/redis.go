package model

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
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
const (
	redisDialTimeout = 2 * time.Second
	// 读写分开：读放宽到 5s——大 hash 的 HGETALL、稀疏键空间的 SCAN 在 2s 下会假性超时；
	// 写命令都是单键操作，保持文档建议的 2s。
	redisReadTimeout  = 5 * time.Second
	redisWriteTimeout = 2 * time.Second
	redisPoolTimeout  = 3 * time.Second
	redisPoolSize     = 200
	redisMaxIdleConns = 200
	redisMaxRetries   = 3
	redisMaxRedirects = 3
	redisConnMaxIdle  = 30 * time.Minute
)

// RedisDefaultDatabases Redis 默认的逻辑库数量（redis.conf 的 databases，默认 16）
const RedisDefaultDatabases = 16

// Redis 部署形态：审核期判定 Cluster/Proxy 规则用；空串表示探测失败（未知）。
const (
	RedisModeStandalone = "standalone"
	RedisModeCluster    = "cluster"
	RedisModeProxy      = "proxy"
)

// redisModeTTL 部署形态缓存时长：形态基本不变，避免每次检测/查询都重跑 CLUSTER INFO。
const redisModeTTL = 10 * time.Minute

type redisModeEntry struct {
	mode     string
	expireAt time.Time
}

var (
	redisModeCache   = map[string]redisModeEntry{}
	redisModeCacheMu sync.Mutex
)

// RedisDeployment 返回部署形态（standalone / cluster / proxy），探测失败返回空串表示未知。
// 判定口径与 ConnectRedisWith 一致：cluster_enabled:1 且槽位地址与配置地址同主机才算直连集群；
// 代理同样报 cluster_enabled:1，但槽位是实例内网地址。
func (s *CoreDataSource) RedisDeployment() string {
	key := s.SourceId
	redisModeCacheMu.Lock()
	if e, ok := redisModeCache[key]; ok && time.Now().Before(e.expireAt) {
		redisModeCacheMu.Unlock()
		return e.mode
	}
	redisModeCacheMu.Unlock()

	mode := s.detectRedisMode()
	redisModeCacheMu.Lock()
	redisModeCache[key] = redisModeEntry{mode: mode, expireAt: time.Now().Add(redisModeTTL)}
	redisModeCacheMu.Unlock()
	return mode
}

// RedisDeploymentOf 取部署形态，非 Redis 数据源直接返回空串（不做无谓探测）。
func RedisDeploymentOf(s *CoreDataSource) string {
	if s == nil || s.DBType != DBTypeRedis {
		return ""
	}
	return s.RedisDeployment()
}

func (s *CoreDataSource) detectRedisMode() string {
	client, err := s.ConnectRedis()
	if err != nil {
		return ""
	}
	defer func() { _ = client.Close() }()

	// ConnectRedisWith 已经做过一次判定：命中「cluster_enabled 且槽位地址与配置一致」
	// 时才返回集群客户端。这里先看客户端类型，避免重复判断口径不一致。
	if IsRedisCluster(client) {
		return RedisModeCluster
	}
	ctx, cancel := context.WithTimeout(context.Background(), RedisTimeout)
	defer cancel()
	info, ierr := client.ClusterInfo(ctx).Result()
	if ierr != nil {
		// 不支持 CLUSTER 命令：普通单机
		return RedisModeStandalone
	}
	if RedisClusterEnabled(info) {
		// 自称集群但给的是普通客户端 = 代理转发（槽位是实例内网地址）
		return RedisModeProxy
	}
	return RedisModeStandalone
}

// RedisScanLimit 列键名时单次抽样的上限。
// 查询页只需展示一批键名，取够就停，避免在大实例上长时间游标遍历。
const RedisScanLimit = 200

// RedisConn 统一单机与集群客户端：*redis.Client、*redis.ClusterClient 都实现该接口
type RedisConn = redis.UniversalClient

// redisUniversalOptions 单机/代理与集群共用的连接选项：
// go-redis 的 UniversalOptions 能同时构造 *redis.Client 与 *redis.ClusterClient，
// 省掉 Options / ClusterOptions 两份逐字段抄写。
// 口令为空表示实例未开认证——redis 允许无密码，这里不做非空校验（Mongo 不同）；
// Username 留空即旧版「只有密码」的 ACL 形态。
func redisUniversalOptions(s *CoreDataSource, password string, db int) *redis.UniversalOptions {
	return &redis.UniversalOptions{
		Addrs:           []string{net.JoinHostPort(s.IP, strconv.Itoa(s.Port))},
		Username:        s.Username,
		Password:        password,
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

// RedisOptions 单机 / 云厂商 Proxy 的客户端选项
func RedisOptions(s *CoreDataSource, password string) *redis.Options {
	return redisUniversalOptions(s, password, 0).Simple()
}

// RedisClusterOptions 集群选项：数据源的 ip:port 只作种子节点，
// go-redis 会经 CLUSTER SLOTS 自动发现其余节点。
func RedisClusterOptions(s *CoreDataSource, password string) *redis.ClusterOptions {
	return redisUniversalOptions(s, password, 0).Cluster()
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
	if info, err := client.ClusterInfo(ctx).Result(); err == nil && RedisClusterEnabled(info) {
		slots, serr := client.ClusterSlots(ctx).Result()
		if serr == nil && slotsMatchHost(slots, s.IP) {
			_ = client.Close()
			cluster := redis.NewClusterClient(RedisClusterOptions(s, password))
			if err := cluster.Ping(ctx).Err(); err != nil {
				_ = cluster.Close()
				return nil, err
			}
			return cluster, nil
		}
	}
	return client, nil
}

// slotsMatchHost 槽位节点里是否有与 host 同主机的地址（直连返回分片地址，代理返回内网地址）
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

	client := redis.NewClient(redisUniversalOptions(s, enc.Decrypt(C.General.SecretKey, s.Password), db).Simple())
	ctx, cancel := context.WithTimeout(context.Background(), RedisTimeout)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

// RedisDatabases 返回可选的逻辑库名（db0..dbN），供查询页的树使用。
// 集群只有 db0；单机/代理以 CONFIG GET databases 为准（实例配了多少就列多少，空库也要能选），
// 托管 Redis 常禁用 CONFIG，读不到时退回 INFO keyspace / 默认 16 个。
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
	if m, cerr := client.ConfigGet(ctx, "databases").Result(); cerr == nil {
		if raw, ok := m["databases"]; ok {
			if parsed, perr := strconv.Atoi(raw); perr == nil && parsed > 0 {
				n = parsed
			}
		}
	} else if info, ierr := client.Info(ctx, "keyspace").Result(); ierr == nil {
		// CONFIG 被禁用时只能看 keyspace：此时列出有键的库，好过给 16 个空库
		if dbs := keyspaceDatabases(info); len(dbs) > 0 {
			return dbs, nil
		}
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, "db"+strconv.Itoa(i))
	}
	return out, nil
}

// keyspaceDatabases 解析 INFO keyspace 里的库名（db0:keys=3,expires=0,avg_ttl=0），按库号排序
func keyspaceDatabases(info string) []string {
	var out []string
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		name, _, ok := strings.Cut(line, ":")
		if !ok || !strings.HasPrefix(name, "db") {
			continue
		}
		if _, err := strconv.Atoi(name[2:]); err != nil {
			continue
		}
		out = append(out, name)
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := strconv.Atoi(out[i][2:])
		b, _ := strconv.Atoi(out[j][2:])
		return a < b
	})
	return out
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
	// COUNT 只是「每轮工作量提示」，给大一点能显著减少稀疏键空间下的往返次数；
	// 同时限制轮数：命中率极低时不该一直扫到 ctx 超时才返回。
	const (
		scanCount = 1000
		maxRounds = 20
	)
	for round := 0; len(keys) < limit && round < maxRounds; round++ {
		batch, next, serr := client.Scan(ctx, cursor, pattern, scanCount).Result()
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

// RedisParseDB 把查询页传来的库名（dbN）解析成库号。
// 空名表示默认 db0；非法值报错——静默回落 db0 会让命令跑到错误的库上。
// 库名只作为选择器使用，不参与任何命令拼接。
func RedisParseDB(name string) (int, error) {
	s := strings.TrimSpace(name)
	if s == "" {
		return 0, nil
	}
	if len(s) < 3 || !strings.EqualFold(s[:2], "db") {
		return 0, fmt.Errorf("非法逻辑库名 %q，应形如 db0", name)
	}
	n, err := strconv.Atoi(s[2:])
	if err != nil || n < 0 {
		return 0, fmt.Errorf("非法逻辑库名 %q，应形如 db0", name)
	}
	return n, nil
}
