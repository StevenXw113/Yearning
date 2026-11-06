package api

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// RedisScope 命令语义：SLOWLOG 与 GET 都只读，但风险差一个量级，授权要按档位。
type RedisScope int

const (
	// ScopeData 业务数据：GET / HGETALL / LRANGE / SCAN …
	ScopeData RedisScope = iota
	// ScopeMeta 键与实例元信息：TYPE / TTL / EXISTS / DBSIZE / PING …
	ScopeMeta
	// ScopeDiag 实例诊断：SLOWLOG / LATENCY / DUMP / COMMAND / INFO 全量。
	// 默认不放行，需要数据源级开关。
	ScopeDiag
	// ScopeWrite 变更：SET / DEL / HSET …
	ScopeWrite
	// ScopeAdmin 管理：CONFIG / FLUSHALL / EVAL …（未登记命令也落在这里）
	ScopeAdmin
)

// 业务数据（只读）
var dataCommands = map[string]struct{}{
	// string
	"GET": {}, "MGET": {}, "STRLEN": {}, "GETRANGE": {}, "SUBSTR": {},
	// hash
	"HGET": {}, "HMGET": {}, "HGETALL": {}, "HKEYS": {}, "HVALS": {}, "HLEN": {}, "HEXISTS": {}, "HSCAN": {}, "HRANDFIELD": {},
	// list
	"LRANGE": {}, "LLEN": {}, "LINDEX": {}, "LPOS": {},
	// set
	"SMEMBERS": {}, "SCARD": {}, "SISMEMBER": {}, "SMISMEMBER": {}, "SRANDMEMBER": {}, "SSCAN": {}, "SDIFF": {}, "SINTER": {}, "SUNION": {}, "SINTERCARD": {},
	// zset
	"ZRANGE": {}, "ZRANGEBYSCORE": {}, "ZREVRANGE": {}, "ZREVRANGEBYSCORE": {}, "ZCARD": {}, "ZSCORE": {}, "ZMSCORE": {},
	"ZRANK": {}, "ZREVRANK": {}, "ZCOUNT": {}, "ZSCAN": {}, "ZRANGEBYLEX": {}, "ZREVRANGEBYLEX": {}, "ZRANDMEMBER": {},
	// 位图 / HyperLogLog / 流（读取）
	"GETBIT": {}, "BITCOUNT": {}, "BITPOS": {}, "PFCOUNT": {},
	"XLEN": {}, "XRANGE": {}, "XREVRANGE": {}, "XREAD": {}, "XINFO": {},
	// 地理
	"GEOPOS": {}, "GEODIST": {}, "GEOHASH": {}, "GEOSEARCH": {},
	// 键遍历（返回的是键名）
	"SCAN": {},
}

// 键与实例元信息（只读、不含业务值）
var metaCommands = map[string]struct{}{
	"EXISTS": {}, "TYPE": {}, "TTL": {}, "PTTL": {}, "RANDOMKEY": {}, "TOUCH": {},
	"OBJECT": {}, "MEMORY": {}, "DBSIZE": {}, "PING": {}, "ECHO": {}, "TIME": {}, "LOLWUT": {},
}

// 实例诊断（默认不放行）
var diagCommands = map[string]struct{}{
	"SLOWLOG": {}, "LATENCY": {}, "DUMP": {}, "COMMAND": {}, "INFO": {},
}

