package redischeck

import (
	"strings"
	"time"
)

// TTL 与缓存规范：新建必须有过期时间、覆盖写入不得静默丢 TTL、
// 过期时长上下限，以及「立即过期等同于删除」。
func init() {
	Register(commandRule{
		name: "RedisRequireTtl", desc: "缓存类键新建后必须有过期时间；明确登记为永久数据的业务键可豁免",
		cat: CategorySecurity, modes: writeOnly, level: LevelError,
		enabled: func(c Config) bool { return c.RequireTTL },
		hit: func(_ Config, cmd *Command) string {
			switch {
			case cmd.Name == "SET":
				if cmd.HasAnyArg("EX", "PX", "EXAT", "PXAT", "KEEPTTL") {
					return ""
				}
				return "SET 未设置过期时间，键会常驻内存；如确需永久保存请登记豁免或在规则集中关闭该规则"
			case inNameSet(requireTtlCommands, cmd.Name):
				// 这些命令本身不支持过期参数，新建键只能靠后续 EXPIRE
				return cmd.Name + " 会创建键但不带过期时间；缓存类数据请在同一流程里补 EXPIRE/PEXPIRE"
			}
			return ""
		},
	})

	Register(commandRule{
		name: "RedisTtlLossOnOverwrite", desc: "覆盖写入可能清除原 TTL，且未明确指定过期策略或保留 TTL 时提示",
		cat: CategoryStructure, modes: writeOnly, level: LevelWarn,
		enabled: func(c Config) bool { return c.TtlLossOnOverwrite },
		hit: func(_ Config, cmd *Command) string {
			switch cmd.Name {
			case "SET", "GETSET":
				if cmd.HasAnyArg("EX", "PX", "EXAT", "PXAT", "KEEPTTL") {
					return ""
				}
				return cmd.Name + " 会清除键 " + cmd.Arg(1) + " 原有的 TTL；如需保留请加 KEEPTTL"
			case "MSET", "MSETNX":
				return cmd.Name + " 逐键覆盖写入，会清除这些键原有的 TTL"
			}
			return ""
		},
	})

	Register(commandRule{
		name: "RedisForbidPersist", desc: "禁止 PERSIST，避免将有过期时间的缓存转为永久保存；允许永久化的业务可单独豁免",
		cat: CategorySecurity, modes: writeOnly, level: LevelError,
		enabled: func(c Config) bool { return c.ForbidPersist },
		hit: func(_ Config, cmd *Command) string {
			if cmd.Name != "PERSIST" {
				return ""
			}
			return "PERSIST 会移除键 " + cmd.Arg(1) + " 的过期时间，使其常驻内存"
		},
	})

	Register(commandRule{
		name: "RedisMinTtl", desc: "过期时间低于业务最小值时提示；最小值按业务配置，0 表示不检查",
		cat: CategoryPerformance, modes: writeOnly, level: LevelWarn,
		enabled: func(c Config) bool { return c.MinTTL > 0 },
		hit: func(cfg Config, cmd *Command) string {
			sec, ok := expireSeconds(cmd)
			if !ok || sec >= int64(cfg.MinTTL) {
				return ""
			}
			return "过期时间 " + itoa(int(sec)) + " 秒，低于最小值 " + itoa(cfg.MinTTL) + " 秒，键可能过早失效"
		},
	})

	Register(commandRule{
		name: "RedisMaxTtl", desc: "过期时间超过业务最大值时提示；最大值按业务配置，0 表示不检查",
		cat: CategoryPerformance, modes: writeOnly, level: LevelWarn,
		enabled: func(c Config) bool { return c.MaxTTL > 0 },
		hit: func(cfg Config, cmd *Command) string {
			sec, ok := expireSeconds(cmd)
			if !ok || sec <= int64(cfg.MaxTTL) {
				return ""
			}
			return "过期时间 " + itoa(int(sec)) + " 秒，超过最大值 " + itoa(cfg.MaxTTL) + " 秒；过长的 TTL 实质上等于常驻内存"
		},
	})

	Register(commandRule{
		name: "RedisImmediateExpire", desc: "指定立即过期或已过去的过期时间时，按删除操作处理并提示",
		cat: CategoryStructure, modes: writeOnly, level: LevelWarn,
		enabled: func(c Config) bool { return c.ImmediateExpire },
		hit: func(_ Config, cmd *Command) string {
			if !immediateExpire(cmd) {
				return ""
			}
			return cmd.Name + " 的过期时间已到或为 0：Redis 会直接删除该键，等同于一次删除操作"
		},
	})
}

