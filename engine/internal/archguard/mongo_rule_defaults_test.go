package archguard

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"engine/internal/mongocheck"
)

// TestMongoRuleDefaultsMatchFrontend 核对规则页声明的默认级别与引擎一致。
//
// 规则页把「未配置」的级别显示成 默认(拦截) / 默认(提示) / 默认(观察)，括号里的值取自
// rules.ts 每个规则行的 level 字段——它是引擎 DefaultLevel 的手工镜像。
// 两边一旦不一致，页面就会撒谎（显示拦截、实际只提示），所以这里强行对齐。
func TestMongoRuleDefaultsMatchFrontend(t *testing.T) {
	rulesTS := readFile(t, filepath.Join(moduleRoot(t), "..", "front/src/views/manager/rules/rules.ts"))
	for _, r := range mongocheck.All() {
		block, ok := ruleRowIn(rulesTS, r.Name())
		if !ok {
			t.Errorf("规则 %q 不在 rules.ts 里", r.Name())
			continue
		}
		got := declaredLevel(block)
		if got == "" {
			t.Errorf("规则 %q 在 rules.ts 里没有声明 level：级别下拉显示不出「默认(拦截/提示/观察)」", r.Name())
			continue
		}
		if got != string(r.DefaultLevel()) {
			t.Errorf("规则 %q 前端声明默认级别 %q，引擎是 %q：两边必须一致（页面按前端声明显示）",
				r.Name(), got, r.DefaultLevel())
		}
	}
}

// ruleRowIn 截出 rules.ts 里某条规则的对象字面量：从 name 到下一个 name 之前
func ruleRowIn(src, name string) (string, bool) {
	i := strings.Index(src, "name: '"+name+"'")
	if i < 0 {
		return "", false
	}
	rest := src[i+1:]
	if j := strings.Index(rest, "name: '"); j >= 0 {
		return src[i : i+1+j], true
	}
	return src[i:], true
}

var declaredLevelRe = regexp.MustCompile(`level:\s*'(error|warn|observe)'`)

func declaredLevel(block string) string {
	if m := declaredLevelRe.FindStringSubmatch(block); m != nil {
		return m[1]
	}
	return ""
}
