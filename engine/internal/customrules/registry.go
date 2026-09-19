package customrules

import "sync"

var (
	mu    sync.RWMutex
	rules []Rule
	names = map[string]bool{}
)

// Register 注册一条自定义规则，通常在 init 中调用。
// 规则为 nil 或重名会 panic——与 Bytebase advisor.Register 的行为一致，
// 让配置错误在启动/测试期就暴露，而不是静默失效。
func Register(r Rule) {
	if r == nil {
		panic("customrules: Register 的规则为 nil")
	}
	name := r.Name()
	if name == "" {
		panic("customrules: 规则的 Name 不能为空")
	}
	mu.Lock()
	defer mu.Unlock()
	if names[name] {
		panic("customrules: 规则重复注册: " + name)
	}
	names[name] = true
	rules = append(rules, r)
}

// All 返回已注册的全部自定义规则（按注册顺序）。
func All() []Rule {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Rule, len(rules))
	copy(out, rules)
	return out
}
