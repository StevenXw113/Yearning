package redischeck

import "strings"

func init() {
	Register(forbidRename{})
	Register(keyPrefix{})
	Register(maxKeyLen{})
	Register(forbidMove{})
}

// writeModes 结构类只对变更生效：查询命令不改键名，检查它没有意义
var writeModes = []Mode{ModeWrite}

// forbidRename RENAME 会静默覆盖目标键——目标键的旧值就此丢失，且没有回滚余地
type forbidRename struct{}

func (forbidRename) Name() string        { return "RedisForbidRename" }
func (forbidRename) Category() Category  { return CategoryStructure }
func (forbidRename) Modes() []Mode       { return writeModes }
func (forbidRename) DefaultLevel() Level { return LevelWarn }
func (forbidRename) Desc() string {
	return "禁止 RENAME / RENAMENX：会静默覆盖目标键，旧值无法找回"
}
func (r forbidRename) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.ForbidRename {
		return nil
	}
	switch cmd.Name {
	case "RENAME", "RENAMENX":
		return finding(r, cfg, "RENAME 会覆盖目标键 "+cmd.Arg(2)+" 的原有数据，请确认该键可被覆盖")
	}
	return nil
}

// keyPrefix 键名前缀规范：团队里靠前缀区分业务，配了才检查
type keyPrefix struct{}

func (keyPrefix) Name() string        { return "RedisKeyPrefix" }
func (keyPrefix) Category() Category  { return CategoryStructure }
func (keyPrefix) Modes() []Mode       { return writeModes }
func (keyPrefix) DefaultLevel() Level { return LevelObserve }
func (keyPrefix) Desc() string {
	return "键名必须以指定前缀开头（如 app:），空表示不检查"
}
func (r keyPrefix) Check(cfg Config, cmd *Command) []Finding {
	if cfg.KeyPrefix == "" || len(cmd.Keys) == 0 {
		return nil
	}
	var bad []string
	for _, k := range cmd.Keys {
		if !strings.HasPrefix(k, cfg.KeyPrefix) {
			bad = append(bad, k)
		}
	}
	if len(bad) == 0 {
		return nil
	}
	return finding(r, cfg, "键名应以 "+cfg.KeyPrefix+" 开头："+strings.Join(bad, "、"))
}

// maxKeyLen 键名长度上限：超长的键名既难排查，也吃内存
type maxKeyLen struct{}

func (maxKeyLen) Name() string        { return "RedisMaxKeyLen" }
func (maxKeyLen) Category() Category  { return CategoryStructure }
func (maxKeyLen) Modes() []Mode       { return writeModes }
func (maxKeyLen) DefaultLevel() Level { return LevelWarn }
func (maxKeyLen) Desc() string {
	return "键名长度上限（字节），0 表示不限制"
}
func (r maxKeyLen) Check(cfg Config, cmd *Command) []Finding {
	if cfg.MaxKeyLen <= 0 || len(cmd.Keys) == 0 {
		return nil
	}
	var bad []string
	for _, k := range cmd.Keys {
		// 按字节算：Redis 的键是字节串，与 maxmemory 的口径一致
		if len(k) > cfg.MaxKeyLen {
			bad = append(bad, k)
		}
	}
	if len(bad) == 0 {
		return nil
	}
	return finding(r, cfg, "键名超过 "+itoa(cfg.MaxKeyLen)+" 字节："+strings.Join(bad, "、"))
}

// forbidMove MOVE / SELECT：跨库操作会让「这个键在哪个库」变得难追踪
type forbidMove struct{}

func (forbidMove) Name() string        { return "RedisForbidMove" }
func (forbidMove) Category() Category  { return CategoryStructure }
func (forbidMove) Modes() []Mode       { return writeModes }
func (forbidMove) DefaultLevel() Level { return LevelWarn }
func (forbidMove) Desc() string {
	return "禁止 MOVE / SELECT：跨库操作会让键的归属难以追踪"
}
func (r forbidMove) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.ForbidMove {
		return nil
	}
	switch cmd.Name {
	case "MOVE":
		return finding(r, cfg, "MOVE 会把键移到另一个逻辑库，请确认目标库没有同名键、且业务侧知道这次搬迁")
	case "SELECT":
		return finding(r, cfg, "SELECT 切换逻辑库：单条工单里切换库会让后续命令的目标库变得不明确")
	}
	return nil
}
