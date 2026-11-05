package redischeck

import "strings"

// Cluster / Proxy 拓扑类规则。
//
// 判定依据是部署形态（Config.Deployment），由主程序探测实例后随审核请求传入。
// 形态未知（空串）时这些规则一律不生效：未知既不当违规，也不当安全——
// 拿不到拓扑就贸然按集群语义拦，会把单机实例的正常命令全部误判。
func init() {
	Register(commandRule{
		name: "RedisClusterDbZero", desc: "原生 Cluster 仅允许 DB 0；选择其它 DB 时拦截",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.Deployment == "cluster" && c.ClusterDbZero },
		hit: func(cfg Config, _ *Command) string {
			if db := dbIndexOfSchema(cfg.Schema); db > 0 {
				return "Cluster 只支持 db0，当前目标库是 " + cfg.Schema
			}
			return ""
		},
	})

	Register(commandRule{
		name: "RedisClusterCrossSlot", desc: "Cluster 中多键命令必须落在同一 slot，否则拦截",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.Deployment == "cluster" && c.ClusterCrossSlot },
		hit: func(_ Config, cmd *Command) string {
			if len(cmd.Keys) < 2 {
				return ""
			}
			first := cmd.Keys[0]
			slot := redisSlot(first)
			var bad []string
			for _, k := range cmd.Keys[1:] {
				if redisSlot(k) != slot {
					bad = append(bad, k)
				}
			}
			if len(bad) == 0 {
				return ""
			}
			return "键 " + strings.Join(bad, "、") + " 与 " + first +
				" 不在同一 slot（Cluster 要求多键同 slot，可用 {tag} 强制）"
		},
	})

	Register(commandRule{
		name: "RedisClusterScanScope", desc: "Cluster 下 SCAN 只覆盖单个节点，不能当作全局遍历",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.Deployment == "cluster" && c.ClusterScanScope },
		hit:     scanFamilyHit("Cluster 下 SCAN 只遍历当前节点，覆盖范围不完整，不能作为全局扫描或批量变更依据"),
	})

	Register(commandRule{
		name: "RedisClusterTransactionSlot", desc: "Cluster 事务要求所有键同一 slot；单命令入口无法保证，拦截",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.Deployment == "cluster" && c.ClusterTransactionSlot },
		hit:     transactionHit("Cluster 事务必须所有键在同一 slot，当前入口无法校验完整事务"),
	})

	Register(commandRule{
		name: "RedisClusterScriptSlot", desc: "Cluster 脚本/函数必须显式声明同 slot 的键；未验证时拦截",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.Deployment == "cluster" && c.ClusterScriptSlot },
		hit:     scriptHit("Cluster 下脚本要求所有键落在同一 slot，无法确认键声明"),
	})

	// ---- Proxy：能力未知即拦截，与"未知不误判为安全"一致 ----
	Register(commandRule{
		name: "RedisProxyDbSupport", desc: "Proxy 未验证支持非 db0 逻辑库时拦截",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.Deployment == "proxy" && c.ProxyDbSupport },
		hit: func(cfg Config, _ *Command) string {
			if db := dbIndexOfSchema(cfg.Schema); db > 0 {
				return "代理模式未验证支持 " + cfg.Schema + "，请使用 db0"
			}
			return ""
		},
	})

	Register(commandRule{
		name: "RedisProxyCrossShard", desc: "多键命令的键跨 slot 时拦截（代理按 slot 路由，跨分片命令无法保证结果）",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.Deployment == "proxy" && c.ProxyCrossShard },
		hit: func(_ Config, cmd *Command) string {
			if len(cmd.Keys) < 2 || sameSlot(cmd.Keys) {
				return ""
			}
			return "键 " + strings.Join(cmd.Keys, "、") +
				" 不在同一 slot：代理按 slot 路由，跨分片的多键命令会被拒绝或结果不一致；请用 {tag} 强制同 slot 或拆成单键"
		},
	})

	Register(commandRule{
		name: "RedisProxyCommandSupport", desc: "Proxy 集群未实现或受限的命令（如 Redis 4.0 代理不支持 BZPOPMAX/BZPOPMIN）",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.Deployment == "proxy" && c.ProxyCommandSupport },
		hit: func(_ Config, cmd *Command) string {
			if _, bad := proxyUnsupportedCommands[cmd.Name]; bad {
				return cmd.Name + " 在 Proxy 集群上未实现或受限" +
					"（华为云 DCS：Redis 4.0 代理不支持 BZPOPMAX/BZPOPMIN；5.0+ 要求所有键同 slot）"
			}
			return ""
		},
	})

	Register(commandRule{
		name: "RedisProxyAtomicity", desc: "源→目的类命令跨分片无法保证原子性（SMOVE/RENAME/RPOPLPUSH/BITOP/MSETNX 等要求同 slot）",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.Deployment == "proxy" && c.ProxyAtomicity },
		hit: func(_ Config, cmd *Command) string {
			if _, ok := proxyAtomicCommands[cmd.Name]; !ok {
				return ""
			}
			if len(cmd.Keys) < 2 || sameSlot(cmd.Keys) {
				return ""
			}
			return cmd.Name + " 涉及多个键且跨 slot：代理无法在单个分片内完成，原子性无法保证" +
				"（请用 {tag} 强制同 slot，或拆成多条单键命令）"
		},
	})

	Register(commandRule{
		name: "RedisProxyScriptSupport", desc: "Proxy 集群要求脚本显式声明 key（numkeys ≥ 1）且所有 key 同 slot",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.Deployment == "proxy" && c.ProxyScriptSupport },
		hit: func(_ Config, cmd *Command) string {
			switch cmd.Name {
			case "EVAL", "EVALSHA", "EVAL_RO", "EVALSHA_RO", "FCALL", "FCALL_RO":
			default:
				return ""
			}
			n, ok := cmd.IntArg(2)
			if !ok || n < 1 {
				return cmd.Name + " 未声明 key：Proxy 集群要求 numkeys ≥ 1，" +
					"否则报 ERR eval/evalsha numkeys must be bigger than zero in redis cluster mode"
			}
			if len(cmd.Keys) > 1 && !sameSlot(cmd.Keys) {
				return cmd.Name + " 的 key 跨 slot：代理按第一个 key 路由，其余 key 会失败（请用 {tag} 同 slot）"
			}
			return ""
		},
	})

	Register(commandRule{
		name: "RedisProxyTransactionSupport", desc: "Proxy 集群只支持单分片内事务，MULTI/EXEC 与 WATCH 跨分片无法保证",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.Deployment == "proxy" && c.ProxyTransactionSupport },
		hit:     transactionHit("代理集群只支持单分片内事务：MULTI/EXEC 与 WATCH 跨分片无法保证，请拆成单键操作"),
	})

	Register(commandRule{
		name: "RedisProxyScanScope", desc: "Proxy 下扫描覆盖范围未知，不能作为全局扫描或批量变更依据",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.Deployment == "proxy" && c.ProxyScanScope },
		hit:     scanFamilyHit("代理模式下 SCAN 的覆盖范围与游标语义未验证，不能作为全局扫描或批量变更依据"),
	})

	// 事务完整性：当前入口按单条命令审核，MULTI 与 EXEC 之间的命令无法保证都被审到，
	// 因此事务控制命令一律拒绝（与部署形态无关）。
	Register(commandRule{
		name: "RedisTransactionIntegrity", desc: "MULTI/EXEC 必须组成完整审核单元；单命令入口无法保证，直接拦截",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.TransactionIntegrity },
		hit:     transactionHit("当前入口按单条命令审核，无法保证 MULTI/EXEC 成对且事务内命令全部受审"),
	})
}

