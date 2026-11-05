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
//
// 多参数规则统一压在一个字段里（例如 DbScope 写成 "0-15"），
// 因为 AuditRole 的 Redis* 开关与规则必须一一对应，一条规则只给一个开关。
type Config struct {
	// ---- 基础校验与权限 ----
	// CommandSyntax 校验命令、子命令、参数数量与选项组合
	CommandSyntax bool
	// UnknownCommand 禁止未登记语义的命令（含模块命令与厂商扩展）
	UnknownCommand bool
	// KeyExtraction 必须能完整识别源键与目标键
	KeyExtraction bool
	// KeyPrefix 键名前缀规范，空 = 不检查
	KeyPrefix string
	// MaxKeyLen 键名长度上限（字节），0 = 不限制
	MaxKeyLen int
	// ForbiddenKeyPattern 受保护键的匹配规则，多个用逗号分隔，空 = 不检查
	ForbiddenKeyPattern string
	// SensitiveKeyAccess 敏感键的匹配规则，多个用逗号分隔，空 = 不检查
	SensitiveKeyAccess string
	// DbScope 允许访问的逻辑库范围，如 "0-15"，空 = 不检查
	DbScope string
	// ForbidConnectionControl 禁止 AUTH / HELLO / QUIT / RESET
	ForbidConnectionControl bool

	// ---- 危险命令与管理命令 ----
	ForbidDangerous     bool
	ForbidAdmin         bool
	ForbidDebug         bool
	ForbidMonitor       bool
	ForbidAclChange     bool
	ForbidModuleChange  bool
	ForbidReplica       bool
	ForbidClusterChange bool
	ForbidPersistence   bool
	ForbidMigration     bool
	ForbidMove          bool

	// ---- 读取与扫描 ----
	ForbidKeysCommand bool
	ForbidRandomKey   bool
	ScanMatchRequired bool
	// MaxScanCount SCAN 系命令的 COUNT 上限，0 = 不限制
	MaxScanCount int
	// FullCollectionRead 禁止一次性读取整个集合
	FullCollectionRead bool
	// MaxReadElements 单次范围/随机读取的元素数上限，0 = 不限制
	MaxReadElements int
	// MaxReadKeys 单条批量读取命令的键数上限，0 = 不限制
	MaxReadKeys int
	// ExpensiveRead 复杂读取（SORT / 集合交并差）提示
	ExpensiveRead bool

	// ---- 写入与数据变更 ----
	// MaxValueSize 单个写入值的字节数上限，0 = 不限制
	MaxValueSize int
	// MaxCommandPayload 单条命令参数总字节数上限，0 = 不限制
	MaxCommandPayload int
	// MaxWriteKeys 单条写入命令涉及的键数上限，0 = 不限制
	MaxWriteKeys int
	// MaxWriteElements 单条命令新增/修改的元素数上限，0 = 不限制
	MaxWriteElements int
	// NonIdempotentWrite 非幂等写入提示
	NonIdempotentWrite bool
	// LargeOffsetWrite 按偏移写入的预计对象大小上限（字节），0 = 不限制
	LargeOffsetWrite int
	ForbidRename     bool
	CopyOverwrite    bool
	StoreOperation   bool

	// ---- TTL 与缓存规范 ----
	RequireTTL bool
	// TtlLossOnOverwrite 覆盖写入可能清除原 TTL 时提示
	TtlLossOnOverwrite bool
	ForbidPersist      bool
	// MinTTL 过期时间下限（秒），0 = 不检查
	MinTTL int
	// MaxTTL 过期时间上限（秒），0 = 不检查
	MaxTTL int
	// ImmediateExpire 立即过期（等同于删除）提示
	ImmediateExpire bool

	// ---- 删除与大 Key ----
	DeleteKey bool
	// MaxDeleteKeys 单条删除命令的键数上限，0 = 不限制
	MaxDeleteKeys int
	// ProtectedKeyDelete 禁止删除的键匹配规则，多个用逗号分隔，空 = 不检查
	ProtectedKeyDelete string
	// LargeRangeDelete 范围删除/裁剪提示
	LargeRangeDelete bool

	// ---- 事务、脚本与特殊操作 ----
	ForbidScript           bool
	ForbidScriptManagement bool
	ForbidWatch            bool
	ForbidBlockingCommand  bool
	ForbidSubscription     bool
	StreamStateChange      bool
	TransactionIntegrity   bool

	// ---- Cluster / Proxy 拓扑 ----
	// 这些开关只在对应部署形态下生效（见 Deployment），形态未知时一律不触发。
	ClusterDbZero          bool
	ClusterCrossSlot       bool
	ClusterScanScope       bool
	ClusterTransactionSlot bool
	ClusterScriptSlot      bool
	ProxyDbSupport         bool
	ProxyCrossShard        bool
	ProxyScanScope         bool

	// Levels 规则级别：key 为规则名，value 为 error / warn / observe。
	// 没配（或配了非法值）时取规则自带的 DefaultLevel。
	Levels map[string]string
	// Mode 审核模式：ModeWrite（空值同此）/ ModeQuery
	Mode Mode
	// Deployment 部署形态（standalone / cluster / proxy），由调用方探测后传入。
	// 空表示未知：Cluster/Proxy 规则此时不生效——未知既不当违规也不当安全。
	Deployment string
	// Schema 本次命令的目标逻辑库（dbN），供 ClusterDbZero / ProxyDbSupport 判定。
	Schema string
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
