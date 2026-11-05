package redischeck

import (
	"strconv"
	"strings"
)

// 基础校验与权限：命令语法、语义登记、键识别与键权限、DB 范围、连接控制。
func init() {
	Register(commandRule{
		name: "RedisCommandSyntax", desc: "检查命令、子命令、参数数量及选项组合；语法非法或参数不完整时拦截",
		cat: CategoryStructure, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.CommandSyntax },
		hit:     func(_ Config, cmd *Command) string { return syntaxErr(cmd) },
	})

	Register(commandRule{
		name: "RedisUnknownCommand", desc: "禁止尚未登记语义的命令、模块命令及厂商扩展命令",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.UnknownCommand },
		hit: func(_ Config, cmd *Command) string {
			if knownCommand(cmd.Name) {
				return ""
			}
			return "命令 " + cmd.Name + " 未登记语义：模块命令与厂商扩展命令需要先登记再执行"
		},
	})

	Register(commandRule{
		name: "RedisKeyExtraction", desc: "必须完整识别命令访问的源键和目标键；无法识别时拦截",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.KeyExtraction },
		hit: func(_ Config, cmd *Command) string {
			if cmd.KeysKnown {
				return ""
			}
			return "无法识别 " + cmd.Name + " 访问的源键与目标键：键权限无法判定，按未知即拦截处理"
		},
	})

	Register(commandRule{
		name: "RedisKeyPrefix", desc: "键名必须位于授权前缀范围内，所有源键和目标键均需检查；留空表示不检查前缀",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.KeyPrefix != "" },
		hit: func(cfg Config, cmd *Command) string {
			var bad []string
			for _, k := range cmd.Keys {
				// 与 RedisScriptTemplateScope 共用同一套前缀语义（支持多前缀与通配）
				if !keyPrefixAllowed(cfg.KeyPrefix, k) {
					bad = append(bad, k)
				}
			}
			if len(bad) == 0 {
				return ""
			}
			return "键名应以 " + cfg.KeyPrefix + " 开头：" + strings.Join(bad, "、")
		},
	})

	Register(commandRule{
		name: "RedisMaxKeyLen", desc: "键名长度上限（字节），0 表示不限制",
		cat: CategoryStructure, modes: allModes, level: LevelWarn,
		enabled: func(c Config) bool { return c.MaxKeyLen > 0 },
		hit: func(cfg Config, cmd *Command) string {
			var bad []string
			for _, k := range cmd.Keys {
				if len(k) > cfg.MaxKeyLen {
					bad = append(bad, k)
				}
			}
			if len(bad) == 0 {
				return ""
			}
			return "键名超过 " + itoa(cfg.MaxKeyLen) + " 字节：" + strings.Join(bad, "、")
		},
	})

	Register(commandRule{
		name: "RedisForbiddenKeyPattern", desc: "禁止访问匹配保护规则的键，如系统键、核心锁及内部状态键；多个规则用逗号分隔",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.ForbiddenKeyPattern != "" },
		hit: func(cfg Config, cmd *Command) string {
			patterns := splitPatterns(cfg.ForbiddenKeyPattern)
			var bad []string
			for _, k := range cmd.Keys {
				if matchAny(patterns, k) {
					bad = append(bad, k)
				}
			}
			if len(bad) == 0 {
				return ""
			}
			return "键 " + strings.Join(bad, "、") + " 命中受保护规则，不允许访问"
		},
	})

	Register(commandRule{
		name: "RedisSensitiveKeyAccess", desc: "访问凭据、Token、个人信息等敏感业务键时提示；敏感范围由业务配置",
		cat: CategoryStructure, modes: allModes, level: LevelWarn,
		enabled: func(c Config) bool { return c.SensitiveKeyAccess != "" },
		hit: func(cfg Config, cmd *Command) string {
			patterns := splitPatterns(cfg.SensitiveKeyAccess)
			var hit []string
			for _, k := range cmd.Keys {
				if matchAny(patterns, k) {
					hit = append(hit, k)
				}
			}
			if len(hit) == 0 {
				return ""
			}
			return "访问了敏感键 " + strings.Join(hit, "、") + "，请确认操作必要且已授权"
		},
	})

	Register(commandRule{
		name: "RedisDbScope", desc: "命令只能访问指定 DB 范围；超出授权范围时拦截；格式如 0-15",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.DbScope != "" },
		hit: func(cfg Config, cmd *Command) string {
			lo, hi, ok := parseDbScope(cfg.DbScope)
			if !ok {
				// 配置非法不能静默放行：范围无法确认时按「未授权」处理
				return "DB 范围配置 " + cfg.DbScope + " 无法解析（应形如 0-15），请修正后再执行"
			}
			var bad []string
			for _, db := range dbArgs(cmd) {
				if db < int64(lo) || db > int64(hi) {
					bad = append(bad, strconv.FormatInt(db, 10))
				}
			}
			if len(bad) == 0 {
				return ""
			}
			return "DB " + strings.Join(bad, "、") + " 超出授权范围 " + cfg.DbScope
		},
	})

	Register(commandRule{
		name: "RedisForbidConnectionControl", desc: "禁止提交 AUTH、HELLO 等认证或连接状态控制命令，连接状态由审核入口统一管理",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.ForbidConnectionControl },
		hit:     forbidTable(connectionCommands),
	})
}

