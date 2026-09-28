package mongocheck

import "strings"

// Category 规则类别：决定文档归类与默认级别（安全类必须 error，由 archguard 护栏强制）
type Category string

const (
	// CategorySecurity 安全类：破坏性/不可逆/绕过审计的操作，默认拦截
	CategorySecurity Category = "security"
	// CategoryStructure 结构类：集合与索引结构变更、命名规范
	CategoryStructure Category = "structure"
	// CategoryPerformance 性能类：无法用索引、全集合扫描等形态可疑的写法，默认只提示
	CategoryPerformance Category = "performance"
)

// Level 命中级别，取值与规则集的 RuleLevels 一致。
// 数值语义见 enginev1.Record.Level：1 拦截 / 2 警告 / 3 观察（前端只拦 1）。
type Level string

const (
	LevelError   Level = "error"   // 拦截，不允许提交/执行
	LevelWarn    Level = "warn"    // 提示，不影响提交
	LevelObserve Level = "observe" // 只记录
)

// ValidLevel 是否是合法级别
func ValidLevel(l Level) bool {
	switch l {
	case LevelError, LevelWarn, LevelObserve:
		return true
	}
	return false
}

// Mode 审核模式，对应 enginev1.CheckRequest.mode。
// 同一个引擎服务两种入口，必须由调用方明说在审哪一种——申请页里填 find
// 要报「不是变更命令」，而不是悄悄按查询规则放行。
type Mode string

const (
	// ModeWrite 变更命令审核（mode 为空时同此，保证旧调用方行为不变）
	ModeWrite Mode = "write"
	// ModeQuery 只读命令审核（查询页）
	ModeQuery Mode = "query"
)

// Finding 一条命中的规则
type Finding struct {
	Rule    string // 规则名（= AuditRole 字段名 = RuleLevels 的 key）
	Level   Level  // 拦截 / 提示 / 观察
	Message string
}

// Rule 一条静态审核规则。
//
// Name 严格 1:1 对应 enginev1.AuditRole 里的开关字段与 RuleLevels 的 key：
// 规则页按它渲染开关、按它存级别、archguard 按它核对三处是否一致。
type Rule interface {
	// Name 规则名
	Name() string
	// Desc 规则页与 SELF_RULES.md 里的一句话说明
	Desc() string
	// Category 规则类别
	Category() Category
	// Modes 适用模式（变更审核 / 查询审核），至少一个
	Modes() []Mode
	// DefaultLevel 规则集里没配级别时用的级别
	DefaultLevel() Level
	// Check 命中即返回；开关关闭时必须返回 nil（新增规则不能改变既有审核行为）
	Check(cfg Config, cmd *Command) []Finding
}

var (
	registry []Rule
	names    = map[string]struct{}{}
)

// Register 注册一条规则，在规则文件的 init() 中调用。
//
// 重名、名称为空、级别非法、安全类默认不拦截都直接 panic——
// 与 internal/customrules.Register 的做法一致：让配置错误在启动/测试期暴露，
// 而不是变成「页面上勾了却永远不生效」。
func Register(r Rule) {
	if r == nil {
		panic("mongocheck: Register 的规则为 nil")
	}
	name := r.Name()
	if name == "" {
		panic("mongocheck: 规则的 Name 不能为空")
	}
	if !ValidLevel(r.DefaultLevel()) {
		panic("mongocheck: 规则 " + name + " 的默认级别非法: " + string(r.DefaultLevel()))
	}
	if r.Category() == CategorySecurity && r.DefaultLevel() != LevelError {
		panic("mongocheck: 安全类规则 " + name + " 的默认级别必须是 error")
	}
	if len(r.Modes()) == 0 {
		panic("mongocheck: 规则 " + name + " 没有声明适用模式")
	}
	if _, dup := names[name]; dup {
		panic("mongocheck: 规则重复注册: " + name)
	}
	names[name] = struct{}{}
	registry = append(registry, r)
}

// All 返回已注册的全部规则（按注册顺序）。
// 这是「规则注册表」的唯一出口：SELF_RULES.md 的生成与 archguard 的核对都读它。
func All() []Rule {
	out := make([]Rule, len(registry))
	copy(out, registry)
	return out
}

// applies 该规则是否适用于当前模式
func applies(r Rule, mode Mode) bool {
	for _, m := range r.Modes() {
		if m == mode {
			return true
		}
	}
	return false
}

// Blocked 返回第一条拦截级（error）命中；没有则返回 nil（warn / observe 不阻断）
func Blocked(findings []Finding) *Finding {
	for i := range findings {
		if findings[i].Level == LevelError {
			return &findings[i]
		}
	}
	return nil
}

// Severity 命中的整体严重程度，取记录用的数字：1 拦截 / 2 警告 / 3 观察
func Severity(findings []Finding) int {
	sev := 3
	for _, f := range findings {
		if f.Level == LevelError {
			return 1
		}
		if f.Level == LevelWarn {
			sev = 2
		}
	}
	return sev
}

// Messages 把命中拼成一行说明
func Messages(findings []Finding) string {
	parts := make([]string, 0, len(findings))
	for _, f := range findings {
		parts = append(parts, "["+string(f.Level)+"] "+f.Message)
	}
	return strings.Join(parts, "；")
}
