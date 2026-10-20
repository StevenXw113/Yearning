package redischeck

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// 变更类型，取值与工单 Type 对齐（主程序侧 vars.DDL=0 / vars.DML=1）
const (
	KindQuery = -1 // 只读命令，不走工单
	KindDDL   = 0  // 管理/结构类
	KindDML   = 1  // 改数据
)

// 命令表。
//
// engine 是独立 Go module，不能复用主程序 src/lib/rediscmd 里的表，所以维护一份。
// 两处语义必须一致：主程序那份是**执行底线**（不受配置影响），
// 这份是**可按规则集开关**的审核，改任何一边都要同步另一边。
var (
	// readCommands 只读命令白名单：不在表里的一律按变更处理。
	// 用白名单而非黑名单——Redis 命令两百多个还在增加，漏一个就等于查询页能写数据。
	readCommands = map[string]struct{}{
		"GET": {}, "MGET": {}, "STRLEN": {}, "GETRANGE": {}, "SUBSTR": {},
		"EXISTS": {}, "TYPE": {}, "TTL": {}, "PTTL": {}, "SCAN": {}, "RANDOMKEY": {},
		"DUMP": {}, "OBJECT": {}, "MEMORY": {},
		"HGET": {}, "HMGET": {}, "HGETALL": {}, "HKEYS": {}, "HVALS": {}, "HLEN": {}, "HEXISTS": {}, "HSCAN": {}, "HRANDFIELD": {},
		"LRANGE": {}, "LLEN": {}, "LINDEX": {}, "LPOS": {},
		"SMEMBERS": {}, "SCARD": {}, "SISMEMBER": {}, "SMISMEMBER": {}, "SRANDMEMBER": {}, "SSCAN": {}, "SDIFF": {}, "SINTER": {}, "SUNION": {},
		"ZRANGE": {}, "ZRANGEBYSCORE": {}, "ZREVRANGE": {}, "ZREVRANGEBYSCORE": {}, "ZCARD": {}, "ZSCORE": {}, "ZMSCORE": {},
		"ZRANK": {}, "ZREVRANK": {}, "ZCOUNT": {}, "ZSCAN": {}, "ZRANGEBYLEX": {}, "ZREVRANGEBYLEX": {},
		"GETBIT": {}, "BITCOUNT": {}, "BITPOS": {}, "PFCOUNT": {},
		"XLEN": {}, "XRANGE": {}, "XREVRANGE": {}, "XINFO": {},
		"GEOPOS": {}, "GEODIST": {}, "GEOHASH": {}, "GEOSEARCH": {},
		"INFO": {}, "DBSIZE": {}, "PING": {}, "ECHO": {}, "TIME": {}, "COMMAND": {}, "SLOWLOG": {}, "LATENCY": {},
	}

	// writeCommands 改数据的命令（分类用；安全校验不依赖它）
	writeCommands = map[string]struct{}{
		"SET": {}, "SETEX": {}, "PSETEX": {}, "SETNX": {}, "MSET": {}, "MSETNX": {}, "APPEND": {}, "SETRANGE": {},
		"INCR": {}, "DECR": {}, "INCRBY": {}, "DECRBY": {}, "INCRBYFLOAT": {}, "GETSET": {}, "GETDEL": {}, "GETEX": {},
		"DEL": {}, "UNLINK": {}, "EXPIRE": {}, "PEXPIRE": {}, "EXPIREAT": {}, "PEXPIREAT": {}, "PERSIST": {},
		"RENAME": {}, "RENAMENX": {}, "MOVE": {}, "COPY": {}, "RESTORE": {},
		"HSET": {}, "HSETNX": {}, "HMSET": {}, "HDEL": {}, "HINCRBY": {}, "HINCRBYFLOAT": {},
		"LPUSH": {}, "RPUSH": {}, "LPUSHX": {}, "RPUSHX": {}, "LPOP": {}, "RPOP": {}, "LSET": {}, "LINSERT": {},
		"LREM": {}, "LTRIM": {}, "RPOPLPUSH": {}, "LMOVE": {},
		"SADD": {}, "SREM": {}, "SPOP": {}, "SMOVE": {}, "SINTERSTORE": {}, "SUNIONSTORE": {}, "SDIFFSTORE": {},
		"ZADD": {}, "ZREM": {}, "ZINCRBY": {}, "ZREMRANGEBYRANK": {}, "ZREMRANGEBYSCORE": {}, "ZREMRANGEBYLEX": {},
		"ZPOPMIN": {}, "ZPOPMAX": {}, "ZUNIONSTORE": {}, "ZINTERSTORE": {}, "ZDIFFSTORE": {}, "ZRANGESTORE": {},
		"SETBIT": {}, "BITOP": {}, "BITFIELD": {}, "PFADD": {}, "PFMERGE": {},
		"XADD": {}, "XDEL": {}, "XTRIM": {}, "XACK": {}, "XGROUP": {}, "XCLAIM": {}, "XSETID": {},
		"GEOADD": {}, "GEORADIUS": {}, "GEORADIUSBYMEMBER": {}, "GEOSEARCHSTORE": {},
	}

	// dangerousCommands 影响面是整个实例的命令（RedisForbidDangerous）
	dangerousCommands = map[string]string{
		"FLUSHALL": "禁止 FLUSHALL（清空所有库）",
		"FLUSHDB":  "禁止 FLUSHDB（清空当前库）",
		"SHUTDOWN": "禁止 SHUTDOWN（关闭实例）",
		"SWAPDB":   "禁止 SWAPDB（交换两个库的数据）",
		"MIGRATE":  "禁止 MIGRATE（跨实例搬键）",
	}

	// adminCommands 运维管理类（RedisForbidAdmin）
	adminCommands = map[string]string{
		"CONFIG":       "禁止 CONFIG（修改实例配置）",
		"DEBUG":        "禁止 DEBUG（调试命令可造成阻塞或崩溃）",
		"MODULE":       "禁止 MODULE（加载/卸载模块）",
		"MONITOR":      "禁止 MONITOR（实时打印所有命令，泄露数据且影响性能）",
		"CLIENT":       "禁止 CLIENT（管理连接，例如 CLIENT KILL）",
		"BGSAVE":       "禁止 BGSAVE（触发落盘）",
		"BGREWRITEAOF": "禁止 BGREWRITEAOF（触发全量重写）",
		"SAVE":         "禁止 SAVE（阻塞式落盘）",
	}

	// scriptCommands 服务端脚本（RedisForbidScript）
	scriptCommands = map[string]string{
		"EVAL":     "禁止 EVAL（服务端执行 Lua 脚本）",
		"EVALSHA":  "禁止 EVALSHA（服务端执行 Lua 脚本）",
		"SCRIPT":   "禁止 SCRIPT（管理服务端脚本）",
		"FUNCTION": "禁止 FUNCTION（管理服务端函数）",
		"FCALL":    "禁止 FCALL（调用服务端函数）",
	}

	// replicaCommands 主从关系（RedisForbidReplica）
	replicaCommands = map[string]string{
		"REPLICAOF": "禁止 REPLICAOF（改变主从关系）",
		"SLAVEOF":   "禁止 SLAVEOF（改变主从关系，旧名）",
	}

	// blockingCommands 会长时间占用连接的阻塞命令（RedisForbidBlocking）
	blockingCommands = map[string]string{
		"BLPOP":      "禁止 BLPOP（阻塞式弹出，会长时间占用连接）",
		"BRPOP":      "禁止 BRPOP（阻塞式弹出，会长时间占用连接）",
		"BLMOVE":     "禁止 BLMOVE（阻塞式移动）",
		"BRPOPLPUSH": "禁止 BRPOPLPUSH（阻塞式移动）",
		"BLMPOP":     "禁止 BLMPOP（阻塞式弹出）",
		"BZPOPMIN":   "禁止 BZPOPMIN（阻塞式弹出）",
		"BZPOPMAX":   "禁止 BZPOPMAX（阻塞式弹出）",
		"WAIT":       "禁止 WAIT（等待副本确认，会阻塞）",
		"WAITAOF":    "禁止 WAITAOF（等待持久化确认，会阻塞）",
		"SUBSCRIBE":  "禁止 SUBSCRIBE（订阅会占用连接，且可能长期不返回）",
		"PSUBSCRIBE": "禁止 PSUBSCRIBE（订阅会占用连接，且可能长期不返回）",
	}
)

