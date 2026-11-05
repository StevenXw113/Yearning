package redischeck

import (
	"strconv"
	"strings"

	"engine/api"
)

// noKeyCommands 不访问键的命令：参数里出现的不是键（游标 / 频道 / 子命令 / 配置项）。
var noKeyCommands = map[string]struct{}{
	"PING": {}, "ECHO": {}, "TIME": {}, "COMMAND": {}, "SLOWLOG": {}, "LATENCY": {},
	"INFO": {}, "DBSIZE": {}, "SCAN": {}, "RANDOMKEY": {}, "LASTSAVE": {},
	"SELECT": {}, "AUTH": {}, "HELLO": {}, "QUIT": {}, "RESET": {},
	"MULTI": {}, "EXEC": {}, "DISCARD": {}, "UNWATCH": {},
	"PUBLISH": {}, "SPUBLISH": {},
	"SCRIPT": {}, "FUNCTION": {},
	"CONFIG": {}, "CLIENT": {}, "ACL": {}, "CLUSTER": {}, "MODULE": {},
	"DEBUG": {}, "MONITOR": {}, "SAVE": {}, "BGSAVE": {}, "BGREWRITEAOF": {},
	"FLUSHALL": {}, "FLUSHDB": {}, "SWAPDB": {}, "SHUTDOWN": {},
	"REPLICAOF": {}, "SLAVEOF": {}, "WAIT": {}, "WAITAOF": {},
	"SUBSCRIBE": {}, "PSUBSCRIBE": {}, "SSUBSCRIBE": {},
	"UNSUBSCRIBE": {}, "PUNSUBSCRIBE": {}, "SUNSUBSCRIBE": {},
}

// commandKeys 取命令访问的源键与目标键。
//
// 键权限类规则（前缀 / 保护键 / 敏感键 / DB 范围）都以它为准：
// 源键和目标键都要检查，例如 RENAME src dst、COPY src dst、MSET k1 v1 k2 v2。
//
// 第三个返回 false 表示命令未登记语义，无法判断它访问了哪些键；
// RedisUnknownCommand 与 RedisKeyExtraction 分别负责拦下这种情况。
func commandKeys(args []string) (src, dst []string, ok bool) {
	if len(args) == 0 {
		return nil, nil, true
	}
	name := strings.ToUpper(args[0])
	if _, none := noKeyCommands[name]; none {
		return nil, nil, true
	}
	switch name {
	case "MSET", "MSETNX":
		var keys []string
		for i := 1; i < len(args); i += 2 {
			keys = append(keys, args[i])
		}
		return keys, nil, true
	case "DEL", "UNLINK", "TOUCH", "EXISTS", "MGET", "PFCOUNT", "WATCH", "SDIFF", "SINTER", "SUNION":
		return args[1:], nil, true
	case "PFMERGE":
		if len(args) > 2 {
			return args[2:], args[1:2], true
		}
		return args[1:], nil, true
	case "BITOP":
		if len(args) > 3 {
			return args[3:], args[2:3], true
		}
		return nil, nil, true
	case "RENAME", "RENAMENX", "COPY", "SMOVE", "LMOVE", "RPOPLPUSH", "BRPOPLPUSH", "BLMOVE", "ZRANGESTORE", "GEOSEARCHSTORE":
		if len(args) > 2 {
			return args[1:2], args[2:3], true
		}
		return args[1:], nil, true
	case "SINTERCARD", "LMPOP", "ZMPOP":
		// SINTERCARD numkeys key [key ...] / LMPOP numkeys key [key ...] LEFT
		n, err := strconv.Atoi(argAt(args, 1))
		if err != nil || n < 0 || len(args) < 2+n {
			return nil, nil, false
		}
		return args[2 : 2+n], nil, true
	case "SINTERSTORE", "SUNIONSTORE", "SDIFFSTORE":
		if len(args) > 2 {
			return args[2:], args[1:2], true
		}
		return nil, args[1:], true
	case "ZUNIONSTORE", "ZINTERSTORE", "ZDIFFSTORE":
		// ZUNIONSTORE dst numkeys key [key ...]
		n, err := strconv.Atoi(argAt(args, 2))
		if err != nil || n < 0 || len(args) < 3+n {
			return nil, args[1:2], true
		}
		return args[3 : 3+n], args[1:2], true
	case "SORT", "SORT_RO", "GEORADIUS", "GEORADIUSBYMEMBER":
		return args[1:2], storeTarget(args), true
	case "BLPOP", "BRPOP", "BZPOPMIN", "BZPOPMAX":
		// 最后一个参数是超时秒数，其余都是键
		if len(args) > 2 {
			return args[1 : len(args)-1], nil, true
		}
		return args[1:], nil, true
	case "EVAL", "EVALSHA", "EVAL_RO", "EVALSHA_RO", "FCALL", "FCALL_RO":
		// EVAL script numkeys key [key ...] arg [arg ...]
		n, err := strconv.Atoi(argAt(args, 2))
		if err != nil || n < 0 || len(args) < 3+n {
			return nil, nil, false
		}
		return args[3 : 3+n], nil, true
	case "OBJECT", "MEMORY":
		// OBJECT ENCODING key / MEMORY USAGE key
		switch strings.ToUpper(argAt(args, 1)) {
		case "ENCODING", "FREQ", "IDLETIME", "REFCOUNT", "USAGE":
			// 缺键（只写了 OBJECT ENCODING）时不能直接切，args 长度为 2 会越界 panic；
			// 按「键识别失败」处理，交给 Syntax / KeyExtraction 规则拦。
			if len(args) > 2 {
				return args[2:3], nil, true
			}
			return nil, nil, false
		}
		return nil, nil, true
	case "XGROUP", "XINFO":
		// XGROUP CREATE key group ... / XINFO STREAM key
		if len(args) > 2 {
			return args[2:3], nil, true
		}
		return nil, nil, true
	case "XREAD", "XREADGROUP":
		// ... STREAMS key [key ...] id [id ...]
		for i := 1; i < len(args); i++ {
			if strings.EqualFold(args[i], "STREAMS") {
				n := (len(args) - i - 1) / 2
				if n <= 0 {
					return nil, nil, true
				}
				return args[i+1 : i+1+n], nil, true
			}
		}
		return nil, nil, true
	case "MIGRATE":
		// MIGRATE host port key db timeout ... [KEYS key ...]
		if len(args) > 3 && args[3] != "" {
			return args[3:4], nil, true
		}
		for i := 4; i < len(args); i++ {
			if strings.EqualFold(args[i], "KEYS") {
				return args[i+1:], nil, true
			}
		}
		return nil, nil, false
	}
	if _, ok := writeCommands[name]; ok {
		return args[1:2], nil, true
	}
	if api.RedisIsReadCommand(name) {
		return args[1:2], nil, true
	}
	return nil, nil, false
}

