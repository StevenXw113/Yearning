package redischeck

import "strings"

// 读取与扫描：全量遍历、无界范围读取、批量键读取与高代价读取。
func init() {
	Register(commandRule{
		name: "RedisForbidKeysCommand", desc: "禁止 KEYS，包括带业务前缀的 KEYS；建议改用受限 SCAN",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.ForbidKeysCommand },
		hit: func(_ Config, cmd *Command) string {
			if cmd.Name != "KEYS" {
				return ""
			}
			return "禁止 KEYS：它会遍历整个键空间并阻塞实例，请改用带 MATCH 的 SCAN"
		},
	})

	Register(commandRule{
		name: "RedisForbidRandomKey", desc: "禁止 RANDOMKEY，避免读取范围超出业务键授权范围",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.ForbidRandomKey },
		hit: func(_ Config, cmd *Command) string {
			if cmd.Name != "RANDOMKEY" {
				return ""
			}
			return "禁止 RANDOMKEY：随机读取无法限定在授权键范围内"
		},
	})

	Register(commandRule{
		name: "RedisScanMatchRequired", desc: "SCAN 必须指定允许的业务匹配范围；MATCH 规则无法确认属于授权范围时拦截",
		cat: CategorySecurity, modes: queryOnly, level: LevelError,
		enabled: func(c Config) bool { return c.ScanMatchRequired },
		hit: func(_ Config, cmd *Command) string {
			switch cmd.Name {
			case "SCAN", "HSCAN", "SSCAN", "ZSCAN":
				if cmd.HasAnyArg("MATCH") {
					return ""
				}
				return cmd.Name + " 未带 MATCH，会遍历整个键空间；请加上业务前缀限定范围"
			}
			return ""
		},
	})

	Register(commandRule{
		name: "RedisMaxScanCount", desc: "SCAN、HSCAN、SSCAN、ZSCAN 的 COUNT 建议不超过配置值；COUNT 仅是工作量提示，不代表结果条数上限",
		cat: CategoryPerformance, modes: queryOnly, level: LevelWarn,
		enabled: func(c Config) bool { return c.MaxScanCount > 0 },
		hit: func(cfg Config, cmd *Command) string {
			switch cmd.Name {
			case "SCAN", "HSCAN", "SSCAN", "ZSCAN":
			default:
				return ""
			}
			for i := 2; i+1 < cmd.ArgCount(); i++ {
				if !strings.EqualFold(cmd.Arg(i), "COUNT") {
					continue
				}
				n, ok := cmd.IntArg(i + 1)
				if !ok || n <= int64(cfg.MaxScanCount) {
					return ""
				}
				return cmd.Name + " 的 COUNT=" + itoa(int(n)) + " 超过建议值 " + itoa(cfg.MaxScanCount)
			}
			return ""
		},
	})

	Register(commandRule{
		name: "RedisFullCollectionRead", desc: "HGETALL、SMEMBERS、全量 LRANGE/ZRANGE 等操作，在集合规模未知或超过阈值时提示",
		cat: CategoryPerformance, modes: queryOnly, level: LevelWarn,
		enabled: func(c Config) bool { return c.FullCollectionRead },
		hit: func(_ Config, cmd *Command) string {
			switch cmd.Name {
			case "HGETALL", "HKEYS", "HVALS", "SMEMBERS", "SUNION", "SINTER", "SDIFF":
				return cmd.Name + " 会一次返回整个集合，大 key 上容易拖垮查询连接，建议改用 SCAN 系命令"
			case "LRANGE", "ZRANGE", "ZREVRANGE", "ZRANGEBYSCORE", "ZREVRANGEBYSCORE", "ZRANGEBYLEX", "ZREVRANGEBYLEX":
				if fullRange(cmd) {
					return cmd.Name + " 取的是整段区间，大 key 上会一次返回全部元素，建议分页取"
				}
			}
			return ""
		},
	})

	Register(commandRule{
		name: "RedisMaxReadElements", desc: "单次范围查询或随机读取请求的元素数上限，0 表示不限制；无法确定上限时提示",
		cat: CategoryPerformance, modes: queryOnly, level: LevelWarn,
		enabled: func(c Config) bool { return c.MaxReadElements > 0 },
		hit: func(cfg Config, cmd *Command) string {
			n, known := readElements(cmd)
			if !known {
				if isRangeRead(cmd.Name) {
					return cmd.Name + " 的读取范围无法确定上限（负索引或开区间），大 key 上可能一次读完"
				}
				return ""
			}
			if n <= int64(cfg.MaxReadElements) {
				return ""
			}
			return cmd.Name + " 单次读取 " + itoa(int(n)) + " 个元素，超过上限 " + itoa(cfg.MaxReadElements)
		},
	})

	Register(commandRule{
		name: "RedisMaxReadKeys", desc: "MGET 等单条批量读取命令的键数上限，0 表示不限制",
		cat: CategoryPerformance, modes: allModes, level: LevelWarn,
		enabled: func(c Config) bool { return c.MaxReadKeys > 0 },
		hit: func(cfg Config, cmd *Command) string {
			switch cmd.Name {
			case "MGET", "EXISTS", "TOUCH", "PFCOUNT", "SUNION", "SINTER", "SDIFF", "SINTERCARD", "WATCH":
			default:
				return ""
			}
			if len(cmd.Keys) <= cfg.MaxReadKeys {
				return ""
			}
			return cmd.Name + " 一次读取 " + itoa(len(cmd.Keys)) + " 个键，超过上限 " + itoa(cfg.MaxReadKeys) + "，请分批读取"
		},
	})

	Register(commandRule{
		name: "RedisExpensiveRead", desc: "SORT、集合交并差及其他复杂读取，输入规模未知或超过预算时提示",
		cat: CategoryPerformance, modes: allModes, level: LevelWarn,
		enabled: func(c Config) bool { return c.ExpensiveRead },
		hit: func(_ Config, cmd *Command) string {
			switch cmd.Name {
			case "SORT", "SORT_RO":
				return cmd.Name + " 会对整段数据排序（O(N log N)），数据量大时会占满单线程实例"
			case "SINTER", "SUNION", "SDIFF":
				return cmd.Name + " 需要遍历所有输入集合，集合规模未知时代价不可控"
			}
			return ""
		},
	})
}

