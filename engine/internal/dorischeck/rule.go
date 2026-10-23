// Package dorischeck 实现 Apache Doris 建表语句的静态审核。
//
// Doris 兼容 MySQL 协议与查询/DML 语法，但建表 DDL 是自有语法
// （DUPLICATE/UNIQUE/AGGREGATE KEY、DISTRIBUTED BY HASH(...) BUCKETS n、
// PROPERTIES("replication_num"="1")），MySQL 解析器解析不了。所以这里自研：
// 对建表语句做**文本级**约束（不建完整语法树），够挡住最容易出错的几件事——
// 没指定分桶方式、分桶数过大、副本数不符、聚合模型误用。
//
// 结构对齐 internal/mongocheck：规则注册表 + 共享解析层 + 规则开关，
// 规则在 init() 里注册。
//
// 只用标准库：不引入 SQL 解析器，避免引擎依赖变重。
package dorischeck

import "strings"

// Category 规则类别：决定文档归类与默认级别（安全类必须 error，由 archguard 护栏强制）
type Category string

const (
	CategorySecurity    Category = "security"
	CategoryStructure   Category = "structure"
	CategoryPerformance Category = "performance"
)

// Level 命中级别，取值与规则集的 RuleLevels 一致。
// 数值语义见 enginev1.Record.Level：1 拦截 / 2 警告 / 3 观察（前端只拦 1）。
type Level string

const (
	LevelError   Level = "error"
	LevelWarn    Level = "warn"
	LevelObserve Level = "observe"
)

// ValidLevel 是否是合法级别
func ValidLevel(l Level) bool {
	switch l {
	case LevelError, LevelWarn, LevelObserve:
		return true
	}
	return false
}

// Finding 一条命中的规则
type Finding struct {
	Rule    string
	Level   Level
	Message string
}

// Rule 一条建表审核规则。
//
// Name 严格 1:1 对应 enginev1.AuditRole 里的开关字段与 RuleLevels 的 key。
type Rule interface {
	Name() string
	Desc() string
	Category() Category
	DefaultLevel() Level
	// Check 命中即返回；开关关闭时必须返回 nil
	Check(cfg Config, stmt *CreateTable) []Finding
}

var (
	registry []Rule
	names    = map[string]struct{}{}
)

// Register 注册一条规则，在规则文件的 init() 中调用。
// 重名、名称为空、级别非法、安全类默认不拦截都直接 panic（与 mongocheck 一致）。
func Register(r Rule) {
	if r == nil {
		panic("dorischeck: Register 的规则为 nil")
	}
	name := r.Name()
	if name == "" {
		panic("dorischeck: 规则的 Name 不能为空")
	}
	if !ValidLevel(r.DefaultLevel()) {
		panic("dorischeck: 规则 " + name + " 的默认级别非法: " + string(r.DefaultLevel()))
	}
	if r.Category() == CategorySecurity && r.DefaultLevel() != LevelError {
		panic("dorischeck: 安全类规则 " + name + " 的默认级别必须是 error")
	}
	if _, dup := names[name]; dup {
		panic("dorischeck: 规则重复注册: " + name)
	}
	names[name] = struct{}{}
	registry = append(registry, r)
}

// All 返回已注册的全部规则（按注册顺序）。
func All() []Rule {
	out := make([]Rule, len(registry))
	copy(out, registry)
	return out
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
