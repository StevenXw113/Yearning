package redischeck

import "strings"

// 键状态类规则：键类型/大小由调用方探测后放进 Config.KeySnapshot，引擎不连库。
// 快照里缺席的键按未知处理，只提示不拦截。
const (
	largeValueBytes = 1 << 20 // 1 MB，超过视为大 key
	bigKeyElements  = 1000    // DEL 同步删除超过它就会阻塞
)

func init() {
	Register(commandRule{
		name: "RedisTypeMismatch", desc: "写命令的键类型与命令期望不符，执行会直接报 WRONGTYPE",
		cat: CategorySecurity, modes: writeOnly, level: LevelError,
		enabled: func(c Config) bool { return c.TypeMismatch },
		hit: func(cfg Config, cmd *Command) string {
			want, ok := writeTypeOf[cmd.Name]
			if !ok {
				return ""
			}
			for _, k := range cmd.Keys {
				st, seen := cfg.KeySnapshot[k]
				if !seen || st.Type == "" || st.Type == "none" {
					continue
				}
				if st.Type != want {
					return cmd.Name + " 要求 " + want + " 类型，键 " + k + " 实际是 " + st.Type +
						"（执行会报 WRONGTYPE）"
				}
			}
			return ""
		},
	})

	Register(commandRule{
		name: "RedisOverwriteExisting", desc: "覆盖写入已存在的键，原值会丢失",
		cat: CategoryStructure, modes: writeOnly, level: LevelWarn,
		enabled: func(c Config) bool { return c.OverwriteExisting },
		hit: func(cfg Config, cmd *Command) string {
			if _, ok := overwriteCommands[cmd.Name]; !ok {
				return ""
			}
			var hit []string
			for _, k := range cmd.Keys {
				if st, seen := cfg.KeySnapshot[k]; seen && st.Type != "" && st.Type != "none" {
					hit = append(hit, k)
				}
			}
			if len(hit) == 0 {
				return ""
			}
			return cmd.Name + " 会覆盖已存在的键 " + strings.Join(hit, "、") + "，原值将丢失"
		},
	})

	Register(commandRule{
		name: "RedisLargeValueRead", desc: "读取超大 key（> 1 MB）会占用查询连接与带宽",
		cat: CategoryPerformance, modes: queryOnly, level: LevelWarn,
		enabled: func(c Config) bool { return c.LargeValueRead },
		hit: func(cfg Config, cmd *Command) string {
			if _, ok := readValueCommands[cmd.Name]; !ok {
				return ""
			}
			for _, k := range cmd.Keys {
				if st, seen := cfg.KeySnapshot[k]; seen && st.Size >= largeValueBytes {
					return cmd.Name + " 读取的键 " + k + " 约 " + itoa(int(st.Size)) +
						" 字节，超过 1 MB，会长时间占用连接"
				}
			}
			return ""
		},
	})

	Register(commandRule{
		name: "RedisBigKeySyncDelete", desc: "同步删除大 key 会阻塞服务端，建议改用 UNLINK",
		cat: CategoryPerformance, modes: writeOnly, level: LevelWarn,
		enabled: func(c Config) bool { return c.BigKeySyncDelete },
		hit: func(cfg Config, cmd *Command) string {
			// UNLINK 本来就是后台回收，不需要提示
			if cmd.Name != "DEL" {
				return ""
			}
			for _, k := range cmd.Keys {
				st, seen := cfg.KeySnapshot[k]
				if !seen {
					continue
				}
				if st.Len >= bigKeyElements || st.Size >= largeValueBytes {
					return "DEL " + k + " 的元素数约 " + itoa(int(st.Len)) + "、占用约 " +
						itoa(int(st.Size)) + " 字节，同步删除会阻塞服务端；建议改用 UNLINK"
				}
			}
			return ""
		},
	})
}

// writeTypeOf 写命令期望的键类型（只登记「类型确定」的命令）
var writeTypeOf = map[string]string{
	"SET": "string", "SETNX": "string", "SETEX": "string", "PSETEX": "string", "GETSET": "string",
	"APPEND": "string", "SETRANGE": "string", "SETBIT": "string",
	"INCR": "string", "DECR": "string", "INCRBY": "string", "DECRBY": "string", "INCRBYFLOAT": "string",
	"PFADD": "string",

	"HSET": "hash", "HSETNX": "hash", "HMSET": "hash", "HINCRBY": "hash", "HINCRBYFLOAT": "hash",

	"LPUSH": "list", "RPUSH": "list", "LPUSHX": "list", "RPUSHX": "list",
	"LINSERT": "list", "LSET": "list", "LREM": "list", "LTRIM": "list",

	"SADD": "set", "SREM": "set", "SMOVE": "set",

	"ZADD": "zset", "ZINCRBY": "zset", "ZREM": "zset", "ZREMRANGEBYRANK": "zset",

	"GEOADD": "zset",

	"XADD": "stream",
}

// overwriteCommands 覆盖写入类：目标键已存在时原值会丢失
var overwriteCommands = map[string]struct{}{
	"SET": {}, "SETNX": {}, "SETEX": {}, "PSETEX": {}, "GETSET": {}, "MSET": {}, "MSETNX": {},
	"HSET": {}, "HMSET": {}, "RESTORE": {}, "COPY": {},
}

// readValueCommands 会读取整键值的读命令
var readValueCommands = map[string]struct{}{
	"GET": {}, "MGET": {}, "GETRANGE": {}, "HGETALL": {}, "HVALS": {},
	"LRANGE": {}, "SMEMBERS": {}, "SUNION": {}, "SINTER": {}, "SDIFF": {},
	"ZRANGE": {}, "ZREVRANGE": {}, "XRANGE": {}, "XREVRANGE": {}, "GEOSEARCH": {},
}
