// Package rediscmd 提供 Redis 命令的切词、分类与安全校验。
//
// 与 SQL / MongoDB 的差别：Redis 命令是**文本**（`SET key val EX 60`），
// 既没有语法树也没有 JSON 结构可依附。这里退一步，按 RESP 的参数规则切词，
// 再用「命令名 + 参数形态」做判定——够挡住最常见也最危险的操作
// （清库、服务端脚本、阻塞式全键扫描、运维管理命令）。
//
// 与 mongodb 包一致：本包只做识别与校验，不执行、不依赖驱动。
package rediscmd

import (
	"errors"
	"fmt"
	"strings"
)

// 变更类型，取值与工单 Type 对齐（vars.DDL=0 / vars.DML=1）
const (
	KindQuery = -1 // 只读命令，走查询页
	KindDDL   = 0  // 管理/结构类（CONFIG、CLIENT、CREATE 等）
	KindDML   = 1  // 改数据
)

// dangerousCommands 一律拒绝：这些命令的影响面是整个实例，不该由一条工单承担。
var dangerousCommands = map[string]string{
	"FLUSHALL":  "禁止 FLUSHALL（清空所有库）",
	"FLUSHDB":   "禁止 FLUSHDB（清空当前库）",
	"SHUTDOWN":  "禁止 SHUTDOWN（关闭实例）",
	"SWAPDB":    "禁止 SWAPDB（交换两个库的数据）",
	"MIGRATE":   "禁止 MIGRATE（跨实例搬键）",
	"REPLICAOF": "禁止 REPLICAOF（改变主从关系）",
	"SLAVEOF":   "禁止 SLAVEOF（改变主从关系，旧名）",
}

// adminCommands 运维管理类：会改变实例行为或泄露数据，同样不开放。
var adminCommands = map[string]string{
	"CONFIG":       "禁止 CONFIG（修改实例配置）",
	"DEBUG":        "禁止 DEBUG（调试命令可造成阻塞或崩溃）",
	"MODULE":       "禁止 MODULE（加载/卸载模块）",
	"MONITOR":      "禁止 MONITOR（实时打印所有命令，泄露数据且影响性能）",
	"CLIENT":       "禁止 CLIENT（管理连接，例如 CLIENT KILL）",
	"BGREWRITEAOF": "禁止 BGREWRITEAOF（触发全量重写）",
	"BGSAVE":       "禁止 BGSAVE（触发落盘）",
	"SAVE":         "禁止 SAVE（阻塞式落盘）",
}

// scriptCommands 服务端脚本：与 Mongo 的 $where / eval 同类，一律拦。
var scriptCommands = map[string]string{
	"EVAL":     "禁止 EVAL（服务端执行 Lua 脚本）",
	"EVALSHA":  "禁止 EVALSHA（服务端执行 Lua 脚本）",
	"SCRIPT":   "禁止 SCRIPT（管理服务端脚本）",
	"FUNCTION": "禁止 FUNCTION（管理服务端函数）",
	"FCALL":    "禁止 FCALL（调用服务端函数）",
}

// readCommands 只读命令白名单。
//
// 刻意用白名单而不是黑名单：写命令种类繁多且会持续新增，
// 漏一个就意味着「查询页可以执行写命令」。不在表里的命令一律按变更处理。
var readCommands = map[string]struct{}{
	// string
	"GET": {}, "MGET": {}, "STRLEN": {}, "GETRANGE": {}, "SUBSTR": {},
	// 通用
	"EXISTS": {}, "TYPE": {}, "TTL": {}, "PTTL": {}, "SCAN": {}, "RANDOMKEY": {},
	"DUMP": {}, "TOUCH": {}, "OBJECT": {}, "MEMORY": {},
	// hash
	"HGET": {}, "HMGET": {}, "HGETALL": {}, "HKEYS": {}, "HVALS": {}, "HLEN": {}, "HEXISTS": {}, "HSCAN": {}, "HRANDFIELD": {},
	// list
	"LRANGE": {}, "LLEN": {}, "LINDEX": {}, "LPOS": {},
	// set
	"SMEMBERS": {}, "SCARD": {}, "SISMEMBER": {}, "SMISMEMBER": {}, "SRANDMEMBER": {}, "SSCAN": {}, "SDIFF": {}, "SINTER": {}, "SUNION": {},
	// zset
	"ZRANGE": {}, "ZRANGEBYSCORE": {}, "ZREVRANGE": {}, "ZREVRANGEBYSCORE": {}, "ZCARD": {}, "ZSCORE": {}, "ZMSCORE": {},
	"ZRANK": {}, "ZREVRANK": {}, "ZCOUNT": {}, "ZSCAN": {}, "ZRANGEBYLEX": {}, "ZREVRANGEBYLEX": {},
	// 位图 / HyperLogLog / 流（读取）
	"GETBIT": {}, "BITCOUNT": {}, "BITPOS": {}, "PFCOUNT": {},
	"XLEN": {}, "XRANGE": {}, "XREVRANGE": {}, "XREAD": {}, "XINFO": {},
	// 地理
	"GEOPOS": {}, "GEODIST": {}, "GEOHASH": {}, "GEOSEARCH": {},
	// 服务端信息（只读）
	"INFO": {}, "DBSIZE": {}, "PING": {}, "ECHO": {}, "TIME": {}, "LOLWUT": {},
	"COMMAND": {}, "SLOWLOG": {}, "LATENCY": {},
}