// forbidTable 命中命令表里的命令时返回表内说明
func forbidTable(table map[string]string) func(Config, *Command) string {
	return func(_ Config, cmd *Command) string { return table[cmd.Name] }
}

// minArgs 命令的最少参数个数（含命令名）。
// 只登记常用命令：没登记的按「至少带键」判断，宁可漏报也不误报。
var minArgs = map[string]int{
	"SET": 3, "SETEX": 4, "PSETEX": 4, "SETNX": 3, "APPEND": 3, "SETRANGE": 4, "SETBIT": 4,
	"GET": 2, "GETRANGE": 4, "GETBIT": 3, "GETSET": 3, "GETDEL": 2, "GETEX": 2,
	"INCRBY": 3, "DECRBY": 3, "INCRBYFLOAT": 3,
	"EXPIRE": 3, "PEXPIRE": 3, "EXPIREAT": 3, "PEXPIREAT": 3, "PERSIST": 2,
	"MSET": 3, "MSETNX": 3, "MGET": 2, "DEL": 2, "UNLINK": 2, "EXISTS": 2, "TOUCH": 2,
	"RENAME": 3, "RENAMENX": 3, "COPY": 3, "MOVE": 3, "RESTORE": 4,
	"HGET": 2, "HMGET": 3, "HSET": 4, "HSETNX": 4, "HMSET": 4, "HDEL": 3, "HINCRBY": 4,
	"HINCRBYFLOAT": 4, "HEXISTS": 3, "HGETALL": 2, "HSCAN": 3,
	"LPUSH": 3, "RPUSH": 3, "LPUSHX": 3, "RPUSHX": 3, "LSET": 4, "LINSERT": 5,
	"LREM": 4, "LTRIM": 4, "LRANGE": 4, "LINDEX": 3, "LPOS": 3, "RPOPLPUSH": 3, "LMOVE": 5, "LMPOP": 4,
	"SADD": 3, "SREM": 3, "SMOVE": 4, "SISMEMBER": 3, "SMISMEMBER": 3, "SINTERCARD": 3,
	"ZADD": 4, "ZINCRBY": 4, "ZREM": 3, "ZSCORE": 3, "ZRANGE": 4, "ZREVRANGE": 4,
	"ZRANGEBYSCORE": 4, "ZREVRANGEBYSCORE": 4, "ZRANGESTORE": 4, "ZSCAN": 3,
	"SCAN": 2, "SSCAN": 3, "BITOP": 4, "BITCOUNT": 2, "BITPOS": 3,
	"PFADD": 3, "PFMERGE": 2, "PFCOUNT": 2,
	"XADD": 5, "XGROUP": 3, "XINFO": 2, "XRANGE": 4, "XTRIM": 4, "XACK": 4,
	"OBJECT": 2, "MEMORY": 2, "MIGRATE": 6,
	"EVAL": 3, "EVALSHA": 3, "EVAL_RO": 3, "EVALSHA_RO": 3, "FCALL": 3, "FCALL_RO": 3,
	"SELECT": 2, "CONFIG": 2, "CLIENT": 2, "ACL": 2, "CLUSTER": 2, "MODULE": 2,
	"SCRIPT": 2, "FUNCTION": 2, "SORT": 2, "SORT_RO": 2,
}

