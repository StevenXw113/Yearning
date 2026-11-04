package redischeck

import "strings"

// 删除与大 Key：删除动作本身的风险、键数上限、受保护键与范围删除。
func init() {
	Register(commandRule{
		name: "RedisDeleteKey", desc: "执行 DEL、UNLINK 时提示数据删除风险",
		cat: CategoryStructure, modes: writeOnly, level: LevelWarn,
		enabled: func(c Config) bool { return c.DeleteKey },
		hit: func(_ Config, cmd *Command) string {
			switch cmd.Name {
			case "DEL", "UNLINK":
				return cmd.Name + " 会永久删除 " + itoa(len(cmd.Keys)) + " 个键，删除不可回滚"
			case "GETDEL":
				return "GETDEL 读取后立即删除键 " + cmd.Arg(1)
			}
			return ""
		},
	})

	Register(commandRule{
		name: "RedisMaxDeleteKeys", desc: "单条删除命令键数上限，0 表示不限制；超过上限转批量删除审核",
		cat: CategoryPerformance, modes: writeOnly, level: LevelError,
		enabled: func(c Config) bool { return c.MaxDeleteKeys > 0 },
		hit: func(cfg Config, cmd *Command) string {
			switch cmd.Name {
			case "DEL", "UNLINK":
			default:
				return ""
			}
			if len(cmd.Keys) <= cfg.MaxDeleteKeys {
				return ""
			}
			return cmd.Name + " 一次删除 " + itoa(len(cmd.Keys)) + " 个键，超过上限 " +
				itoa(cfg.MaxDeleteKeys) + "，请转批量删除审核"
		},
	})

	Register(commandRule{
		name: "RedisProtectedKeyDelete", desc: "禁止删除匹配保护规则的锁、幂等键、核心状态及其他关键业务键；多个规则用逗号分隔",
		cat: CategorySecurity, modes: writeOnly, level: LevelError,
		enabled: func(c Config) bool { return c.ProtectedKeyDelete != "" },
		hit: func(cfg Config, cmd *Command) string {
			switch cmd.Name {
			case "DEL", "UNLINK", "GETDEL":
			default:
				return ""
			}
			patterns := splitPatterns(cfg.ProtectedKeyDelete)
			var bad []string
			for _, k := range cmd.Keys {
				if matchAny(patterns, k) {
					bad = append(bad, k)
				}
			}
			if len(bad) == 0 {
				return ""
			}
			return "键 " + strings.Join(bad, "、") + " 属于受保护键，不允许删除"
		},
	})

	Register(commandRule{
		name: "RedisLargeRangeDelete", desc: "集合范围删除、列表裁剪及 Stream 裁剪等操作，影响元素数量未知或超过阈值时提示",
		cat: CategoryPerformance, modes: writeOnly, level: LevelWarn,
		enabled: func(c Config) bool { return c.LargeRangeDelete },
		hit: func(_ Config, cmd *Command) string {
			switch cmd.Name {
			case "LTRIM", "LREM", "ZREMRANGEBYRANK", "ZREMRANGEBYSCORE", "ZREMRANGEBYLEX":
				return cmd.Name + " 会按范围删除元素，影响数量未知时建议先用范围查询确认"
			case "XTRIM", "XDEL":
				return cmd.Name + " 会按范围裁剪/删除 Stream 消息，请确认消费进度已推进"
			}
			return ""
		},
	})
}
