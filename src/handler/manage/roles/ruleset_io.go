package roles

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"Yearning-go/src/handler/common"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/model"

	"engine/ruleset"
	"github.com/cookieY/yee"
	"gorm.io/gorm"
)

const ruleSetExportVersion = 1

// SuperRulesManifest 返回 MongoDB v2 规则清单，供前端渲染和导入导出校验。
func SuperRulesManifest(c yee.Context) error {
	return c.JSON(http.StatusOK, common.SuccessPayload(ruleset.MongoManifest()))
}

type ruleSetExportPayload struct {
	ExportVersion      int                 `json:"export_version"`
	Dialect            string              `json:"dialect"`
	CompatibleDialects []string            `json:"compatible_dialects,omitempty"`
	Scope              string              `json:"scope"`
	AuditRole          model.JSON          `json:"audit_role,omitempty"`
	Definition         *ruleset.Definition `json:"definition,omitempty"`
}

type ruleSetImportPayload struct {
	ExportVersion      int                 `json:"export_version"`
	Dialect            string              `json:"dialect"`
	CompatibleDialects []string            `json:"compatible_dialects"`
	Scope              string              `json:"scope"`
	AuditRole          model.JSON          `json:"audit_role"`
	Definition         *ruleset.Definition `json:"definition"`
}

// SuperRuleSetExport exports every data-source rule set. MongoDB keeps its v2 Definition;
// legacy SQL/Redis/Doris rule sets export their AuditRole in a versioned envelope.
func SuperRuleSetExport(c yee.Context) error {
	id, err := strconv.Atoi(c.QueryParam("rule_id"))
	if err != nil || id <= 0 {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE("rule_id 非法"))
	}
	var rule model.CoreRules
	err = model.DB().Model(model.CoreRules{}).Where("id = ?", id).First(&rule).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE("规则集不存在"))
	}
	if err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	payload, err := ruleSetExportFromRule(&rule)
	if err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	return c.JSON(http.StatusOK, common.SuccessPayload(payload))
}

func ruleSetExportFromRule(rule *model.CoreRules) (ruleSetExportPayload, error) {
	if rule == nil {
		return ruleSetExportPayload{}, errors.New("规则集不存在")
	}
	payload := ruleSetExportPayload{
		ExportVersion: ruleSetExportVersion,
		Scope:         rule.Scope,
		AuditRole:     rule.AuditRole,
	}
	if payload.Scope == "" {
		payload.Scope = string(ruleset.ScopeDataSource)
	}

	if len(rule.Definition) > 0 && rule.Dialect != "" && rule.Dialect != string(ruleset.DialectMongoDB) {
		return ruleSetExportPayload{}, fmt.Errorf("仅 MongoDB 支持 v2 规则集，当前 dialect=%q", rule.Dialect)
	}
	if len(rule.Definition) > 0 || rule.Dialect == string(ruleset.DialectMongoDB) {
		return mongoRuleSetExport(rule, payload)
	}

	dialect := rule.Dialect
	if dialect == "" {
		inferred, err := factory.InferLegacyRuleDialect(rule)
		if err != nil {
			return ruleSetExportPayload{}, err
		}
		if inferred == factory.LegacyRuleDialectMongo {
			return mongoRuleSetExport(rule, payload)
		}
		switch inferred {
		case factory.LegacyRuleDialectRedis:
			dialect = string(ruleset.DialectRedis)
		case factory.LegacyRuleDialectDoris:
			dialect = string(ruleset.DialectDoris)
		default:
			dialect = "sql"
			payload.CompatibleDialects = []string{string(ruleset.DialectMySQL), string(ruleset.DialectPostgres)}
		}
	}
	payload.Dialect = dialect
	return payload, nil
}

func mongoRuleSetExport(rule *model.CoreRules, payload ruleSetExportPayload) (ruleSetExportPayload, error) {
	if err := normalizeMongoRuleSet(rule); err != nil {
		return ruleSetExportPayload{}, err
	}
	payload.AuditRole = rule.AuditRole
	var def ruleset.Definition
	if err := json.Unmarshal(rule.Definition, &def); err != nil {
		return ruleSetExportPayload{}, fmt.Errorf("解析规则集失败: %w", err)
	}
	if err := ruleset.ValidateMongoDefinition(def); err != nil {
		return ruleSetExportPayload{}, fmt.Errorf("规则集非法: %w", err)
	}
	payload.Dialect = string(ruleset.DialectMongoDB)
	payload.Definition = &def
	return payload, nil
}