// optionConflicts 互斥选项：同时出现即语法非法
var optionConflicts = []struct {
	cmd  string
	opts []string
}{
	{"SET", []string{"NX", "XX"}},
	{"SET", []string{"EX", "PX", "KEEPTTL"}},
	{"GETEX", []string{"PERSIST", "EX", "PX"}},
	{"ZADD", []string{"NX", "XX"}},
	{"ZADD", []string{"GT", "LT"}},
	{"EXPIRE", []string{"NX", "XX"}},
	{"EXPIRE", []string{"GT", "LT"}},
}

// syntaxErr 命令语法问题：参数不足、键值不成对、互斥选项同时出现
func syntaxErr(cmd *Command) string {
	if !cmd.KeysKnown {
		// 语义未登记的命令交给 RedisUnknownCommand
		return ""
	}
	if min := minArgs[cmd.Name]; min > 0 && cmd.ArgCount() < min {
		return cmd.Name + " 参数不完整：至少需要 " + itoa(min-1) + " 个参数，当前只有 " + itoa(cmd.ArgCount()-1) + " 个"
	}
	switch cmd.Name {
	case "MSET", "MSETNX":
		if n := cmd.ArgCount() - 1; n > 0 && n%2 != 0 {
			return cmd.Name + " 的键值必须成对出现，当前有 " + itoa(n) + " 个参数"
		}
	case "HSET", "HMSET":
		if n := cmd.ArgCount() - 2; n > 0 && n%2 != 0 {
			return cmd.Name + " 的字段与值必须成对出现，当前有 " + itoa(n) + " 个参数"
		}
	}
	for _, c := range optionConflicts {
		if !strings.EqualFold(c.cmd, cmd.Name) {
			continue
		}
		var hit []string
		for _, opt := range c.opts {
			if cmd.HasAnyArg(opt) {
				hit = append(hit, opt)
			}
		}
		if len(hit) > 1 {
			return cmd.Name + " 的选项 " + strings.Join(hit, " / ") + " 互斥，不能同时使用"
		}
	}
	return ""
}

// parseDbScope 解析 "0-15" / "5" 形式的 DB 范围
func parseDbScope(s string) (int, int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, false
	}
	if i := strings.IndexByte(s, '-'); i > 0 {
		lo, err1 := strconv.Atoi(strings.TrimSpace(s[:i]))
		hi, err2 := strconv.Atoi(strings.TrimSpace(s[i+1:]))
		if err1 != nil || err2 != nil || lo > hi {
			return 0, 0, false
		}
		return lo, hi, true
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, 0, false
	}
	return n, n, true
}

// dbArgs 命令里出现的逻辑库编号
func dbArgs(cmd *Command) []int64 {
	switch cmd.Name {
	case "SELECT":
		if n, ok := cmd.IntArg(1); ok {
			return []int64{n}
		}
	case "MOVE":
		if n, ok := cmd.IntArg(2); ok {
			return []int64{n}
		}
	case "SWAPDB":
		var out []int64
		for _, i := range []int{1, 2} {
			if n, ok := cmd.IntArg(i); ok {
				out = append(out, n)
			}
		}
		return out
	case "MIGRATE":
		if n, ok := cmd.IntArg(4); ok {
			return []int64{n}
		}
	}
	return nil
}
