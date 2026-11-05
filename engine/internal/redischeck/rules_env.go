package redischeck

import (
	"strconv"
	"strings"
)

// 环境与版本类规则：判定依据是调用方随请求传入的运行时事实
// （数据源是否生产、目标实例的 Redis 版本），引擎本身不连库。

func init() {
	Register(commandRule{
		name: "RedisProductionWrite", desc: "生产数据源上的写命令，需要确认影响面与回滚方式",
		// 归结构类：生产写本身是正常业务，规则是提醒补影响面，不该直接拦
		cat: CategoryStructure, modes: writeOnly, level: LevelWarn,
		enabled: func(c Config) bool { return c.ProductionWrite },
		hit: func(cfg Config, cmd *Command) string {
			if !cfg.Production || cmd.Kind == KindQuery {
				return ""
			}
			return cmd.Name + " 在生产数据源上执行，请在备注里写明影响面与回滚方式"
		},
	})

	Register(commandRule{
		name: "RedisCommandCompatibility", desc: "命令需要更高的 Redis 版本，当前实例不支持",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.CommandCompatibility },
		hit: func(cfg Config, cmd *Command) string {
			need, ok := minRedisVersion[cmd.Name]
			if !ok || cfg.RedisVersion == "" {
				return "" // 版本未知不拦（探测失败不该变成误拦）
			}
			if versionLess(cfg.RedisVersion, need) {
				return cmd.Name + " 需要 Redis " + need + " 及以上，当前实例是 " + cfg.RedisVersion
			}
			return ""
		},
	})
}

// minRedisVersion 命令的最低 Redis 版本（只登记「引入版本明确」的常用命令）。
// 表要跟着命令增长维护：漏登记只是不检查，不会误报。
var minRedisVersion = map[string]string{
	"UNLINK":         "4.0",
	"HSET":           "4.0",
	"MEMORY":         "4.0",
	"SWAPDB":         "4.0",
	"ZADD":           "3.0",
	"XADD":           "5.0",
	"XREAD":          "5.0",
	"XINFO":          "5.0",
	"ZPOPMIN":        "5.0",
	"ZPOPMAX":        "5.0",
	"BZPOPMIN":       "5.0",
	"BZPOPMAX":       "5.0",
	"STREAMS":        "5.0",
	"ZRANDMEMBER":    "6.2",
	"HRANDFIELD":     "6.2",
	"GETDEL":         "6.2",
	"GETEX":          "6.2",
	"SMISMEMBER":     "6.2",
	"ZDIFF":          "6.2",
	"ZUNION":         "6.2",
	"ZINTER":         "6.2",
	"ZRANGESTORE":    "6.2",
	"SINTERCARD":     "7.0",
	"ZINTERCARD":     "7.0",
	"FUNCTION":       "7.0",
	"FCALL":          "7.0",
	"FCALL_RO":       "7.0",
	"LMPOP":          "7.0",
	"ZMPOP":          "7.0",
	"BLMPOP":         "7.0",
	"BZMPOP":         "7.0",
	"OBJECT":         "2.2",
	"BITFIELD_RO":    "6.0",
	"ZRANGEBYLEX":    "2.8",
	"ZREVRANGEBYLEX": "2.8",
}

// versionLess 比较 major.minor（补丁号忽略）：have < need 时返回 true。
// 任一版本号解析不出来就返回 false——宁可漏报也不误报。
func versionLess(have, need string) bool {
	h, ok1 := parseMajorMinor(have)
	n, ok2 := parseMajorMinor(need)
	if !ok1 || !ok2 {
		return false
	}
	if h[0] != n[0] {
		return h[0] < n[0]
	}
	return h[1] < n[1]
}

func parseMajorMinor(v string) ([2]int, bool) {
	var out [2]int
	parts := strings.Split(strings.TrimSpace(v), ".")
	if len(parts) == 0 {
		return out, false
	}
	major, err := strconv.Atoi(strings.TrimFunc(parts[0], func(r rune) bool { return r < '0' || r > '9' }))
	if err != nil {
		return out, false
	}
	out[0] = major
	if len(parts) > 1 {
		if minor, err := strconv.Atoi(strings.TrimFunc(parts[1], func(r rune) bool { return r < '0' || r > '9' })); err == nil {
			out[1] = minor
		}
	}
	return out, true
}