// Command 一条解析后的 Redis 命令
type Command struct {
	// Name 命令名（首参数，大写归一）
	Name string
	// Args 全部参数（含命令名）
	Args []string
	// Kind 变更类型（KindQuery / KindDDL / KindDML）
	Kind int
	// Keys 受影响的键（写命令；只读命令为空）
	Keys []string
}

// Parse 解析一行命令
func Parse(line string) (*Command, error) {
	args, err := Split(line)
	if err != nil {
		return nil, err
	}
	if len(args) == 0 {
		return nil, errors.New("Redis 命令不能为空")
	}
	name := strings.ToUpper(args[0])
	return &Command{
		Name: name,
		Args: args,
		Kind: classify(name),
		Keys: affectedKeys(args),
	}, nil
}

// Split 按 redis-cli 的参数规则切词：空白分隔，双引号内解析 \n \xHH 等转义，
// 单引号内只识别 \' 与 \\。与主程序 src/lib/rediscmd.Split 的实现保持一致。
func Split(line string) ([]string, error) {
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

// classify 分类：危险/管理/脚本/主从都按结构变更处理，
// 否则它们会落进「不是变更命令」那条分支——既拦不住也说不清理由。
func classify(name string) int {
	if _, ok := writeCommands[name]; ok {
		return KindDML
	}
	if _, ok := readCommands[name]; ok {
		return KindQuery
	}
	if _, ok := dangerousCommands[name]; ok {
		return KindDDL
	}
	if _, ok := adminCommands[name]; ok {
		return KindDDL
	}
	if _, ok := scriptCommands[name]; ok {
		return KindDDL
	}
	if _, ok := replicaCommands[name]; ok {
		return KindDDL
	}
	if _, ok := blockingCommands[name]; ok {
		return KindDDL
	}
	// 未知命令按变更处理：宁可走审批，也别把写命令当查询放行
	return KindDDL
}

// affectedKeys 取命令会改动的键（用于 MaxAffectKeys 规则）
func affectedKeys(args []string) []string {
	if len(args) < 2 {
		return nil
	}
	name := strings.ToUpper(args[0])
	dedup := func(in []string) []string {
		seen := map[string]struct{}{}
		out := make([]string, 0, len(in))
		for _, s := range in {
			if s == "" {
				continue
			}
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
		return out
	}
	switch name {
	case "MSET", "MSETNX":
		var out []string
		for i := 1; i < len(args); i += 2 {
			out = append(out, args[i])
		}
		return dedup(out)
	case "DEL", "UNLINK", "TOUCH", "MGET", "PFMERGE", "WATCH", "EXISTS":
		return dedup(args[1:])
	case "BITOP":
		if len(args) > 2 {
			return dedup(args[2:])
		}
		return nil
	case "RENAME", "RENAMENX", "COPY", "SMOVE", "LMOVE", "BLMOVE", "RPOPLPUSH", "BRPOPLPUSH":
		if len(args) > 2 {
			return dedup(args[1:3])
		}
		return dedup(args[1:])
	}
	if _, ok := writeCommands[name]; ok {
		return dedup(args[1:2])
	}
	// GETDEL/GETEX 这类读改混合的命令
	if name == "GETDEL" || name == "GETEX" || name == "GETSET" {
		return dedup(args[1:2])
	}
	return nil
}

// ---------- 规则用的取值helper ----------

// Arg 取第 i 个参数（含命令名，i 从 0 起）；越界返回空串
func (c *Command) Arg(i int) string {
	if i < 0 || i >= len(c.Args) {
		return ""
	}
	return c.Args[i]
}

// ArgsFrom 取第 i 个参数起的全部参数
func (c *Command) ArgsFrom(i int) []string {
	if i < 0 || i >= len(c.Args) {
		return nil
	}
	return c.Args[i:]
}

// ArgCount 参数个数（含命令名）
func (c *Command) ArgCount() int { return len(c.Args) }

// IntArg 把第 i 个参数当整数取；不是整数返回 0、false
func (c *Command) IntArg(i int) (int64, bool) {
	s := c.Arg(i)
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// HasAnyArg 参数里是否出现某个选项（大小写不敏感），
// 用于判断 SET 是否带了 EX / PX / EXAT / KEEPTTL 这类过期选项。
func (c *Command) HasAnyArg(names ...string) bool {
	for _, a := range c.Args {
		for _, n := range names {
			if strings.EqualFold(a, n) {
				return true
			}
		}
	}
	return false
}

// KeyCount 受影响的键数
func (c *Command) KeyCount() int { return len(c.Keys) }