// SuperRuleSetImport accepts the versioned all-dialect envelope and remains compatible with
// the first MongoDB v2 raw Definition file format.
func SuperRuleSetImport(c yee.Context) error {
	var payload ruleSetImportPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE("规则集 JSON 非法"))
	}
	switch {
	case payload.ExportVersion == ruleSetExportVersion:
		return importRuleSetEnvelope(c, payload)
	case payload.Definition != nil:
		return importMongoDefinition(c, *payload.Definition)
	default:
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE("规则集 JSON 缺少 export_version 或 definition"))
	}
}

func importMongoDefinition(c yee.Context, def ruleset.Definition) error {
	data, err := json.Marshal(def)
	if err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	rule := &model.CoreRules{
		Desc:       c.QueryParam("desc"),
		Definition: data,
		Dialect:    string(ruleset.DialectMongoDB),
		Scope:      string(ruleset.ScopeDataSource),
	}
	if err := normalizeMongoRuleSet(rule); err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	if err := model.DB().Create(rule).Error; err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	writeRuleSetHistory(rule.ID, nil, rule, operatorName(c), "导入 MongoDB v2 规则集")
	return c.JSON(http.StatusOK, common.SuccessPayload(rule))
}

func importRuleSetEnvelope(c yee.Context, payload ruleSetImportPayload) error {
	dialect := payload.Dialect
	if dialect == "sql" {
		dialect = c.QueryParam("target_dialect")
		if dialect == "" {
			dialect = string(ruleset.DialectMySQL)
		}
	}
	if err := validateImportDialect(dialect); err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	scope := payload.Scope
	if scope == "" {
		scope = string(ruleset.ScopeDataSource)
	}
	if scope != string(ruleset.ScopeDataSource) {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE("规则集导入仅支持 data_source 作用范围"))
	}
	if len(payload.AuditRole) == 0 {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE("规则集导入缺少 audit_role"))
	}

	rule := &model.CoreRules{
		Desc:      c.QueryParam("desc"),
		AuditRole: payload.AuditRole,
		Dialect:   dialect,
		Scope:     scope,
	}
	if dialect == string(ruleset.DialectMongoDB) {
		if payload.Definition != nil {
			data, err := json.Marshal(payload.Definition)
			if err != nil {
				return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
			}
			rule.Definition = data
		}
		if err := normalizeMongoRuleSet(rule); err != nil {
			return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
		}
	} else {
		if payload.Definition != nil {
			return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE("仅 MongoDB 支持 v2 Definition"))
		}
		inferred, err := factory.InferLegacyRuleDialect(rule)
		if err != nil {
			return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
		}
		if !importDialectCompatible(dialect, inferred) {
			return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE("规则集内容与目标数据源方言不匹配"))
		}
	}
	if err := model.DB().Create(rule).Error; err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	writeRuleSetHistory(rule.ID, nil, rule, operatorName(c), "导入规则集")
	return c.JSON(http.StatusOK, common.SuccessPayload(rule))
}

func validateImportDialect(dialect string) error {
	switch ruleset.Dialect(dialect) {
	case ruleset.DialectMySQL, ruleset.DialectPostgres, ruleset.DialectMongoDB, ruleset.DialectRedis, ruleset.DialectDoris:
		return nil
	default:
		return fmt.Errorf("不支持的数据源方言: %s", dialect)
	}
}

func importDialectCompatible(target string, inferred factory.LegacyRuleDialect) bool {
	switch ruleset.Dialect(target) {
	case ruleset.DialectMySQL, ruleset.DialectPostgres:
		return inferred == factory.LegacyRuleDialectSQL || inferred == ""
	case ruleset.DialectMongoDB:
		return inferred == factory.LegacyRuleDialectMongo
	case ruleset.DialectRedis:
		return inferred == factory.LegacyRuleDialectRedis
	case ruleset.DialectDoris:
		return inferred == factory.LegacyRuleDialectDoris
	default:
		return false
	}
}
