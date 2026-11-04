// Package customrule 执行用户自定义的文本规则（正则）。
//
// 内置审核规则编译在引擎里，无法在线新增；这里补的是"文本层"校验：
// 对提交的 SQL 做正则匹配，命中即按级别拦截或提示。不做语法级判断。
//
// 单独成包是因为 model 依赖 engine（AuditRole 定义在 engine 包），
// 放在 engine 里会与 model 形成循环依赖。
package customrule

import (
	"regexp"

	"Yearning-go/src/model"
	"engine/api"
)

// 与 engine/api.Record.Level 取值保持一致：critical=0 error=1 warning=2 info=3
const (
	levelError   = 1
	levelWarning = 2
)

// Enabled 取启用的自定义规则。规则数量少，每次直接查库，不做缓存。
func Enabled() []model.CoreCustomRule {
	var rs []model.CoreCustomRule
	model.DB().Where("enabled = 1").Order("id").Find(&rs)
	return rs
}

// Hits 用自定义规则匹配整段 SQL，命中即产出一条 Record。
// Go regexp 是 RE2，无回溯风险；正则非法时跳过（保存接口已校验，这里兜底）。
func Hits(sql string, rules []model.CoreCustomRule) []*api.Record {
	if sql == "" || len(rules) == 0 {
		return nil
	}
	var out []*api.Record
	for i := range rules {
		re, err := regexp.Compile(rules[i].Pattern)
		if err != nil || !re.MatchString(sql) {
			continue
		}
		level := uint32(levelError)
		if rules[i].Level == "warning" {
			level = levelWarning
		}
		msg := "命中自定义规则「" + rules[i].Name + "」"
		if rules[i].Desc != "" {
			msg += "：" + rules[i].Desc
		}
		out = append(out, &api.Record{Sql: sql, Level: level, Error: msg})
	}
	return out
}
