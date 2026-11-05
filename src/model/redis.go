package model

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"Yearning-go/src/lib/enc"

	"engine/api"

	"github.com/redis/go-redis/v9"
)

// DBTypeRedis 数据源类型：0 mysql（默认）、1 PostgreSQL、2 mongodb、3 redis、4 doris
const DBTypeRedis = 3

// RedisTimeout 建立连接与探活的超时
const RedisTimeout = 15 * time.Second

// 连接参数取云厂商文档推荐值（腾讯云 Redis 连接示例 / 华为云 DCS 客户端建议）。
// 客户端现在进程级复用（见 redisClientPool），空闲连接会长期留在服务端：
// 不设 MinIdleConns（按需建连），MaxIdleConns 收敛到 20，避免数据源多时堆积。
const (
	redisDialTimeout = 2 * time.Second
	// 读写分开：读放宽到 5s——大 hash 的 HGETALL、稀疏键空间的 SCAN 在 2s 下会假性超时；
	// 写命令都是单键操作，保持文档建议的 2s。
	redisReadTimeout  = 5 * time.Second
	redisWriteTimeout = 2 * time.Second
	redisPoolTimeout  = 3 * time.Second
	redisPoolSize     = 200
	redisMaxIdleConns = 20
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

// NormalizeRedisMode 归一化手动配置的部署形态；空串或 auto 表示交给自动探测。
func NormalizeRedisMode(m string) string {
	switch strings.ToLower(strings.TrimSpace(m)) {
	case RedisModeStandalone:
		return RedisModeStandalone
	case RedisModeCluster:
		return RedisModeCluster
	case RedisModeProxy:
		return RedisModeProxy
	}
	return ""
}

// RedisDeployment 返回部署形态，手动配置优先（云代理靠 CLUSTER INFO 判不准）。
func (s *CoreDataSource) RedisDeployment() string {
	if m := NormalizeRedisMode(s.RedisMode); m != "" {
		return m
	}
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
	mode := ""
	err := s.WithRedisClient(0, func(client RedisConn) error {
		// ConnectRedisWith 已经做过一次判定：命中「cluster_enabled 且槽位地址与配置一致」
		// 时才返回集群客户端。这里先看客户端类型，避免重复判断口径不一致。
		if IsRedisCluster(client) {
			mode = RedisModeCluster
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), RedisTimeout)
		defer cancel()
		info, ierr := client.ClusterInfo(ctx).Result()
		if ierr != nil {
			// 不支持 CLUSTER 命令：普通单机
			mode = RedisModeStandalone
			return nil
		}
		if RedisClusterEnabled(info) {
			// 自称集群但给的是普通客户端 = 代理转发（槽位是实例内网地址）
			mode = RedisModeProxy
			return nil
		}
		mode = RedisModeStandalone
		return nil
	})
	if err != nil {
		return ""
	}
	return mode
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

// ConnectRedisWith 用明文口令建连并 Ping 探活，同时自动识别部署形态：
// （显式探活/检测场景用，调用方负责 Close；业务读写走 WithRedisClient 借用连接）
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

// redisClientPool 进程级客户端缓存（数据源 × 逻辑库），避免每请求重新握手。
// 借用带引用计数：编辑/删除数据源时旧客户端退役，等在途借用归还后才 Close。
// 不设淘汰，条目量级几十；失控时再上 LRU。
type redisClientPool struct {
	mu      sync.Mutex
	entries map[redisClientKey]*redisPoolEntry
	retired map[*redisPoolEntry]struct{}
	dial    func(s *CoreDataSource, password string, db int) (RedisConn, error)
}

type redisClientKey struct {
	// owner 优先用 sourceId：配置变更时才能替换同一条目；
	// 没有 sourceId 的场景（如数据源检测）退回地址，避免彼此覆盖。
	owner string
	db    int
}

type redisPoolEntry struct {
	sig     string
	client  RedisConn
	refs    int
	retired bool
}

var redisClients = &redisClientPool{
	entries: map[redisClientKey]*redisPoolEntry{},
	retired: map[*redisPoolEntry]struct{}{},
	dial:    (*CoreDataSource).connectRedisDBWith,
}

// WithRedisClient 借出该数据源在 db 库上的进程级共享客户端，fn 返回后自动归还。
// fn 执行期间即使数据源被编辑或删除，连接也不会被提前关闭；
// 客户端只在 fn 内有效，不要保存到闭包外使用。
func (s *CoreDataSource) WithRedisClient(db int, fn func(RedisConn) error) error {
	if s == nil {
		return fmt.Errorf("数据源为空")
	}
	client, release, err := redisClients.acquire(s, db)
	if err != nil {
		return err
	}
	defer release()
	return fn(client)
}

func (p *redisClientPool) acquire(s *CoreDataSource, db int) (RedisConn, func(), error) {
	password := enc.Decrypt(C.General.SecretKey, s.Password)
	key := redisClientKey{owner: s.SourceId, db: db}
	if key.owner == "" {
		key.owner = net.JoinHostPort(s.IP, strconv.Itoa(s.Port))
	}
	sig := redisClientSignature(s, password, db)

	p.mu.Lock()
	if e, ok := p.entries[key]; ok && e.sig == sig {
		e.refs++
		p.mu.Unlock()
		return e.client, p.releaser(e), nil
	}
	p.mu.Unlock()

	// 建连放在锁外：网络往返最长 15s，不能挡住其它数据源。
	client, err := p.dial(s, password, db)
	if err != nil {
		return nil, nil, err
	}

	p.mu.Lock()
	var closeNow RedisConn
	if e, ok := p.entries[key]; ok {
		if e.sig == sig {
			// 并发下另一个请求先建好了：用它的，关掉自己这份。
			e.refs++
			p.mu.Unlock()
			_ = client.Close()
			return e.client, p.releaser(e), nil
		}
		delete(p.entries, key)
		closeNow = p.retireLocked(e)
	}
	e := &redisPoolEntry{sig: sig, client: client, refs: 1}
	p.entries[key] = e
	p.mu.Unlock()
	if closeNow != nil {
		_ = closeNow.Close()
	}
	return client, p.releaser(e), nil
}

// retireLocked 让旧客户端退役：仍有借用者的等归还，无人借用则立即返回待关闭。
func (p *redisClientPool) retireLocked(e *redisPoolEntry) RedisConn {
	e.retired = true
	p.retired[e] = struct{}{}
	if e.refs > 0 {
		return nil
	}
	delete(p.retired, e)
	return e.client
}

// releaser 归还一次借用；退役客户端在最后一个借用者归还时关闭。
// once 兜住重复归还，计数不会被减成负数。
func (p *redisClientPool) releaser(e *redisPoolEntry) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			p.mu.Lock()
			e.refs--
			var closeNow RedisConn
			if e.retired && e.refs <= 0 {
				delete(p.retired, e)
				closeNow = e.client
			}
			p.mu.Unlock()
			if closeNow != nil {
				_ = closeNow.Close()
			}
		})
	}
}