// scanFamilyHit 命中 SCAN 家族（游标遍历类命令）
func scanFamilyHit(msg string) func(Config, *Command) string {
	return func(_ Config, cmd *Command) string {
		switch cmd.Name {
		case "SCAN", "HSCAN", "SSCAN", "ZSCAN":
			return msg
		}
		return ""
	}
}

// transactionHit 命中事务控制命令
func transactionHit(msg string) func(Config, *Command) string {
	return func(_ Config, cmd *Command) string {
		switch cmd.Name {
		case "MULTI", "EXEC", "DISCARD", "WATCH", "UNWATCH":
			return msg
		}
		return ""
	}
}

// scriptHit 命中脚本/函数执行命令
func scriptHit(msg string) func(Config, *Command) string {
	return func(_ Config, cmd *Command) string {
		switch cmd.Name {
		case "EVAL", "EVALSHA", "EVAL_RO", "EVALSHA_RO", "FCALL", "FCALL_RO":
			return msg
		}
		return ""
	}
}

// dbIndexOfSchema 从库名（dbN）解析库号；空名或非法值按 db0 处理（库名合法性由 RedisDbScope 管）。
func dbIndexOfSchema(schema string) int {
	s := strings.TrimSpace(schema)
	if len(s) < 3 || !strings.EqualFold(s[:2], "db") {
		return 0
	}
	n := 0
	for i := 2; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0
		}
		n = n*10 + int(s[i]-'0')
	}
	return n
}

// redisSlot 计算键在集群里的 slot：CRC16-XMODEM mod 16384，支持 {hash tag}。
func redisSlot(key string) int {
	if i := strings.IndexByte(key, '{'); i >= 0 {
		if j := strings.IndexByte(key[i+1:], '}'); j > 0 {
			key = key[i+1 : i+1+j]
		}
	}
	return int(crc16(key)) % 16384
}

// crc16 CRC16-XMODEM（Redis Cluster 用的那张表对应的多项式）
func crc16(s string) uint16 {
	var crc uint16
	for i := 0; i < len(s); i++ {
		crc ^= uint16(s[i]) << 8
		for j := 0; j < 8; j++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// proxyUnsupportedCommands Proxy 集群上未实现或明确受限的命令（华为云 DCS 实例受限使用命令）
var proxyUnsupportedCommands = map[string]struct{}{
	"BZPOPMAX": {}, "BZPOPMIN": {},
}

// proxyAtomicCommands 源→目的类命令：所有键必须同 slot，否则代理无法保证原子性
var proxyAtomicCommands = map[string]struct{}{
	"SMOVE": {}, "RENAME": {}, "RENAMENX": {}, "RPOPLPUSH": {}, "LMOVE": {}, "BLMOVE": {}, "COPY": {},
	"BITOP": {}, "MSETNX": {},
	"SINTERSTORE": {}, "SUNIONSTORE": {}, "SDIFFSTORE": {},
	"ZUNIONSTORE": {}, "ZINTERSTORE": {}, "ZDIFFSTORE": {},
	"PFMERGE": {},
}

// sameSlot 所有键是否落在同一 slot
func sameSlot(keys []string) bool {
	if len(keys) < 2 {
		return true
	}
	first := redisSlot(keys[0])
	for _, k := range keys[1:] {
		if redisSlot(k) != first {
			return false
		}
	}
	return true
}
