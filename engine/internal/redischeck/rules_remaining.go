package redischeck

import (
	"strconv"
	"strings"
)

// 规格里最后 4 条：原始描述假设多命令上下文，这里改用单命令内可观测的等价信号。

const (
	deleteManifestKeys = 10   // 删除超过它要求附清单
	expireRoundSeconds = 3600 // TTL 取整小时视为集中过期风险
)

func init() {
	// 1) 批量过期集中：单条命令看不到「同一批键」，但 TTL 取整点是集中过期的直接诱因——
	//    一批键同时写入、同时失效，缓存雪崩就是这么来的。
	Register(commandRule{
		name: "RedisBatchExpireConcentration", desc: "TTL 取整点（整小时）会让同类键在同一时刻集中失效，建议加随机抖动",
		cat: CategoryPerformance, modes: writeOnly, level: LevelWarn,
		enabled: func(c Config) bool { return c.BatchExpireConcentration },
		hit: func(_ Config, cmd *Command) string {
			sec, ok := expireSeconds(cmd)
			if !ok || sec < expireRoundSeconds || sec%expireRoundSeconds != 0 {
				return ""
			}
			return cmd.Name + " 的 TTL 是 " + itoa(int(sec)) + " 秒（整小时）：" +
				"同类键在同一时刻批量写入会集中失效，建议加随机抖动（如 3600±300）"
		},
	})

	// 2) 批量删除附清单：与 MaxDeleteKeys（硬上限）互补，这条只是流程提示。
	Register(commandRule{
		name: "RedisDeleteManifestRequired", desc: "批量删除需要附待删键清单并确认已备份",
		cat: CategoryStructure, modes: writeOnly, level: LevelWarn,
		enabled: func(c Config) bool { return c.DeleteManifestRequired },
		hit: func(_ Config, cmd *Command) string {
			switch cmd.Name {
			case "DEL", "UNLINK":
			default:
				return ""
			}
			if len(cmd.Keys) < deleteManifestKeys {
				return ""
			}
			return cmd.Name + " 一次删除 " + itoa(len(cmd.Keys)) + " 个键，" +
				"请在备注里附上待删键清单并确认已备份"
		},
	})

	// 3) 事务命令数：CommandCount 由调用方给（单命令入口恒为 1）。
	Register(commandRule{
		name: "RedisMaxTransactionCommands", desc: "单次事务（提交）内的命令数上限，0 表示不限制",
		cat: CategoryPerformance, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.MaxTransactionCommands > 0 },
		hit: func(cfg Config, cmd *Command) string {
			if cmd.Kind == KindQuery || cfg.CommandCount <= cfg.MaxTransactionCommands {
				return ""
			}
			return "本次提交包含 " + itoa(cfg.CommandCount) + " 条命令，超过单次事务上限 " +
				itoa(cfg.MaxTransactionCommands) + "，请拆分提交"
		},
	})

	// 4) 脚本只能碰声明过且在允许前缀内的键（未声明的键在集群下无法路由）。
	Register(commandRule{
		name: "RedisScriptTemplateScope", desc: "脚本只能访问显式声明（KEYS 下标不越界）且在允许前缀内的键",
		cat: CategorySecurity, modes: allModes, level: LevelError,
		enabled: func(c Config) bool { return c.ScriptTemplateScope },
		hit: func(cfg Config, cmd *Command) string {
			script, nkeys, ok := scriptAndKeys(cmd)
			if !ok {
				return ""
			}
			if nkeys != len(cmd.Keys) {
				return cmd.Name + " 声明了 numkeys=" + itoa(nkeys) + "，实际带了 " +
					itoa(len(cmd.Keys)) + " 个键，键声明不完整"
			}
			if maxIdx := maxKeysIndex(script); maxIdx >= nkeys {
				return cmd.Name + " 的脚本引用了 KEYS[" + itoa(maxIdx) + "]，超过声明的 numkeys=" +
					itoa(nkeys) + "：未声明的键无法路由，也是绕过键权限的入口"
			}
			for _, k := range cmd.Keys {
				if !keyPrefixAllowed(cfg.KeyPrefix, k) {
					return "键 " + k + " 不在允许的前缀范围内，脚本不得访问"
				}
			}
			return ""
		},
	})
}

// scriptAndKeys 取脚本内容与 numkeys；非脚本命令返回 ok=false。
// EVALSHA/FCALL 只带 sha 或函数名，没有脚本文本，只能校验键数。
func scriptAndKeys(cmd *Command) (script string, nkeys int, ok bool) {
	switch cmd.Name {
	case "EVAL", "EVAL_RO":
		if cmd.ArgCount() < 3 {
			return "", 0, false
		}
		n, ok := cmd.IntArg(2)
		if !ok {
			return "", 0, false
		}
		return cmd.Arg(1), int(n), true
	case "EVALSHA", "EVALSHA_RO", "FCALL", "FCALL_RO":
		if cmd.ArgCount() < 3 {
			return "", 0, false
		}
		n, ok := cmd.IntArg(2)
		if !ok {
			return "", 0, false
		}
		return "", int(n), true
	}
	return "", 0, false
}

// maxKeysIndex 脚本里出现的最大 KEYS[n] 下标；没有引用返回 -1。
func maxKeysIndex(script string) int {
	if script == "" {
		return -1
	}
	max := -1
	upper := strings.ToUpper(script)
	for i := 0; i < len(upper); {
		j := strings.Index(upper[i:], "KEYS[")
		if j < 0 {
			break
		}
		start := i + j + len("KEYS[")
		end := strings.IndexByte(script[start:], ']')
		if end < 0 {
			break
		}
		if n, err := strconv.Atoi(strings.TrimSpace(script[start : start+end])); err == nil && n > max {
			max = n
		}
		i = start + end
	}
	return max
}
