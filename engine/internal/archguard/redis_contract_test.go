package archguard

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"engine/internal/redischeck"
)

// TestRedisConfigCoversAllSwitches 核对 proto 里每个 Redis* 开关都能被 redischeck.Config 取到。
//
// 挡的是「加了 proto 字段 + 前端开关，忘了在 Config 里加字段」这一类静默失效：
// 页面勾得动、规则集存得下，但引擎读不到，规则永远不生效。Redis 的开关全部是
// 静态审核规则（没有执行期例外），所以判据比 Mongo 侧更直接：每个 Redis* 字段都得有。
func TestRedisConfigCoversAllSwitches(t *testing.T) {
	cfg := redischeck.Config{}
	checked := 0
	for _, name := range auditRoleFields(t) {
		if !strings.HasPrefix(strings.ToLower(name), "redis") {
			continue
		}
		checked++
		if !cfg.Has(name) {
			t.Errorf("api.AuditRole 的 %q 在 redischeck.Config 里没有对应字段：规则读不到它，开关会变成「勾了不生效」", name)
		}
	}
	if checked == 0 {
		t.Error("未在 AuditRole 里找到任何 Redis* 开关")
	}
}

// TestRedisRuleDefaultLevels 核对规则自带的默认级别：取值必须合法，
// 且安全类规则必须默认拦截——安全类降成 warn/observe 意味着「默认不拦」，
// 那种降级只能由使用者在规则集里显式配置，不能写死在代码里。
func TestRedisRuleDefaultLevels(t *testing.T) {
	for _, r := range redischeck.All() {
		lv := r.DefaultLevel()
		if !redischeck.ValidLevel(lv) {
			t.Errorf("规则 %q 的默认级别非法: %q", r.Name(), lv)
			continue
		}
		if r.Category() == redischeck.CategorySecurity && lv != redischeck.LevelError {
			t.Errorf("安全类规则 %q 的默认级别是 %q，必须是 error", r.Name(), lv)
		}
	}
}

// TestRedisFrontendRuleLevels 核对规则页里每条 Redis 规则都写了 level，且与引擎一致。
//
// 规则页的 level 决定级别下拉显示成「默认(拦截/提示/观察)」；漏写会退回 error，
// 于是提示类规则在页面上看起来像拦截规则，用户不敢开。
func TestRedisFrontendRuleLevels(t *testing.T) {
	root := filepath.Join(moduleRoot(t), "..")
	rulesTS := readFile(t, filepath.Join(root, "front/src/views/manager/rules/rules.ts"))
	// redisRule('Name', 'category', tp, 'level')：tp 与 level 都可省略
	re := regexp.MustCompile(`redisRule\('([A-Za-z]+)',\s*'[^']+'(?:,\s*\d+)?(?:,\s*'([a-z]+)')?\)`)
	got := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(rulesTS, -1) {
		got[m[1]] = m[2]
	}
	for _, r := range redischeck.All() {
		lv, ok := got[r.Name()]
		if !ok {
			t.Errorf("规则 %q 不在 rules.ts 的 redisRule 调用里", r.Name())
			continue
		}
		if lv == "" {
			t.Errorf("规则 %q 没写 level：级别下拉会显示成「默认(拦截)」，与引擎的 %q 不一致", r.Name(), r.DefaultLevel())
			continue
		}
		if lv != string(r.DefaultLevel()) {
			t.Errorf("规则 %q 前端 level=%q，引擎 DefaultLevel=%q", r.Name(), lv, r.DefaultLevel())
		}
	}
}
