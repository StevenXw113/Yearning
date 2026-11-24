package factory

import (
	"Yearning-go/src/engine"
	"Yearning-go/src/model"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// LegacyRuleDialect 表示旧 AuditRole 中可以推断出的方言族。
// 旧 SQL 规则无法区分 MySQL 与 PostgreSQL，因此保留 SQL 族并在绑定时双向兼容。
type LegacyRuleDialect string

const (
	LegacyRuleDialectSQL   LegacyRuleDialect = "sql"
	LegacyRuleDialectMongo LegacyRuleDialect = "mongodb"
	LegacyRuleDialectRedis LegacyRuleDialect = "redis"
	LegacyRuleDialectDoris LegacyRuleDialect = "doris"
)

// InferLegacyRuleDialect 从旧 AuditRole 推断方言。只把已配置（true / 非 0 / 非空字符串）
// 的已知字段作为证据；未知元数据不参与判断，避免未来字段破坏旧规则集兼容。
// 混合方言返回错误，要求先拆分为独立规则集。
func InferLegacyRuleDialect(rule *model.CoreRules) (LegacyRuleDialect, error) {
	if rule == nil || len(rule.AuditRole) == 0 {
		return "", nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rule.AuditRole, &fields); err != nil {
		return "", fmt.Errorf("解析旧规则集失败: %w", err)
	}
	known := knownAuditRoleFieldNames()
	seen := make(map[LegacyRuleDialect]struct{})
	for name, raw := range fields {
		if name == "RuleLevel" {
			var levels map[string]json.RawMessage
			if err := json.Unmarshal(raw, &levels); err != nil {
				return "", fmt.Errorf("解析旧规则级别失败: %w", err)
			}
			for levelName, levelValue := range levels {
				if _, ok := known[levelName]; !ok {
					continue
				}
				if nonZeroJSONValue(levelValue) {
					seen[legacyDialectFromKey(levelName)] = struct{}{}
				}
			}
			continue
		}
		if _, ok := known[name]; !ok {
			continue
		}
		if nonZeroJSONValue(raw) {
			seen[legacyDialectFromKey(name)] = struct{}{}
		}
	}
	_, hasSQL := seen[LegacyRuleDialectSQL]
	_, hasMongo := seen[LegacyRuleDialectMongo]
	_, hasRedis := seen[LegacyRuleDialectRedis]
	_, hasDoris := seen[LegacyRuleDialectDoris]
	// 旧 Doris / Redis 规则集中允许同时携带通用 SQL 字段：历史实现把两类配置放在
	// 同一 AuditRole 里，导出时应按专用方言归组。只有真正互斥的专用方言混合才拒绝。
	if (hasMongo && (hasSQL || hasRedis || hasDoris)) || (hasRedis && hasDoris) {
		return "", fmt.Errorf("旧规则集混合了多种数据源方言，请先拆分为独立规则集")
	}
	if hasMongo {
		return LegacyRuleDialectMongo, nil
	}
	if hasRedis {
		return LegacyRuleDialectRedis, nil
	}
	if hasDoris {
		return LegacyRuleDialectDoris, nil
	}
	if hasSQL {
		return LegacyRuleDialectSQL, nil
	}
	return "", nil
}

func legacyDialectFromKey(name string) LegacyRuleDialect {
	switch {
	case strings.HasPrefix(name, "Mongo"):
		return LegacyRuleDialectMongo
	case strings.HasPrefix(name, "Redis"):
		return LegacyRuleDialectRedis
	case strings.HasPrefix(name, "Doris"):
		return LegacyRuleDialectDoris
	default:
		return LegacyRuleDialectSQL
	}
}

func knownAuditRoleFieldNames() map[string]struct{} {
	out := make(map[string]struct{})
	t := reflect.TypeOf(engine.AuditRole{})
	for i := 0; i < t.NumField(); i++ {
		name := t.Field(i).Name
		if tag := t.Field(i).Tag.Get("json"); tag != "" {
			name = strings.Split(tag, ",")[0]
		}
		if name == "-" {
			continue
		}
		out[name] = struct{}{}
	}
	return out
}

func nonZeroJSONValue(raw json.RawMessage) bool {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" || value == "false" || value == "0" || value == `""` {
		return false
	}
	var decoded interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return true
	}
	switch v := decoded.(type) {
	case nil, bool:
		return v == true
	case float64:
		return v != 0
	case string:
		return strings.TrimSpace(v) != ""
	default:
		return true
	}
}
