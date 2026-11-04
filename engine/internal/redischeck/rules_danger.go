package redischeck

import "strings"

// 危险命令与管理命令：默认拦截。只读子命令（CONFIG GET、CLIENT LIST、ACL LIST、
// CLUSTER INFO、MODULE LIST）不在此列，需要单独审核时再配规则。
func init() {
	Register(commandRule{
		name: "RedisForbidDangerous", desc: "禁止 FLUSHALL、FLUSHDB、SHUTDOWN、SWAPDB 等清库、关闭实例或交换数据库的命令",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.ForbidDangerous },
		hit:     forbidTable(dangerousCommands),
	})

	Register(commandRule{
		name: "RedisForbidAdmin", desc: "禁止修改实例配置或连接状态的管理子命令，如 CONFIG SET/REWRITE/RESETSTAT、CLIENT KILL/PAUSE；只读子命令单独审核",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.ForbidAdmin },
		hit: func(_ Config, cmd *Command) string {
			subs, ok := adminWriteSubs[cmd.Name]
			if !ok || !inSet(subs, subArg(cmd, 1)) {
				return ""
			}
			return adminCommands[cmd.Name]
		},
	})

	Register(commandRule{
		name: "RedisForbidDebug", desc: "禁止 DEBUG 命令",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.ForbidDebug },
		hit:     forbidTable(debugCommands),
	})

	Register(commandRule{
		name: "RedisForbidMonitor", desc: "禁止 MONITOR，避免持续监听带来的性能及数据暴露风险",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.ForbidMonitor },
		hit:     forbidTable(monitorCommands),
	})

	Register(commandRule{
		name: "RedisForbidAclChange", desc: "禁止修改用户及权限的 ACL 子命令，如 SETUSER、DELUSER、LOAD",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.ForbidAclChange },
		hit: func(_ Config, cmd *Command) string {
			if cmd.Name != "ACL" || !inSet(aclWriteSubs, subArg(cmd, 1)) {
				return ""
			}
			return "禁止 ACL " + subArg(cmd, 1) + "：修改用户与权限会影响所有连接"
		},
	})

	Register(commandRule{
		name: "RedisForbidModuleChange", desc: "禁止 MODULE LOAD、LOADEX、UNLOAD 等模块变更命令",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.ForbidModuleChange },
		hit: func(_ Config, cmd *Command) string {
			if cmd.Name != "MODULE" || !inSet(moduleWriteSubs, subArg(cmd, 1)) {
				return ""
			}
			return moduleCommands["MODULE"]
		},
	})

	Register(commandRule{
		name: "RedisForbidReplica", desc: "禁止 REPLICAOF、SLAVEOF，避免改变复制关系",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.ForbidReplica },
		hit:     forbidTable(replicaCommands),
	})

	Register(commandRule{
		name: "RedisForbidClusterChange", desc: "禁止修改集群拓扑和 Slot 状态的 CLUSTER 子命令；只读查询子命令单独审核",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.ForbidClusterChange },
		hit: func(_ Config, cmd *Command) string {
			if cmd.Name != "CLUSTER" || !inSet(clusterWriteSubs, subArg(cmd, 1)) {
				return ""
			}
			return "禁止 CLUSTER " + subArg(cmd, 1) + "：修改拓扑与 Slot 状态会影响整个集群"
		},
	})

	Register(commandRule{
		name: "RedisForbidPersistence", desc: "禁止普通业务入口执行 SAVE、BGSAVE、BGREWRITEAOF 等持久化管理命令",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.ForbidPersistence },
		hit:     forbidTable(persistenceCommands),
	})

	Register(commandRule{
		name: "RedisForbidMigration", desc: "禁止 MIGRATE、RESTORE 等迁移或恢复操作，转专项审核",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.ForbidMigration },
		hit:     forbidTable(migrationCommands),
	})

	Register(commandRule{
		name: "RedisForbidMove", desc: "禁止 MOVE，避免跨 DB 迁移数据；SELECT 另按 DB 权限及部署模式检查",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.ForbidMove },
		hit: func(_ Config, cmd *Command) string {
			if cmd.Name != "MOVE" {
				return ""
			}
			return "MOVE 会把键 " + cmd.Arg(1) + " 移到 DB " + cmd.Arg(2) + "，键的归属就此改变"
		},
	})
}

// subArg 取第 i 个参数并大写（子命令归一）
func subArg(cmd *Command, i int) string { return strings.ToUpper(cmd.Arg(i)) }

// inSet 集合里是否有该元素
func inSet(set map[string]struct{}, k string) bool {
	_, ok := set[k]
	return ok
}

// adminWriteSubs CONFIG / CLIENT 中会改变状态的子命令
var adminWriteSubs = map[string]map[string]struct{}{
	"CONFIG": {"SET": {}, "REWRITE": {}, "RESETSTAT": {}},
	"CLIENT": {"KILL": {}, "PAUSE": {}, "UNPAUSE": {}, "NO-EVICT": {}, "NO-TOUCH": {}, "REPLY": {}},
}

// aclWriteSubs ACL 中会改变用户或权限的子命令
var aclWriteSubs = map[string]struct{}{
	"SETUSER": {}, "DELUSER": {}, "LOAD": {}, "SAVE": {},
}

// moduleWriteSubs MODULE 中会改变模块状态的子命令
var moduleWriteSubs = map[string]struct{}{
	"LOAD": {}, "LOADEX": {}, "UNLOAD": {},
}

// clusterWriteSubs CLUSTER 中会改变拓扑或 Slot 状态的子命令
var clusterWriteSubs = map[string]struct{}{
	"SETSLOT": {}, "ADDSLOTS": {}, "DELSLOTS": {}, "ADDSLOTSRANGE": {}, "DELSLOTSRANGE": {},
	"SET-CONFIG-EPOCH": {}, "BUMPEPOCH": {}, "FORGET": {}, "MEET": {}, "REPLICATE": {},
	"RESET": {}, "FAILOVER": {}, "FLUSHSLOTS": {},
}