// drop 让某数据源的全部客户端退役（数据源删除时调用）。
func (p *redisClientPool) drop(sourceId string) {
	var stale []RedisConn
	p.mu.Lock()
	for k, e := range p.entries {
		if k.owner == sourceId {
			delete(p.entries, k)
			if c := p.retireLocked(e); c != nil {
				stale = append(stale, c)
			}
		}
	}
	p.mu.Unlock()
	for _, c := range stale {
		_ = c.Close()
	}
}

// DropRedisClients 数据源被删除后回收其进程级客户端与部署形态缓存
// （source_id 可能被新数据源复用，留着会把旧源的形态带过去）。
// 编辑无需调用：地址与口令都进了指纹，下次访问自然换新。
func DropRedisClients(sourceId string) {
	if sourceId == "" {
		return
	}
	redisClients.drop(sourceId)
	redisModeCacheMu.Lock()
	delete(redisModeCache, sourceId)
	redisModeCacheMu.Unlock()
}

// redisClientSignature 连接参数指纹：任一变化都必须换客户端，
// 否则改了口令还会拿着旧连接继续跑。口令只留哈希，不额外复制明文。
func redisClientSignature(s *CoreDataSource, password string, db int) string {
	sum := sha256.Sum256([]byte(password))
	return fmt.Sprintf("%s|%d|%s|%x|%d", s.IP, s.Port, s.Username, sum, db)
}