// fullRange 判断区间参数是否覆盖整段：按索引/分值 0 -1、-inf +inf，按字典序 - +
func fullRange(cmd *Command) bool {
	lo := strings.ToLower(cmd.Arg(2))
	hi := strings.ToLower(cmd.Arg(3))
	return (lo == "0" || lo == "-inf" || lo == "-") && (hi == "-1" || hi == "+inf" || hi == "+")
}

// isRangeRead 是否是区间读取命令
func isRangeRead(name string) bool {
	switch name {
	case "LRANGE", "ZRANGE", "ZREVRANGE", "ZRANGEBYSCORE", "ZREVRANGEBYSCORE", "ZRANGEBYLEX", "ZREVRANGEBYLEX":
		return true
	}
	return false
}

// readElements 单次读取的元素数；第二个返回 false 表示无法确定上限
func readElements(cmd *Command) (int64, bool) {
	if isRangeRead(cmd.Name) {
		lo, ok1 := cmd.IntArg(2)
		hi, ok2 := cmd.IntArg(3)
		if !ok1 || !ok2 || lo < 0 || hi < 0 {
			return 0, false
		}
		if hi < lo {
			return 0, true
		}
		return hi - lo + 1, true
	}
	switch cmd.Name {
	case "SRANDMEMBER", "ZRANDMEMBER", "HRANDFIELD":
		if cmd.ArgCount() <= 2 {
			return 1, true
		}
		n, ok := cmd.IntArg(2)
		if !ok {
			return 0, false
		}
		if n < 0 {
			n = -n
		}
		return n, true
	}
	return 0, false
}