// expireSeconds 取本次设置的过期秒数；false = 没设或认不出（不猜）。
// 绝对时间戳按当前时间折算成剩余秒数。
func expireSeconds(cmd *Command) (int64, bool) {
	switch cmd.Name {
	case "SETEX", "EXPIRE":
		return positiveSeconds(cmd, 2, 1)
	case "PSETEX", "PEXPIRE":
		return positiveSeconds(cmd, 2, 1000)
	case "EXPIREAT", "PEXPIREAT":
		div := int64(1)
		if cmd.Name == "PEXPIREAT" {
			div = 1000
		}
		sec, ok := positiveSeconds(cmd, 2, div)
		if !ok || sec <= time.Now().Unix() {
			return 0, false
		}
		return sec - time.Now().Unix(), true
	case "RESTORE":
		// RESTORE key ttl(毫秒) payload：ttl=0 表示不设过期
		return positiveSeconds(cmd, 2, 1000)
	case "SET":
		return optionSeconds(cmd, 3)
	case "GETEX":
		return optionSeconds(cmd, 2)
	}
	return 0, false
}

// positiveSeconds 取第 i 个参数并换算成秒（毫秒按 div 折算）；不是正数视为未设置
func positiveSeconds(cmd *Command, i int, div int64) (int64, bool) {
	n, ok := cmd.IntArg(i)
	if !ok || n <= 0 {
		return 0, false
	}
	return n / div, true
}

// optionSeconds 从选项区第 from 个参数起找 EX / PX / EXAT / PXAT，返回换算后的秒数
func optionSeconds(cmd *Command, from int) (int64, bool) {
	for i := from; i+1 < cmd.ArgCount(); i++ {
		switch strings.ToUpper(cmd.Arg(i)) {
		case "EX":
			return positiveSeconds(cmd, i+1, 1)
		case "PX":
			return positiveSeconds(cmd, i+1, 1000)
		case "EXAT", "PXAT":
			div := int64(1)
			if strings.EqualFold(cmd.Arg(i), "PXAT") {
				div = 1000
			}
			sec, ok := positiveSeconds(cmd, i+1, div)
			if !ok || sec <= time.Now().Unix() {
				return 0, false
			}
			return sec - time.Now().Unix(), true
		}
	}
	return 0, false
}

// immediateExpire 过期时间是否已到或为 0（等同删除）
func immediateExpire(cmd *Command) bool {
	now := time.Now().Unix()
	switch cmd.Name {
	case "EXPIRE", "PEXPIRE", "SETEX", "PSETEX":
		n, ok := cmd.IntArg(2)
		return ok && n <= 0
	case "EXPIREAT":
		n, ok := cmd.IntArg(2)
		return ok && n <= now
	case "PEXPIREAT":
		n, ok := cmd.IntArg(2)
		return ok && n/1000 <= now
	case "SET", "GETEX":
		from := 2
		if cmd.Name == "SET" {
			from = 3
		}
		for i := from; i+1 < cmd.ArgCount(); i++ {
			switch strings.ToUpper(cmd.Arg(i)) {
			case "EX", "PX":
				if n, ok := cmd.IntArg(i + 1); ok && n <= 0 {
					return true
				}
			case "EXAT":
				if n, ok := cmd.IntArg(i + 1); ok && n <= now {
					return true
				}
			case "PXAT":
				if n, ok := cmd.IntArg(i + 1); ok && n/1000 <= now {
					return true
				}
			}
		}
	}
	return false
}

// requireTtlCommands 会创建键、且命令本身不带过期参数：新建后需另发 EXPIRE
var requireTtlCommands = map[string]struct{}{
	"SETNX": {}, "MSETNX": {},
	"HSET": {}, "HSETNX": {}, "HMSET": {},
	"LPUSH": {}, "RPUSH": {}, "LPUSHX": {}, "RPUSHX": {}, "LINSERT": {},
	"SADD": {}, "ZADD": {}, "XADD": {},
	"INCR": {}, "DECR": {}, "INCRBY": {}, "DECRBY": {}, "INCRBYFLOAT": {},
	"SETBIT": {}, "PFADD": {}, "GEOADD": {},
}

func inNameSet(set map[string]struct{}, name string) bool {
	_, ok := set[name]
	return ok
}
