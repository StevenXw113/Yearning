package redischeck

func init() {
	Register(forbidDangerous{})
	Register(forbidAdmin{})
	Register(forbidScript{})
	Register(forbidReplica{})
	Register(forbidKeysCommand{})
}

// finding 用规则自带的 Name 与生效级别造一条命中。
// 级别取 cfg.Level(r)：规则集里配了就用配置值，否则用规则自带的 DefaultLevel。
func finding(r Rule, cfg Config, msg string) []Finding {
	return []Finding{{Rule: r.Name(), Level: cfg.Level(r), Message: msg}}
}

// securityModes 安全类规则在变更与查询两种模式下都生效——
// 查询页同样不该出现 FLUSHALL 之类。
var securityModes = []Mode{ModeWrite, ModeQuery}

// forbidDangerous 影响面是整个实例的命令
type forbidDangerous struct{}

func (forbidDangerous) Name() string       { return "RedisForbidDangerous" }
func (forbidDangerous) Category() Category { return CategorySecurity }
func (forbidDangerous) Modes() []Mode      { return securityModes }
func (forbidDangerous) DefaultLevel() Level {
	return LevelError
}
func (forbidDangerous) Desc() string {
	return "禁止 FLUSHALL / FLUSHDB / SHUTDOWN / SWAPDB / MIGRATE 等影响整个实例的命令"
}
func (r forbidDangerous) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.ForbidDangerous {
		return nil
	}
	msg, bad := dangerousCommands[cmd.Name]
	if !bad {
		return nil
	}
	return finding(r, cfg, msg)
}

// forbidAdmin 运维管理类：会改变实例行为或泄露数据
type forbidAdmin struct{}

func (forbidAdmin) Name() string       { return "RedisForbidAdmin" }
func (forbidAdmin) Category() Category { return CategorySecurity }
func (forbidAdmin) Modes() []Mode      { return securityModes }
func (forbidAdmin) DefaultLevel() Level {
	return LevelError
}
func (forbidAdmin) Desc() string {
	return "禁止 CONFIG / DEBUG / MODULE / MONITOR / CLIENT 等运维管理命令"
}
func (r forbidAdmin) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.ForbidAdmin {
		return nil
	}
	msg, bad := adminCommands[cmd.Name]
	if !bad {
		return nil
	}
	return finding(r, cfg, msg)
}

// forbidScript 服务端脚本：与 Mongo 的 $where / eval 同类
type forbidScript struct{}

func (forbidScript) Name() string       { return "RedisForbidScript" }
func (forbidScript) Category() Category { return CategorySecurity }
func (forbidScript) Modes() []Mode      { return securityModes }
func (forbidScript) DefaultLevel() Level {
	return LevelError
}
func (forbidScript) Desc() string {
	return "禁止 EVAL / EVALSHA / SCRIPT / FUNCTION / FCALL（服务端执行脚本）"
}
func (r forbidScript) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.ForbidScript {
		return nil
	}
	msg, bad := scriptCommands[cmd.Name]
	if !bad {
		return nil
	}
	return finding(r, cfg, msg)
}

// forbidReplica 主从关系：改这个会直接影响可用性
type forbidReplica struct{}

func (forbidReplica) Name() string       { return "RedisForbidReplica" }
func (forbidReplica) Category() Category { return CategorySecurity }
func (forbidReplica) Modes() []Mode      { return securityModes }
func (forbidReplica) DefaultLevel() Level {
	return LevelError
}
func (forbidReplica) Desc() string {
	return "禁止 REPLICAOF / SLAVEOF（改变主从关系）"
}
func (r forbidReplica) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.ForbidReplica {
		return nil
	}
	msg, bad := replicaCommands[cmd.Name]
	if !bad {
		return nil
	}
	return finding(r, cfg, msg)
}

// forbidKeysCommand KEYS 是 O(N) 且阻塞单线程实例：
// 大键空间下它本身就是一次故障，提示改用 SCAN。
type forbidKeysCommand struct{}

func (forbidKeysCommand) Name() string       { return "RedisForbidKeysCommand" }
func (forbidKeysCommand) Category() Category { return CategorySecurity }
func (forbidKeysCommand) Modes() []Mode      { return securityModes }
func (forbidKeysCommand) DefaultLevel() Level {
	return LevelError
}
func (forbidKeysCommand) Desc() string {
	return "禁止 KEYS：它会遍历整个键空间并阻塞实例，请改用 SCAN"
}
func (r forbidKeysCommand) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.ForbidKeysCommand || cmd.Name != "KEYS" {
		return nil
	}
	return finding(r, cfg, "禁止 KEYS：它会遍历整个键空间并阻塞实例，请改用 SCAN 游标遍历")
}
