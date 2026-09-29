package mongocheck

import (
	"reflect"
	"strings"

	enginev1 "engine/gen/engine/v1"
)

// Config 规则开关的取值快照，由 server 从 enginev1.AuditRole 构造。
//
// 字段是**显式类型**：规则直接读 cfg.ForbidEmptyFilter，拼错名字编译期就报错，
// IDE 里也能跳转。需要靠反射消灭的不是这些字段，而是「proto 字段 → Config 字段」
// 那一层手工映射（mongoRulesFrom 里一行一个）——规则一多必漏，漏了不报错，
// 只是页面勾了不生效。ConfigFromProto 用名字对应关系自动填，由 archguard 护栏核对。
type Config struct {
	// ---- 规则开关与参数：字段名与 AuditRole 的 mongo_* 字段一一对应 ----
	ForbidEmptyFilter      bool
	ForbidDangerous        bool
	ForbidWhere            bool
	ForbidDropCollection   bool
	ForbidSystemCollection bool
	ForbidAdminCommand     bool
	ForbidImmutableID      bool

	// ---- 结构类（批次 4）：命名规范与结构类默认只提示，convertToCapped 除外 ----
	ForbidCappedConvert  bool
	ForbidCollMod        bool
	IndexKeyLimit        int
	IndexNameSpec        string
	CollectionPrefix     string
	MaxCollectionNameLen int

	// ---- 性能类（批次 5）：只提示不拦 ----
	RegexUnanchored     bool
	NegationOperator    bool
	OrClause            bool
	LargeInList         int
	QueryForbidNoFilter bool
	QueryForbidLookup   bool
	QueryForbidNoLimit  bool

	// Levels 规则级别：key 为规则名，value 为 error / warn / observe。
	// 没配（或配了非法值）时取规则自带的 DefaultLevel。
	Levels map[string]string
	// Mode 审核模式：ModeWrite（空值同此）/ ModeQuery
	Mode Mode
}

// ConfigFromProto 从规则集构造 Config。
//
// 按名字对应关系反射填充：AuditRole 里 mongo_* 的字段，会自动填进 Config 的同名字段
// （忽略大小写与下划线）。因此新增一条规则只需要在 Config 里加字段 + 写规则文件，
// 不必再在 server 侧补一行映射；「proto 有而 Config 没有」由
// archguard 的 TestMongoConfigCoversAllSwitches 挡下。
func ConfigFromProto(r *enginev1.AuditRole, mode Mode) Config {
	cfg := Config{Mode: mode}
	if r == nil {
		return cfg
	}
	cfg.Levels = r.GetRuleLevels()

	ref := r.ProtoReflect()
	fds := ref.Descriptor().Fields()
	dst := reflect.ValueOf(&cfg).Elem()
	for i := 0; i < fds.Len(); i++ {
		fd := fds.Get(i)
		if !strings.HasPrefix(strings.ToLower(string(fd.Name())), "mongo") {
			continue
		}
		field := dst.FieldByNameFunc(func(name string) bool {
			return normSwitch(name) == protoFieldKey(string(fd.Name()))
		})
		if !field.IsValid() || !field.CanSet() {
			continue
		}
		val := ref.Get(fd)
		switch field.Kind() {
		case reflect.Bool:
			field.SetBool(val.Bool())
		case reflect.Int:
			field.SetInt(val.Int())
		case reflect.String:
			field.SetString(val.String())
		}
	}
	return cfg
}

// Has 配置里是否存在该开关字段（忽略大小写与下划线）。
// 供 archguard 核对「proto 里的每个 Mongo* 开关都能被取到」。
func (Config) Has(name string) bool {
	return reflect.ValueOf(Config{}).FieldByNameFunc(func(n string) bool {
		return normSwitch(n) == protoFieldKey(name)
	}).IsValid()
}

// protoFieldKey 把 proto 字段名归一化成 Config 字段名：
// mongo_forbid_empty_filter → forbidemptyfilter。
// proto 侧的 mongo_ 前缀在 Config 里不重复写——字段已经在本包里，
// 再带个 Mongo 前缀只是自说自话（规则名常量仍保留完整名字，那是规则集的 key）。
func protoFieldKey(protoName string) string {
	return strings.TrimPrefix(normSwitch(protoName), "mongo")
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

// normSwitch 归一化开关名后比较：Go/proto 侧写 PascalCase，字段名是 snake_case
func normSwitch(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), "_", ""))
}