// allKeys 源键 + 目标键（去重）：键规范类规则用
func allKeys(src, dst []string) []string {
	seen := make(map[string]struct{}, len(src)+len(dst))
	out := make([]string, 0, len(src)+len(dst))
	for _, group := range [][]string{src, dst} {
		for _, k := range group {
			if k == "" {
				continue
			}
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, k)
		}
	}
	return out
}

// storeTarget 取 STORE / STOREDIST 后面的目标键
func storeTarget(args []string) []string {
	for i := 2; i+1 < len(args); i++ {
		switch strings.ToUpper(args[i]) {
		case "STORE", "STOREDIST":
			return args[i+1 : i+2]
		}
	}
	return nil
}

// argAt 越界返回空串
func argAt(args []string, i int) string {
	if i < 0 || i >= len(args) {
		return ""
	}
	return args[i]
}

// splitPatterns 拆分配置里的多个匹配规则：逗号、分号、空白都算分隔符
func splitPatterns(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// globMatch 简化的通配匹配：* 任意串、? 单字符，其余按字面量（大小写敏感）。
func globMatch(pattern, s string) bool {
	var (
		pi, si int
		star   = -1
		match  int
	)
	for si < len(s) {
		switch {
		case pi < len(pattern) && (pattern[pi] == '?' || pattern[pi] == s[si]):
			pi++
			si++
		case pi < len(pattern) && pattern[pi] == '*':
			star = pi
			match = si
			pi++
		case star >= 0:
			pi = star + 1
			match++
			si = match
		default:
			return false
		}
	}
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern)
}

// matchAny 是否命中任意一条匹配规则
func matchAny(patterns []string, key string) bool {
	for _, p := range patterns {
		if globMatch(p, key) {
			return true
		}
	}
	return false
}

// keyPrefixAllowed 键是否落在允许的前缀范围内。
//
// 配置支持逗号/空白分隔的多个前缀；含 * 或 ? 的按通配匹配，其余按字面前缀。
// RedisKeyPrefix 与 RedisScriptTemplateScope 共用这套语义——两处各写一套
// 会让同一份配置在两条规则下得出相反结论。
func keyPrefixAllowed(raw, key string) bool {
	prefixes := splitPatterns(raw)
	if len(prefixes) == 0 {
		return true
	}
	for _, p := range prefixes {
		if strings.ContainsAny(p, "*?") {
			if globMatch(p, key) {
				return true
			}
			continue
		}
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	return false
}
