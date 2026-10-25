package model

import (
	"context"
	"net"
	"sort"
	"strconv"
	"time"

	"Yearning-go/src/lib/enc"

	"github.com/redis/go-redis/v9"
)

// DBTypeRedis 数据源类型：0 mysql（默认）、1 PostgreSQL（未实现）、2 mongodb、3 redis
const DBTypeRedis = 3

// RedisTimeout 建立连接与探活的超时
const RedisTimeout = 15 * time.Second

// RedisDefaultDatabases Redis 默认的逻辑库数量（redis.conf 的 databases，默认 16）
const RedisDefaultDatabases = 16

// RedisScanLimit 列键名时单次抽样的上限。
// 查询页只需要展示一批键名，取够就停，避免在大实例上长时间游标遍历。
const RedisScanLimit = 200

// RedisOptions 按数据源配置构造客户端选项。
// 口令为空表示实例未开认证——Redis 允许无密码，这里不做非空校验（Mongo 不同）。
// 认证支持 Redis 6+ 的 ACL 用户名：留空即旧版的「只有密码」。
func RedisOptions(s *CoreDataSource, password string) *redis.Options {
	opt := &redis.Options{
		Addr:         net.JoinHostPort(s.IP, strconv.Itoa(s.Port)),
		Password:     password,
		DB:           0,
		DialTimeout:  RedisTimeout,
		ReadTimeout:  RedisTimeout,
		WriteTimeout: RedisTimeout,
	}
	if s.Username != "" {
		opt.Username = s.Username
	}
	return opt
}

// ConnectRedis 按数据源配置建立客户端（口令列先解密再建连）。
// Redis 不经 gorm，调用方负责 Close。
func (s *CoreDataSource) ConnectRedis() (*redis.Client, error) {
	return s.ConnectRedisWith(enc.Decrypt(C.General.SecretKey, s.Password))
}

// ConnectRedisWith 用明文口令建连并 Ping 探活。数据源管理页的「检测」已经把口令解好，
// 这里再解一次会变成空口令——与 ConnectMongoWith 同理。
func (s *CoreDataSource) ConnectRedisWith(password string) (*redis.Client, error) {
	client := redis.NewClient(RedisOptions(s, password))
	ctx, cancel := context.WithTimeout(context.Background(), RedisTimeout)
	defer cancel()
	// 连接是懒建立的，Ping 一次才能把「地址/口令不对」在入口处暴露出来
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

// ConnectRedisDB 建连并选中指定逻辑库（工单/查询页按 dbN 指定目标库时用）。
// go-redis 是在连接建立时按 Options.DB 发 SELECT 的，所以换库要另建客户端，
// 不能在一个客户端上手动 SELECT（连接池里各连接的库会不一致）。
func (s *CoreDataSource) ConnectRedisDB(db int) (*redis.Client, error) {
	if db <= 0 {
		return s.ConnectRedis()
	}
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
// 库数量以 CONFIG GET databases 为准；托管 Redis 常禁用 CONFIG，读不到就按默认 16 个。
// 每个库都会实际建连探活，连不上直接报错（与 Mongo 列库的行为一致）。
func (s *CoreDataSource) RedisDatabases() ([]string, error) {
	client, err := s.ConnectRedis()
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()

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