// 变更类：改数据（审核分类判 DML、抓前镜像、回滚都依赖它）
var writeCommands = map[string]struct{}{
	"SET": {}, "SETEX": {}, "PSETEX": {}, "SETNX": {}, "MSET": {}, "MSETNX": {}, "APPEND": {}, "SETRANGE": {},
	"INCR": {}, "DECR": {}, "INCRBY": {}, "DECRBY": {}, "INCRBYFLOAT": {}, "GETSET": {}, "GETDEL": {}, "GETEX": {},
	"DEL": {}, "UNLINK": {}, "EXPIRE": {}, "PEXPIRE": {}, "EXPIREAT": {}, "PEXPIREAT": {}, "PERSIST": {},
	"RENAME": {}, "RENAMENX": {}, "MOVE": {}, "COPY": {}, "RESTORE": {},
	"HSET": {}, "HSETNX": {}, "HMSET": {}, "HDEL": {}, "HINCRBY": {}, "HINCRBYFLOAT": {},
	"LPUSH": {}, "RPUSH": {}, "LPUSHX": {}, "RPUSHX": {}, "LPOP": {}, "RPOP": {}, "LSET": {}, "LINSERT": {},
	"LREM": {}, "LTRIM": {}, "RPOPLPUSH": {}, "LMOVE": {}, "LMPOP": {},
	// 阻塞式弹出：会改数据，抓前镜像与分类都要算写
	"BLPOP": {}, "BRPOP": {}, "BLMOVE": {}, "BRPOPLPUSH": {}, "BLMPOP": {},
	"BZPOPMIN": {}, "BZPOPMAX": {}, "BZMPOP": {},
	"SADD": {}, "SREM": {}, "SPOP": {}, "SMOVE": {}, "SINTERSTORE": {}, "SUNIONSTORE": {}, "SDIFFSTORE": {},
	"ZADD": {}, "ZREM": {}, "ZINCRBY": {}, "ZREMRANGEBYRANK": {}, "ZREMRANGEBYSCORE": {}, "ZREMRANGEBYLEX": {},
	"ZPOPMIN": {}, "ZPOPMAX": {}, "ZMPOP": {}, "ZUNIONSTORE": {}, "ZINTERSTORE": {}, "ZDIFFSTORE": {}, "ZRANGESTORE": {},
	"SETBIT": {}, "BITOP": {}, "BITFIELD": {}, "PFADD": {}, "PFMERGE": {},
	"XADD": {}, "XDEL": {}, "XTRIM": {}, "XACK": {}, "XGROUP": {}, "XCLAIM": {}, "XAUTOCLAIM": {}, "XSETID": {},
	"GEOADD": {}, "GEORADIUS": {}, "GEORADIUSBYMEMBER": {}, "GEOSEARCHSTORE": {},
}

// RedisIsWriteCommand 命令是否改数据
func RedisIsWriteCommand(name string) bool {
	return RedisSemantics(name) == ScopeWrite
}

// dataCommands / metaCommands / diagCommands 是白名单，未登记的一律按 ScopeAdmin 处理：
// Redis 命令两百多个还在增加，漏一个就等于查询页能执行写命令。
var readCommands = buildReadCommands()

func buildReadCommands() map[string]struct{} {
	out := make(map[string]struct{}, len(dataCommands)+len(metaCommands)+len(diagCommands))
	for _, table := range []map[string]struct{}{dataCommands, metaCommands, diagCommands} {
		for name := range table {
			out[name] = struct{}{}
		}
	}
	return out
}

// RedisSemantics 返回命令语义（命令名大小写不敏感）。
func RedisSemantics(name string) RedisScope {
	n := strings.ToUpper(strings.TrimSpace(name))
	switch {
	case inScope(dataCommands, n):
		return ScopeData
	case inScope(metaCommands, n):
		return ScopeMeta
	case inScope(diagCommands, n):
		return ScopeDiag
	case inScope(writeCommands, n):
		return ScopeWrite
	}
	return ScopeAdmin
}

func inScope(table map[string]struct{}, name string) bool {
	_, ok := table[name]
	return ok
}

// RedisIsReadCommand 命令是否只读（数据 / 元信息 / 诊断都算只读）。
// 引擎的规则分类用它：分类看语义，不看执行授权。
func RedisIsReadCommand(name string) bool {
	return RedisSemantics(name) <= ScopeDiag
}