// connectRedisDBWith 新建客户端并选中指定逻辑库（连接池建连用，调用方负责 Close）。
// go-redis 是在连接建立时按 Options.DB 发 SELECT 的，所以换库要另建客户端，
// 不能在一个客户端上手动 SELECT（连接池里各连接的库会不一致）。
// 集群模式只有 db0，选其它库直接报错。
func (s *CoreDataSource) connectRedisDBWith(password string, db int) (RedisConn, error) {
	if db <= 0 {
		return s.ConnectRedisWith(password)
	}
	probe, err := s.ConnectRedisWith(password)
	if err != nil {
		return nil, err
	}
	if IsRedisCluster(probe) {
		_ = probe.Close()
		return nil, fmt.Errorf("Redis Cluster 只有 db0，不支持 db%d", db)
	}
	_ = probe.Close()

	client := redis.NewClient(redisUniversalOptions(s, password, db).Simple())
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
	var out []string
	err := s.WithRedisClient(0, func(client RedisConn) error {
		if IsRedisCluster(client) {
			out = []string{"db0"}
			return nil
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
				out = dbs
				return nil
			}
		}
		out = make([]string, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, "db"+strconv.Itoa(i))
		}
		return nil
	})
	return out, err
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
	var keys []string
	err := s.WithRedisClient(db, func(client RedisConn) error {
		ctx, cancel := context.WithTimeout(context.Background(), RedisTimeout)
		defer cancel()

		var cursor uint64
		// COUNT 只是「每轮工作量提示」，给大一点能显著减少稀疏键空间下的往返次数；
		// 同时限制轮数：命中率极低时不该一直扫到 ctx 超时才返回。
		const (
			scanCount = 1000
			maxRounds = 20
		)
		for round := 0; len(keys) < limit && round < maxRounds; round++ {
			batch, next, serr := client.Scan(ctx, cursor, pattern, scanCount).Result()
			if serr != nil {
				return serr
			}
			keys = append(keys, batch...)
			cursor = next
			if cursor == 0 {
				break
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
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

// RedisKeyStateLimit 单次审核最多探测的键数，防止 MGET 把探测放大成几十次往返。
const RedisKeyStateLimit = 20

// RedisKeySnapshot 探测键的类型/占用/元素数（两次 pipeline），供审核规则判定。
// 拿不到的键不进结果——规则按未知处理，探测失败不会变成误拦。
func RedisKeySnapshot(s *CoreDataSource, db int, keys []string) map[string]api.KeyState {
	if s == nil || len(keys) == 0 {
		return nil
	}
	if len(keys) > RedisKeyStateLimit {
		keys = keys[:RedisKeyStateLimit]
	}
	out := map[string]api.KeyState{}
	_ = s.WithRedisClient(db, func(client RedisConn) error {
		ctx, cancel := context.WithTimeout(context.Background(), RedisTimeout)
		defer cancel()

		pipe := client.Pipeline()
		types := make([]*redis.StatusCmd, len(keys))
		sizes := make([]*redis.Cmd, len(keys))
		for i, k := range keys {
			types[i] = pipe.Type(ctx, k)
			sizes[i] = pipe.Do(ctx, "MEMORY", "USAGE", k)
		}
		_, _ = pipe.Exec(ctx)

		for i, k := range keys {
			st := api.KeyState{}
			if t, err := types[i].Result(); err == nil {
				st.Type = t
			}
			if v, err := sizes[i].Int64(); err == nil {
				st.Size = v
			}
			if st.Type != "" && st.Type != "none" {
				out[k] = st
			}
		}

		// 第二轮：元素数（按类型选命令）
		pipe2 := client.Pipeline()
		lens := map[string]*redis.IntCmd{}
		for k, st := range out {
			switch st.Type {
			case "hash":
				lens[k] = pipe2.HLen(ctx, k)
			case "list":
				lens[k] = pipe2.LLen(ctx, k)
			case "set":
				lens[k] = pipe2.SCard(ctx, k)
			case "zset":
				lens[k] = pipe2.ZCard(ctx, k)
			case "stream":
				lens[k] = pipe2.XLen(ctx, k)
			}
		}
		if len(lens) > 0 {
			_, _ = pipe2.Exec(ctx)
		}
		for k, c := range lens {
			if v, err := c.Result(); err == nil {
				st := out[k]
				st.Len = v
				out[k] = st
			}
		}
		return nil
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

// redisVersionTTL 版本探测缓存时长：实例版本基本不动，避免每次检测都跑 INFO
const redisVersionTTL = 10 * time.Minute

type redisVersionEntry struct {
	version  string
	expireAt time.Time
}

var (
	redisVersionCache   = map[string]redisVersionEntry{}
	redisVersionCacheMu sync.Mutex
)

// RedisVersionOf 探测实例的 redis_version；空串 = 未知（审核侧不拦）。
func RedisVersionOf(s *CoreDataSource) string {
	if s == nil || s.DBType != DBTypeRedis {
		return ""
	}
	key := s.SourceId
	redisVersionCacheMu.Lock()
	if e, ok := redisVersionCache[key]; ok && time.Now().Before(e.expireAt) {
		redisVersionCacheMu.Unlock()
		return e.version
	}
	redisVersionCacheMu.Unlock()

	version := ""
	_ = s.WithRedisClient(0, func(client RedisConn) error {
		ctx, cancel := context.WithTimeout(context.Background(), RedisTimeout)
		defer cancel()
		info, err := client.Info(ctx, "server").Result()
		if err != nil {
			return err
		}
		version = redisVersionFromInfo(info)
		return nil
	})

	redisVersionCacheMu.Lock()
	redisVersionCache[key] = redisVersionEntry{version: version, expireAt: time.Now().Add(redisVersionTTL)}
	redisVersionCacheMu.Unlock()
	return version
}

// redisVersionFromInfo 从 INFO server 的文本里取 redis_version
func redisVersionFromInfo(info string) string {
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "redis_version:"); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
