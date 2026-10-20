package redischeck

import "strconv"

func init() {
	Register(maxAffectKeys{})
	Register(maxValueLen{})
	Register(requireTTL{})
	Register(forbidPersist{})
	Register(scanRequireMatch{})
	Register(forbidBlocking{})
}

// itoa 小工具，省得每处都 import strconv
func itoa(n int) string { return strconv.Itoa(n) }

// maxAffectKeys 单条命令影响的键数上限：DEL k1 … k10000 这种属于批量操作，
// 出错时影响面大、回滚代价高，应该分批。
type maxAffectKeys struct{}

func (maxAffectKeys) Name() string        { return "RedisMaxAffectKeys" }
func (maxAffectKeys) Category() Category  { return CategoryPerformance }
func (maxAffectKeys) Modes() []Mode       { return writeModes }
func (maxAffectKeys) DefaultLevel() Level { return LevelWarn }
func (maxAffectKeys) Desc() string {
	return "单条命令允许影响的键数上限，0 表示不限制"
}
func (r maxAffectKeys) Check(cfg Config, cmd *Command) []Finding {
	if cfg.MaxAffectKeys <= 0 || cmd.KeyCount() <= cfg.MaxAffectKeys {
		return nil
	}
	return finding(r, cfg, "本次命令涉及 "+itoa(cmd.KeyCount())+" 个键，超过上限 "+
		itoa(cfg.MaxAffectKeys)+"，请分批提交")
}

// maxValueLen 单次写入的 value 大小上限。
//
// 判定是**近似**的：跳过第 1 个参数（键名，另有 RedisMaxKeyLen 管），
// 取其余参数里最长的那个。这样对 SET / APPEND / LPUSH / HSET / ZADD 都够用，
// 又不必为每条命令维护一张「第几个参数才是值」的位置表。
type maxValueLen struct{}

func (maxValueLen) Name() string        { return "RedisMaxValueLen" }
func (maxValueLen) Category() Category  { return CategoryPerformance }
func (maxValueLen) Modes() []Mode       { return writeModes }
func (maxValueLen) DefaultLevel() Level { return LevelWarn }
func (maxValueLen) Desc() string {
	return "单次写入值的大小上限（字节），0 表示不限制"
}
func (r maxValueLen) Check(cfg Config, cmd *Command) []Finding {
	if cfg.MaxValueLen <= 0 {
		return nil
	}
	longest := ""
	for _, a := range cmd.ArgsFrom(2) {
		if len(a) > len(longest) {
			longest = a
		}
	}
	if len(longest) <= cfg.MaxValueLen {
		return nil
	}
	return finding(r, cfg, "单个值 "+itoa(len(longest))+" 字节，超过上限 "+itoa(cfg.MaxValueLen)+" 字节")
}

// requireTTL SET 不带过期时间的键会一直留在内存里，是 Redis 内存增长的常见来源
type requireTTL struct{}

func (requireTTL) Name() string        { return "RedisRequireTTL" }
func (requireTTL) Category() Category  { return CategoryPerformance }
func (requireTTL) Modes() []Mode       { return writeModes }
func (requireTTL) DefaultLevel() Level { return LevelObserve }
func (requireTTL) Desc() string {
	return "SET 必须带过期时间（EX / PX / EXAT），避免键无上限增长"
}
func (r requireTTL) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.RequireTTL || cmd.Name != "SET" {
		return nil
	}
	if cmd.HasAnyArg("EX", "PX", "EXAT", "PXAT", "KEEPTTL") {
		return nil
	}
	return finding(r, cfg, "SET 未设置过期时间，键会常驻内存；如确实需要请改用 EX 0 之外的显式声明")
}

// forbidPersist 去掉键的 TTL：与 RequireTTL 相对的另一半
type forbidPersist struct{}

func (forbidPersist) Name() string        { return "RedisForbidPersist" }
func (forbidPersist) Category() Category  { return CategoryPerformance }
func (forbidPersist) Modes() []Mode       { return writeModes }
func (forbidPersist) DefaultLevel() Level { return LevelObserve }
func (forbidPersist) Desc() string {
	return "禁止 PERSIST：会把键的过期时间去掉，变成常驻内存"
}
func (r forbidPersist) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.ForbidPersist || cmd.Name != "PERSIST" {
		return nil
	}
	return finding(r, cfg, "PERSIST 会移除键 "+cmd.Arg(1)+" 的过期时间，使其常驻内存")
}

// scanRequireMatch 不带 MATCH 的 SCAN 等同于全量遍历，只是不阻塞而已
type scanRequireMatch struct{}

func (scanRequireMatch) Name() string        { return "RedisScanRequireMatch" }
func (scanRequireMatch) Category() Category  { return CategoryPerformance }
func (scanRequireMatch) Modes() []Mode       { return []Mode{ModeWrite, ModeQuery} }
func (scanRequireMatch) DefaultLevel() Level { return LevelObserve }
func (scanRequireMatch) Desc() string {
	return "SCAN 应带 MATCH 限定范围，否则等同于全量遍历"
}
func (r scanRequireMatch) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.ScanRequireMatch || cmd.Name != "SCAN" {
		return nil
	}
	if cmd.HasAnyArg("MATCH") {
		return nil
	}
	return finding(r, cfg, "SCAN 未带 MATCH，会遍历整个键空间，建议加上前缀限定范围")
}

// forbidBlocking 阻塞命令会长时间占用连接：在工单/查询这种同步等待的链路里，
// 它带来的通常是超时而不是结果。
type forbidBlocking struct{}

func (forbidBlocking) Name() string        { return "RedisForbidBlocking" }
func (forbidBlocking) Category() Category  { return CategoryPerformance }
func (forbidBlocking) Modes() []Mode       { return []Mode{ModeWrite, ModeQuery} }
func (forbidBlocking) DefaultLevel() Level { return LevelWarn }
func (forbidBlocking) Desc() string {
	return "禁止阻塞命令（BLPOP / BRPOP / BLMOVE / WAIT / SUBSCRIBE 等）"
}
func (r forbidBlocking) Check(cfg Config, cmd *Command) []Finding {
	if !cfg.ForbidBlocking {
		return nil
	}
	msg, bad := blockingCommands[cmd.Name]
	if !bad {
		return nil
	}
	return finding(r, cfg, msg)
}
