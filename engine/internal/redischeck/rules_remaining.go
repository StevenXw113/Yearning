package redischeck

import (
	"strconv"
	"strings"
)

// 规格里最后 4 条规则。
//
// 它们的原始描述都假设「能看到多条命令的上下文」，而当前入口是单命令审核：
// 这里不硬凑，改为「单命令内可观测的等价信号」，判定依据写在各条注释里；
// 唯一依赖多命令上下文的是 MaxTransactionCommands，事实由调用方（CommandCount）传入。

const (
	// deleteManifestKeys 批量删除需要附清单的键数阈值
	deleteManifestKeys = 10
	// expireRoundSeconds TTL 取整点（整小时整数倍）视为集中过期风险
	expireRoundSeconds = 3600
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

	// 2) 批量删除需要清单：键数达到阈值时要求附待删键清单。
	//    MaxDeleteKeys 是硬上限（超了直接拦），这条是流程要求（提示补清单、确认备份）。
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

	// 3) 事务命令数：事务块大小只能由调用方统计（当前入口单命令，恒为 1）。
	//    MULTI 本身已被 TransactionIntegrity 拦；这条面向「将来放开多命令提交」的工单。
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

	// 4) 脚本模板范围：脚本只能操作「显式声明 + 允许前缀内」的键。
	//    numkeys 与实际键数不符、脚本引用 KEYS[i] 越界，都会让未声明的键参与操作，
	//    在集群/代理上直接路由失败，在主从上也可能是绕过键权限的入口。
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
