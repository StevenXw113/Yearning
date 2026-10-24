package redischeck

import (
	"reflect"
	"strings"

	"engine/api"
)

// Config 规则开关的取值快照，由 server 从 api.AuditRole 构造。
//
// 字段是**显式类型**：规则直接读 cfg.ForbidDangerous，拼错名字编译期就报错。
// 需要靠反射消灭的不是这些字段，而是「AuditRole 字段 → Config 字段」那一层手工映射
// （一行一个、规则一多必漏，漏了不报错，只是页面勾了不生效）。
// ConfigFromAPI 按名字对应关系自动填，由 archguard 护栏核对。
type Config struct {
	// ---- 安全类（默认 error）----
	// ForbidDangerous 清库/关机/搬键等影响整个实例的命令
	ForbidDangerous bool
	// ForbidAdmin 运维管理类（CONFIG / DEBUG / MODULE / MONITOR / CLIENT / 落盘）
	ForbidAdmin bool
	// ForbidScript 服务端脚本（EVAL / EVALSHA / SCRIPT / FUNCTION / FCALL）
	ForbidScript bool
	// ForbidReplica 主从关系（REPLICAOF / SLAVEOF）
	ForbidReplica bool
	// ForbidKeysCommand KEYS：O(N) 且阻塞单线程实例
	ForbidKeysCommand bool

	// ---- 结构类 ----
	ForbidRename bool
	// KeyPrefix key 命名前缀规范，空 = 不检查
	KeyPrefix string
	// MaxKeyLen key 名长度上限，0 = 不限制
	MaxKeyLen int
	// ForbidMove MOVE / SELECT：跨库副作用
	ForbidMove bool

	// ---- 性能 / 风险类（默认只提示）----
	// MaxAffectKeys 单条命令允许影响的键数上限，0 = 不限制
	MaxAffectKeys int
	// MaxValueLen 单次写入 value 的字节数上限，0 = 不限制
	MaxValueLen int
	// RequireTTL SET 必须带过期时间
	RequireTTL bool
	// ForbidPersist 禁止 PERSIST（去掉 TTL）
	ForbidPersist bool
	// ScanRequireMatch SCAN 必须带 MATCH
	ScanRequireMatch bool
	// ForbidBlocking 禁止阻塞命令（BLPOP / BRPOP / BLMOVE / WAIT 等）
	ForbidBlocking bool

	// Levels 规则级别：key 为规则名，value 为 error / warn / observe。
	// 没配（或配了非法值）时取规则自带的 DefaultLevel。
	Levels map[string]string
	// Mode 审核模式：ModeWrite（空值同此）/ ModeQuery
	Mode Mode
}

// ConfigFromAPI 从规则集构造 Config。
//
// 按名字对应关系反射填充：AuditRole 里 redis_* 的字段会自动填进 Config 的同名字段
// （忽略大小写与下划线）。因此新增一条规则只需在 Config 里加字段 + 写规则文件，
// 不必再在 server 侧补映射；「AuditRole 有而 Config 没有」由 archguard 的
// TestRedisConfigCoversAllSwitches 挡下。
func ConfigFromAPI(r *api.AuditRole, mode Mode) Config {
	cfg := Config{Mode: mode}
	if r == nil {
		return cfg
	}
	cfg.Levels = r.RuleLevels

	src := reflect.ValueOf(r).Elem()
	dst := reflect.ValueOf(&cfg).Elem()
	st := src.Type()
	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		if !strings.HasPrefix(strings.ToLower(f.Name), "redis") {
			continue
		}
		field := dst.FieldByNameFunc(func(name string) bool {
			return normSwitch(name) == protoFieldKey(f.Name)
		})
		if !field.IsValid() || !field.CanSet() {
			continue
		}
		switch field.Kind() {
		case reflect.Bool:
			field.SetBool(src.Field(i).Bool())
		case reflect.Int, reflect.Int32, reflect.Int64:
			field.SetInt(src.Field(i).Int())
		case reflect.String:
			field.SetString(src.Field(i).String())
		}
	}
	return cfg
}

// Has 配置里是否存在该开关字段（忽略大小写与下划线）。
// 供 archguard 核对「AuditRole 里的每个 Redis* 开关都能被取到」。
func (Config) Has(name string) bool {
	return reflect.ValueOf(Config{}).FieldByNameFunc(func(n string) bool {
		return normSwitch(n) == protoFieldKey(name)
	}).IsValid()
}

// protoFieldKey 把 AuditRole 字段名归一化成 Config 字段名：
// RedisForbidDangerous → forbiddangerous。
// AuditRole 的 Redis 前缀字段在 Config 里不重复写——字段已经在本包里；
// 规则名常量仍保留完整名字，那是规则集的 key。
func protoFieldKey(protoName string) string {
	return strings.TrimPrefix(normSwitch(protoName), "redis")
}

// Level 取规则的生效级别：RuleLevels 里配了且合法就用配置值，否则用规则自带的默认级别。
// 默认级别让「安全类拦截、性能类只提示」这件事不必依赖前端预先写级别。
func (c Config) Level(r Rule) Level {
	if l, ok := c.Levels[r.Name()]; ok {
		if lv := Level(strings.ToLower(strings.TrimSpace(l))); ValidLevel(lv) {
			return lv
		}
	}
	return r.DefaultLevel()
}

// queryMode 是否查询模式（大小写与空白不敏感）
func (c Config) queryMode() bool {
	return strings.EqualFold(strings.TrimSpace(string(c.Mode)), string(ModeQuery))
}

// mode 归一化后的模式：空值与无法识别的值都按变更审核处理（保持历史行为）
func (c Config) mode() Mode {
	if c.queryMode() {
		return ModeQuery
	}
	return ModeWrite
}

// normSwitch 归一化开关名后比较：AuditRole 字段写 PascalCase(线缆名为 snake_case,见 json tag)
func normSwitch(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), "_", ""))
}
