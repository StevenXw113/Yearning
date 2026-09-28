// Package customrules 存放团队自研的审核规则。
//
// 与 internal/bytebase（从 Bytebase 上游同步、可被整体覆盖）**物理隔离**：
// scripts/sync-bytebase.sh 只重建 internal/bytebase，绝不触碰本目录，
// 因此升级上游时这里写的规则不会被冲掉。
//
// 自定义规则与上游规则由 server.Check 编排在一起：两者命中都映射为统一的 Record。
// 新增规则：实现 Rule，并在本包的 init 中 Register 即可（见 drop.go 示例）。
package customrules

// Level 是规则严重级别，取值与 enginev1.Record.Level 一致
// （0 critical / 1 error / 2 warning / 3 info）。数值越小越严重。
type Level uint32

const (
	LevelError   Level = 1
	LevelWarning Level = 2
	LevelInfo    Level = 3
)

// Config 是自定义规则可读取的规则开关快照。
//
// 由 server 从 enginev1.AuditRole 构造，规则层因此不依赖 gRPC/上游 proto，
// 上游 proto 变动不会传导到这里。需要新开关时在此显式增加字段。
type Config struct {
	// AllowDropDatabase 语义为“允许删除数据库”，false 表示禁止。
	AllowDropDatabase bool
	// AllowDropTable 语义为“允许删除表”，false 表示禁止。
	AllowDropTable bool
	// ForbidTruncate 语义为“禁止 TRUNCATE”，true 表示启用该规则（默认 false）。
	ForbidTruncate bool
}

// Context 是单条 SQL 的审核输入。
type Context struct {
	// SQL 为已拆分、已去掉结尾分号的单条语句。
	SQL string
	// Schema 为当前数据库，可能为空。
	Schema string
}

// Finding 是一条自定义规则的命中结果。
type Finding struct {
	// Title 规则标识，会展示在记录中（形如 [title] content）。
	Title string
	// Content 命中说明。
	Content string
	// Level 严重级别。
	Level Level
}

// Rule 是自定义审核规则。
type Rule interface {
	// Name 唯一标识规则，用于去重与排查。
	Name() string
	// Check 检查单条 SQL；未命中返回 nil。
	Check(cfg Config, c Context) []Finding
}

// Describable 是可选接口：规则实现它即可把元数据交给 archguard，
// 用于生成 engine/SELF_RULES.md 并核对「开关确实存在于 enginev1.AuditRole」。
// 新规则建议实现，否则清单里看不到它的开关。
type Describable interface {
	// Switches 返回这条规则对应的开关名（enginev1.AuditRole 的字段名）。
	// 一条规则可以受多个开关控制，例如 forbid-drop 同时看
	// DDLEnableDropDatabase 与 DDLEnableDropTable。
	Switches() []string
	// Desc 规则的简短说明
	Desc() string
}
