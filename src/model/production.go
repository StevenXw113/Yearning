package model

import "strings"

// ProductionAuditRequired 判断数据源是否命中生产强制审核。
//
// 规则：
//  1. EnvironmentTag 命中全局配置的 ProductionAuditTags 时启用；
//  2. 标签未配置时使用默认 production / pre-production，保证 fail-closed；
//  3. 存量数据源未填标签时继续兼容 IsProduction。
func ProductionAuditRequired(source *CoreDataSource) bool {
	if source == nil {
		return false
	}
	tag := strings.TrimSpace(source.EnvironmentTag)
	if tag == "" {
		return source.IsProduction
	}
	tags := GloOther.Load().ProductionAuditTags
	if len(tags) == 0 {
		tags = []string{"production", "pre-production"}
	}
	for _, item := range tags {
		if strings.EqualFold(strings.TrimSpace(item), tag) {
			return true
		}
	}
	return false
}
