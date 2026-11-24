package roles

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"Yearning-go/src/engine"
	"Yearning-go/src/model"

	"engine/ruleset"
)

// looksLikeMongoAuditRole 仅用于兼容旧数据：dialect 为空时，只有旧 AuditRole 中
// 存在已配置的 Mongo 专用字段，才将其识别为 MongoDB 规则集，避免误伤 MySQL 等旧规则。
func looksLikeMongoAuditRole(rule *model.CoreRules) bool {
	if len(rule.AuditRole) == 0 {
		return false
	}
	var fields map[string]interface{}
	if err := rule.AuditRole.UnmarshalToJSON(&fields); err != nil {
		return false
	}
	for name, value := range fields {
		if name == "RuleLevel" {
			levels, ok := value.(map[string]interface{})
			if !ok {
				continue
			}
			for ruleName, level := range levels {
				if !strings.HasPrefix(ruleName, "Mongo") {
					continue
				}
				if levelText, ok := level.(string); ok && levelText != "" {
					return true
				}
			}
			continue
		}
		if !strings.HasPrefix(name, "Mongo") {
			continue
		}
		switch value := value.(type) {
		case bool:
			if value {
				return true
			}
		case float64:
			if value != 0 {
				return true
			}
		case string:
			if strings.TrimSpace(value) != "" {
				return true
			}
		}
	}
	return false
}

// containsNonMongoAuditRoleValues 防止 MongoDB v2 双写覆盖旧混合规则集。
// 规则集按数据源独立后，混合方言必须显式拆分，不能静默丢弃其它数据源配置。
func containsNonMongoAuditRoleValues(rule *model.CoreRules) (bool, error) {
	if rule == nil || len(rule.AuditRole) == 0 {
		return false, nil
	}
	var fields map[string]json.RawMessage
	if err := rule.AuditRole.UnmarshalToJSON(&fields); err != nil {
		return false, fmt.Errorf("解析旧规则集失败: %w", err)
	}
	known := nonMongoAuditRoleKeys()
	for name, raw := range fields {
		if name == "RuleLevel" {
			var levels map[string]json.RawMessage
			if err := json.Unmarshal(raw, &levels); err != nil {
				return false, fmt.Errorf("解析旧规则级别失败: %w", err)
			}
			for ruleName, level := range levels {
				if strings.HasPrefix(ruleName, "Mongo") {
					continue
				}
				if nonZeroJSONValue(level) {
					return true, nil
				}
			}
			continue
		}
		if _, ok := known[name]; !ok {
			continue
		}
		if nonZeroJSONValue(raw) {
			return true, nil
		}
	}
	return false, nil
}

// nonMongoAuditRoleKeys 只检查旧 AuditRole 中已知且非 Mongo 的字段，避免未来未知元数据误报。
func nonMongoAuditRoleKeys() map[string]struct{} {
	out := map[string]struct{}{}
	t := reflect.TypeOf(engine.AuditRole{})
	for i := 0; i < t.NumField(); i++ {
		name := t.Field(i).Name
		if tag := t.Field(i).Tag.Get("json"); tag != "" {
			if name = strings.Split(tag, ",")[0]; name == "-" {
				continue
			}
		}
		if name == "RuleLevel" || strings.HasPrefix(name, "Mongo") {
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

// normalizeMongoRuleSet 双写 MongoDB 规则集：
// Definition 是 v2 权威数据，AuditRole 是旧调用方兼容层。
func normalizeMongoRuleSet(rule *model.CoreRules) error {
	if rule == nil {
		return errors.New("规则集为空")
	}
	if rule.Dialect == "" && looksLikeMongoAuditRole(rule) {
		rule.Dialect = string(ruleset.DialectMongoDB)
	}
	if len(rule.Definition) > 0 && rule.Dialect == "" {
		rule.Dialect = string(ruleset.DialectMongoDB)
	}
	if rule.Dialect != string(ruleset.DialectMongoDB) {
		if len(rule.Definition) > 0 {
			return fmt.Errorf("仅 MongoDB 支持 v2 规则集，当前 dialect=%q", rule.Dialect)
		}
		return nil
	}
	rule.Dialect = string(ruleset.DialectMongoDB)
	rule.Scope = string(ruleset.ScopeDataSource)

	mixed, err := containsNonMongoAuditRoleValues(rule)
	if err != nil {
		return err
	}
	if mixed {
		return errors.New("MongoDB 规则集包含已配置的非 MongoDB 规则；请先拆分为独立数据源规则集")
	}

	if len(rule.Definition) > 0 {
		def, err := ruleset.Decode(rule.Definition)
		if err != nil {
			return fmt.Errorf("解析 MongoDB 规则集失败: %w", err)
		}
		ruleset.EnforceMongoDBAApprovalRules(&def)
		if err := ruleset.ValidateMongoDefinition(def); err != nil {
			return fmt.Errorf("MongoDB 规则集非法: %w", err)
		}
		data, err := json.Marshal(def)
		if err != nil {
			return fmt.Errorf("生成 MongoDB v2 规则集失败: %w", err)
		}
		rule.Definition = data
		old, err := json.Marshal(engine.AuditRoleFromAPI(ruleset.MongoDefinitionToAPI(def)))
		if err != nil {
			return fmt.Errorf("生成规则集兼容层失败: %w", err)
		}
		rule.AuditRole = old
		return nil
	}

	var old engine.AuditRole
	if len(rule.AuditRole) == 0 {
		return errors.New("MongoDB 规则集不能为空")
	}
	if err := rule.AuditRole.UnmarshalToJSON(&old); err != nil {
		return fmt.Errorf("解析 MongoDB 旧规则集失败: %w", err)
	}
	def := ruleset.MongoDefinitionFromAPI(engine.AuditRoleToAPI(&old))
	ruleset.EnforceMongoDBAApprovalRules(&def)
	if err := ruleset.ValidateMongoDefinition(def); err != nil {
		return fmt.Errorf("MongoDB 规则集非法: %w", err)
	}
	data, err := json.Marshal(def)
	if err != nil {
		return fmt.Errorf("生成 MongoDB v2 规则集失败: %w", err)
	}
	rule.Definition = data
	return nil
}