// RedisReadCommandNames 只读命令名单（已排序），供护栏与文档使用。
func RedisReadCommandNames() []string {
	out := make([]string, 0, len(readCommands))
	for name := range readCommands {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// infoSectionsAllowed INFO 里不含配置与其它连接信息的只读 section。
// INFO 不带参数会返回 server/config/clients 等全量内容，因此不能整条放行。
var infoSectionsAllowed = map[string]struct{}{
	"KEYSPACE": {}, "REPLICATION": {}, "MEMORY": {},
}

// RedisPolicy 查询页的执行策略。
type RedisPolicy struct {
	// AllowDiag 数据源级：是否允许诊断命令（SLOWLOG / LATENCY / DUMP / COMMAND / INFO 全量）
	AllowDiag bool
}

// QueryAllowed 判定查询页能否执行；与 RedisIsReadCommand 分工：一个管语义（审核分类），
// 一个管授权（执行）。reason 直接给用户看。
func (p RedisPolicy) QueryAllowed(args []string) (bool, string) {
	if len(args) == 0 {
		return false, "Redis 命令不能为空"
	}
	name := strings.ToUpper(strings.TrimSpace(args[0]))
	switch RedisSemantics(name) {
	case ScopeData, ScopeMeta:
		return true, ""
	case ScopeDiag:
		if name == "INFO" && infoOnlySafe(args) {
			return true, ""
		}
		if p.AllowDiag {
			return true, ""
		}
		return false, name + " 属于实例诊断命令，能看到配置或其它业务的执行信息；" +
			"默认关闭，需要时在数据源上开启「允许诊断命令」"
	}
	return false, "查询页只能执行只读命令（GET / SCAN / HGETALL / LRANGE 等），变更请到工单申请页提交"
}

// infoOnlySafe INFO 的 section 全部属于只读安全集合时才放行
func infoOnlySafe(args []string) bool {
	if len(args) < 2 {
		return false // 不带 section = 全量信息
	}
	for _, a := range args[1:] {
		if _, ok := infoSectionsAllowed[strings.ToUpper(strings.TrimSpace(a))]; !ok {
			return false
		}
	}
	return true
}

// ParseRedisDB 把 dbN 解析成库号；空名 = db0，非法值报错。引擎与主程序共用。
func ParseRedisDB(name string) (int, error) {
	s := strings.TrimSpace(name)
	if s == "" {
		return 0, nil
	}
	if len(s) < 3 || !strings.EqualFold(s[:2], "db") {
		return 0, fmt.Errorf("非法的 Redis 库名 %q，应为 db0..dbN", name)
	}
	n, err := strconv.Atoi(s[2:])
	if err != nil || n < 0 {
		return 0, fmt.Errorf("非法的 Redis 库名 %q，应为 db0..dbN", name)
	}
	return n, nil
}

// Split 按 redis-cli 的参数规则切词：空白分隔，双引号内解析 \n \xHH 等转义，
// 单引号内只识别 \' 与 \\。与主程序 src/lib/rediscmd.Split 的实现保持一致。
func SplitRedisCommand(line string) ([]string, error) {
	var (
		args  []string
		cur   strings.Builder
		quote byte
		has   bool
	)
	flush := func() {
		if has {
			args = append(args, cur.String())
			cur.Reset()
			has = false
		}
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote == 0 && (c == ' ' || c == '\t' || c == '\r' || c == '\n'):
			flush()
		case quote == 0 && (c == '"' || c == '\''):
			quote = c
			has = true
		case quote != 0 && c == quote:
			quote = 0
		case quote == '"' && c == '\\' && i+1 < len(line):
			i++
			switch line[i] {
			case 'n':
				cur.WriteByte('\n')
			case 'r':
				cur.WriteByte('\r')
			case 't':
				cur.WriteByte('\t')
			case 'b':
				cur.WriteByte('\b')
			case 'a':
				cur.WriteByte('\a')
			case 'x':
				if i+2 < len(line) {
					var v int
					if _, err := fmt.Sscanf(line[i+1:i+3], "%02x", &v); err == nil {
						cur.WriteByte(byte(v))
						i += 2
						break
					}
				}
				cur.WriteByte('x')
			default:
				cur.WriteByte(line[i])
			}
		case quote == '\'' && c == '\\' && i+1 < len(line) && (line[i+1] == '\'' || line[i+1] == '\\'):
			i++
			cur.WriteByte(line[i])
		default:
			cur.WriteByte(c)
			has = true
		}
	}
	if quote != 0 {
		return nil, errors.New("命令里的引号没有闭合")
	}
	flush()
	return args, nil
}