// Split 按 redis-cli 的参数规则把一行命令切成参数。
//
// 规则：参数以空白分隔；双引号内的内容原样保留并解析 \n \r \t \b \a \xHH 等转义；
// 单引号内只识别 \' 与 \\（与 redis-cli 一致，其余反斜杠原样保留）。
func Split(line string) ([]string, error) {
	var (
		args  []string
		cur   strings.Builder
		quote byte // 0 = 不在引号内
		has   bool // 当前参数是否已开始（用于区分空参数 "" 与无参数）
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
				// \xHH：读两位十六进制，非法则按字面保留
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

// CommandName 取命令名（首参数，大写）。Redis 命令名大小写不敏感，
// 首参数即命令名——这与 Mongo 用「JSON 首个键」识别命令是同一个思路。
func CommandName(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return strings.ToUpper(args[0])
}

// Classify 判断命令类别：白名单里的只读命令算查询，其余算变更。
func Classify(name string) int {
	if _, ok := readCommands[name]; ok {
		return KindQuery
	}
	if isWriteCommand(name) {
		return KindDML
	}
	// 既不在只读白名单、也不在写命令表里：当作变更处理（宁可要审批，也别放行）
	return KindDDL
}

// Validate 把一行命令切成参数并做硬保底校验。
//
// 这一层**不读规则集**：它是不受配置影响的执行底线（与 Mongo 的 Validate、
// 抓前镜像的条数上限同类）。可配置的审核（开关 + error/warn/observe 级别）
// 由引擎侧的 internal/redischeck 完成，两边判定语义保持一致。
func Validate(line string) (int, []string, error) {
	args, err := Split(line)
	if err != nil {
		return KindQuery, nil, err
	}
	name := CommandName(args)
	if name == "" {
		return KindQuery, nil, errors.New("Redis 命令不能为空")
	}
	if msg, bad := dangerousCommands[name]; bad {
		return KindDDL, args, errors.New(msg)
	}
	if msg, bad := adminCommands[name]; bad {
		return KindDDL, args, errors.New(msg)
	}
	if msg, bad := scriptCommands[name]; bad {
		return KindDDL, args, errors.New(msg)
	}
	if name == "KEYS" {
		// KEYS 是 O(N) 且阻塞单线程实例，大键空间下等同于故障
		return KindQuery, args, errors.New("禁止 KEYS：它会遍历整个键空间并阻塞实例，请改用 SCAN 游标遍历")
	}
	return Classify(name), args, nil
}

// isWriteCommand 判断是否明确的写命令。
// 只为分类用（区分 DML 与 DDL），安全校验不依赖它。
func isWriteCommand(name string) bool {
	switch name {
	case "SET", "SETEX", "PSETEX", "SETNX", "MSET", "MSETNX", "APPEND", "SETRANGE", "INCR", "DECR", "INCRBY", "DECRBY", "INCRBYFLOAT",
		"GETSET", "GETDEL", "GETEX",
		"DEL", "UNLINK", "EXPIRE", "PEXPIRE", "EXPIREAT", "PEXPIREAT", "PERSIST", "RENAME", "RENAMENX", "MOVE", "COPY", "RESTORE",
		"HSET", "HSETNX", "HMSET", "HDEL", "HINCRBY", "HINCRBYFLOAT",
		"LPUSH", "RPUSH", "LPUSHX", "RPUSHX", "LPOP", "RPOP", "LSET", "LINSERT", "LREM", "LTRIM", "RPOPLPUSH", "LMOVE", "BLPOP", "BRPOP", "BLMOVE",
		"SADD", "SREM", "SPOP", "SMOVE", "SINTERSTORE", "SUNIONSTORE", "SDIFFSTORE",
		"ZADD", "ZREM", "ZINCRBY", "ZREMRANGEBYRANK", "ZREMRANGEBYSCORE", "ZREMRANGEBYLEX", "ZPOPMIN", "ZPOPMAX",
		"ZUNIONSTORE", "ZINTERSTORE", "ZDIFFSTORE", "ZRANGESTORE",
		"SETBIT", "BITOP", "BITFIELD", "PFADD", "PFMERGE",
		"XADD", "XDEL", "XTRIM", "XACK", "XGROUP", "XCLAIM", "XSETID",
		"GEOADD", "GEORADIUS", "GEORADIUSBYMEMBER", "GEOSEARCHSTORE":
		return true
	}
	return false
}
