package redischeck

// 写入与数据变更：单值与整条命令的体量上限、元素数上限、非幂等写入、
// 按偏移写入的最终大小、覆盖写入与结果落盘。
func init() {
	Register(commandRule{
		name: "RedisMaxValueSize", desc: "单个新写入值的字节数上限，0 表示不限制；超过上限拦截",
		cat: CategoryPerformance, modes: writeOnly, level: LevelError,
		enabled: func(c Config) bool { return c.MaxValueSize > 0 },
		hit: func(cfg Config, cmd *Command) string {
			longest := 0
			for _, a := range valueArgs(cmd) {
				if len(a) > longest {
					longest = len(a)
				}
			}
			if longest <= cfg.MaxValueSize {
				return ""
			}
			return "单个值 " + itoa(longest) + " 字节，超过上限 " + itoa(cfg.MaxValueSize) + " 字节"
		},
	})

	Register(commandRule{
		name: "RedisMaxCommandPayload", desc: "单条命令参数总字节数上限，0 表示不限制；防止多个小值绕过单值大小限制",
		cat: CategoryPerformance, modes: writeOnly, level: LevelError,
		enabled: func(c Config) bool { return c.MaxCommandPayload > 0 },
		hit: func(cfg Config, cmd *Command) string {
			total := 0
			for _, a := range cmd.ArgsFrom(1) {
				total += len(a)
			}
			if total <= cfg.MaxCommandPayload {
				return ""
			}
			return "单条命令参数合计 " + itoa(total) + " 字节，超过上限 " + itoa(cfg.MaxCommandPayload) + " 字节，请拆分"
		},
	})

	Register(commandRule{
		name: "RedisMaxWriteKeys", desc: "单条写入命令涉及的键数上限，0 表示不限制；超过上限转批量审核",
		cat: CategoryPerformance, modes: writeOnly, level: LevelError,
		enabled: func(c Config) bool { return c.MaxWriteKeys > 0 },
		hit: func(cfg Config, cmd *Command) string {
			n := len(cmd.Keys)
			if n <= cfg.MaxWriteKeys {
				return ""
			}
			return "本次命令涉及 " + itoa(n) + " 个键，超过上限 " + itoa(cfg.MaxWriteKeys) + "，请分批提交"
		},
	})

	Register(commandRule{
		name: "RedisMaxWriteElements", desc: "单条命令新增或修改的字段、成员、列表元素数上限，0 表示不限制",
		cat: CategoryPerformance, modes: writeOnly, level: LevelError,
		enabled: func(c Config) bool { return c.MaxWriteElements > 0 },
		hit: func(cfg Config, cmd *Command) string {
			n, ok := writeElements(cmd)
			if !ok || n <= cfg.MaxWriteElements {
				return ""
			}
			return cmd.Name + " 本次写入 " + itoa(n) + " 个元素，超过上限 " + itoa(cfg.MaxWriteElements) + "，请分批提交"
		},
	})

	Register(commandRule{
		name: "RedisNonIdempotentWrite", desc: "INCR、DECR、计数增量、列表弹出等重复执行会改变结果的操作，提示重复执行风险",
		cat: CategoryPerformance, modes: writeOnly, level: LevelWarn,
		enabled: func(c Config) bool { return c.NonIdempotentWrite },
		hit: func(_ Config, cmd *Command) string {
			if _, ok := nonIdempotentCommands[cmd.Name]; !ok {
				return ""
			}
			return cmd.Name + " 不是幂等操作：重试或重复执行会再次改变结果，回滚不能用同一条命令"
		},
	})

	Register(commandRule{
		name: "RedisLargeOffsetWrite", desc: "SETRANGE、SETBIT 等按偏移写入的操作，预计对象最终大小超过阈值时拦截，0 表示不限制",
		cat: CategoryPerformance, modes: writeOnly, level: LevelError,
		enabled: func(c Config) bool { return c.LargeOffsetWrite > 0 },
		hit: func(cfg Config, cmd *Command) string {
			size, ok := offsetWriteSize(cmd)
			if !ok || size <= int64(cfg.LargeOffsetWrite) {
				return ""
			}
			return cmd.Name + " 预计使键增长到 " + itoa(int(size)) + " 字节，超过上限 " + itoa(cfg.LargeOffsetWrite) + " 字节"
		},
	})

	Register(commandRule{
		name: "RedisForbidRename", desc: "禁止 RENAME、RENAMENX；RENAME 可能覆盖目标键，RENAMENX 虽不覆盖但仍会改变键名与业务归属",
		cat: CategoryStructure, modes: writeOnly, level: LevelError,
		enabled: func(c Config) bool { return c.ForbidRename },
		hit: func(_ Config, cmd *Command) string {
			switch cmd.Name {
			case "RENAME":
				return "RENAME 会覆盖目标键 " + cmd.Arg(2) + " 的原有数据，且没有回滚余地"
			case "RENAMENX":
				return "RENAMENX 会改变键名归属，业务侧按旧键名读取会失败"
			}
			return ""
		},
	})

	Register(commandRule{
		name: "RedisCopyOverwrite", desc: "COPY 使用 REPLACE 等覆盖行为时提示，并检查源键、目标键及目标 DB 权限",
		cat: CategoryStructure, modes: writeOnly, level: LevelWarn,
		enabled: func(c Config) bool { return c.CopyOverwrite },
		hit: func(_ Config, cmd *Command) string {
			if cmd.Name != "COPY" || !cmd.HasAnyArg("REPLACE") {
				return ""
			}
			return "COPY ... REPLACE 会覆盖目标键 " + cmd.Arg(2) + " 的原有数据"
		},
	})

	Register(commandRule{
		name: "RedisStoreOperation", desc: "集合计算或排序结果写入目标键时，检查输入规模、目标覆盖及所有源键和目标键权限",
		cat: CategoryPerformance, modes: allModes, level: LevelWarn,
		enabled: func(c Config) bool { return c.StoreOperation },
		hit: func(_ Config, cmd *Command) string {
			switch cmd.Name {
			case "SORT", "SORT_RO":
				if !cmd.HasAnyArg("STORE") {
					return ""
				}
				return "SORT ... STORE 会把结果写入 " + cmd.Arg(cmd.ArgCount()-1) + "，输入规模与目标覆盖都要确认"
			case "SINTERSTORE", "SUNIONSTORE", "SDIFFSTORE", "ZUNIONSTORE", "ZINTERSTORE", "ZDIFFSTORE", "ZRANGESTORE", "GEOSEARCHSTORE":
				return cmd.Name + " 会把计算结果写入目标键，请确认覆盖行为与源集合规模"
			case "GEORADIUS", "GEORADIUSBYMEMBER":
				if !cmd.HasAnyArg("STORE", "STOREDIST") {
					return ""
				}
				return cmd.Name + " 带 STORE 时会把结果写入目标键，请确认覆盖行为"
			}
			return ""
		},
	})
}

