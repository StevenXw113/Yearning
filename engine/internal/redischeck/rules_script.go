package redischeck

import "strings"

// 事务、脚本与特殊操作：脚本执行与管理、WATCH、阻塞等待、订阅与消费状态变更。
func init() {
	Register(commandRule{
		name: "RedisForbidScript", desc: "禁止未登记的 EVAL、EVALSHA、EVAL_RO、EVALSHA_RO、FCALL、FCALL_RO 等脚本或函数执行命令",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.ForbidScript },
		hit:     forbidTable(scriptCommands),
	})

	Register(commandRule{
		name: "RedisForbidScriptManagement", desc: "禁止 SCRIPT 及 FUNCTION 中修改、加载、清理脚本或函数状态的子命令；只读子命令单独审核",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.ForbidScriptManagement },
		hit: func(_ Config, cmd *Command) string {
			if cmd.Name != "SCRIPT" && cmd.Name != "FUNCTION" {
				return ""
			}
			sub := subArg(cmd, 1)
			if !inSet(scriptManagementCommands, sub) {
				return ""
			}
			return "禁止 " + cmd.Name + " " + sub + "：脚本与函数的状态变更必须走专项发布流程"
		},
	})

	Register(commandRule{
		name: "RedisForbidWatch", desc: "普通命令入口禁止 WATCH、UNWATCH；仅在明确支持完整会话语义时开放",
		cat: CategorySecurity, modes: writeOnly, level: LevelError,
		enabled: func(c Config) bool { return c.ForbidWatch },
		hit: func(_ Config, cmd *Command) string {
			switch cmd.Name {
			case "WATCH", "UNWATCH":
				return "禁止 " + cmd.Name + "：普通命令入口不保证完整的会话语义，乐观锁可能静默失效"
			}
			return ""
		},
	})

	Register(commandRule{
		name: "RedisForbidBlockingCommand", desc: "普通命令入口禁止列表阻塞、Stream 阻塞等等待型操作；专项场景另行配置",
		cat: CategorySecurity, modes: writeOnly, level: LevelError,
		enabled: func(c Config) bool { return c.ForbidBlockingCommand },
		hit: func(_ Config, cmd *Command) string {
			if msg := blockingCommands[cmd.Name]; msg != "" {
				return msg
			}
			// XREAD / XREADGROUP 只有带 BLOCK 时才是阻塞操作
			if cmd.Name == "XREAD" || cmd.Name == "XREADGROUP" {
				if cmd.HasAnyArg("BLOCK") {
					return "禁止 " + cmd.Name + " 带 BLOCK：会长时间占用连接"
				}
			}
			return ""
		},
	})

	Register(commandRule{
		name: "RedisForbidSubscription", desc: "禁止 SUBSCRIBE、PSUBSCRIBE、SSUBSCRIBE 等持续订阅操作",
		cat: CategorySecurity, modes: writeOnly, level: LevelError,
		enabled: func(c Config) bool { return c.ForbidSubscription },
		hit:     forbidTable(subscriptionCommands),
	})

	Register(commandRule{
		name: "RedisStreamStateChange", desc: "消费组变更、消息确认、认领等操作会改变消费状态，执行时提示",
		cat: CategoryStructure, modes: writeOnly, level: LevelWarn,
		enabled: func(c Config) bool { return c.StreamStateChange },
		hit: func(_ Config, cmd *Command) string {
			switch cmd.Name {
			case "XGROUP":
				if sub := subArg(cmd, 1); sub != "" && !strings.EqualFold(sub, "HELP") {
					return "XGROUP " + sub + " 会改变消费组状态，请确认消费进度"
				}
			case "XACK", "XCLAIM", "XAUTOCLAIM", "XSETID":
				return cmd.Name + " 会改变 Stream 的消费状态（确认/认领/位点），重复执行可能丢消息"
			}
			return ""
		},
	})
}