// nonIdempotentCommands 重复执行会改变结果的命令
var nonIdempotentCommands = map[string]struct{}{
	"INCR": {}, "DECR": {}, "INCRBY": {}, "DECRBY": {}, "INCRBYFLOAT": {},
	"HINCRBY": {}, "HINCRBYFLOAT": {}, "ZINCRBY": {},
	"LPOP": {}, "RPOP": {}, "SPOP": {}, "ZPOPMIN": {}, "ZPOPMAX": {},
	"LMOVE": {}, "RPOPLPUSH": {}, "LMPOP": {}, "ZMPOP": {},
}

// writeElements 单条命令新增/修改的元素数；第二个返回 false 表示无法判断
func writeElements(cmd *Command) (int, bool) {
	switch cmd.Name {
	case "HSET", "HMSET", "ZADD":
		// HSET key f v [f v ...] / ZADD key [NX] score member ...
		return (cmd.ArgCount() - 2) / 2, true
	case "LPUSH", "RPUSH", "LPUSHX", "RPUSHX", "SADD", "PFADD":
		return cmd.ArgCount() - 2, true
	case "MSET", "MSETNX":
		return (cmd.ArgCount() - 1) / 2, true
	}
	return 0, false
}

// offsetWriteSize 按偏移写入后的预计对象大小（字节）
func offsetWriteSize(cmd *Command) (int64, bool) {
	switch cmd.Name {
	case "SETRANGE":
		off, ok := cmd.IntArg(2)
		if !ok || off < 0 {
			return 0, false
		}
		return off + int64(len(cmd.Arg(3))), true
	case "SETBIT":
		off, ok := cmd.IntArg(2)
		if !ok || off < 0 {
			return 0, false
		}
		return off/8 + 1, true
	}
	return 0, false
}

// valueArgs 取命令里的「值」参数：键名不算，否则 MSET 这类命令会把键长当成值长。
// 键值成对的命令按位置取；单值命令取固定位置；其余命令用「排除键名」兜底。
func valueArgs(cmd *Command) []string {
	switch cmd.Name {
	case "MSET", "MSETNX":
		var out []string
		for i := 2; i < cmd.ArgCount(); i += 2 {
			out = append(out, cmd.Arg(i))
		}
		return out
	case "HSET", "HMSET", "HSETNX":
		var out []string
		for i := 3; i < cmd.ArgCount(); i += 2 {
			out = append(out, cmd.Arg(i))
		}
		return out
	case "SET", "SETNX", "GETSET":
		if cmd.ArgCount() > 2 {
			return []string{cmd.Arg(2)}
		}
		return nil
	case "SETEX", "PSETEX", "SETRANGE":
		if cmd.ArgCount() > 3 {
			return []string{cmd.Arg(3)}
		}
		return nil
	}
	keys := make(map[string]struct{}, len(cmd.Keys))
	for _, k := range cmd.Keys {
		keys[k] = struct{}{}
	}
	out := make([]string, 0, cmd.ArgCount())
	for _, a := range cmd.ArgsFrom(1) {
		if _, isKey := keys[a]; isKey {
			continue
		}
		out = append(out, a)
	}
	return out
}
