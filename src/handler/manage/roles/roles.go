package roles

import (
	"Yearning-go/src/engine"
	"Yearning-go/src/handler/common"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/model"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"engine/ruleset"

	"github.com/cookieY/yee"
	"gorm.io/gorm"
)

func SuperSaveRoles(c yee.Context) (err error) {

	u := new(engine.AuditRole)

	if err = c.Bind(u); err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	before, _ := loadRuleSet(0)
	if shouldRejectEmptyRuleSet(&before, u, c.QueryParam("confirm") == "true") {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_RULE_SET_EMPTY)))
	}
	audit, _ := json.Marshal(u)
	applyRuleSet(0, audit)
	writeRuleHistory(0, before, *u, operatorName(c), "更新全局规则")
	return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.INFO_DATA_IS_EDIT)))
}

func SuperFetchRoles(c yee.Context) (err error) {
	var k model.CoreGlobalConfiguration
	model.DB().Select("audit_role").First(&k)
	return c.JSON(http.StatusOK, common.SuccessPayload(k.AuditRole))
}

func SuperRolesList(c yee.Context) (err error) {
	var rules []model.CoreRules
	model.DB().Model(model.CoreRules{}).Find(&rules)
	var rows []ruleSetRefRow
	model.DB().Model(model.CoreDataSource{}).Where("rule_id <> 0").Select("rule_id, source").Scan(&rows)
	return c.JSON(http.StatusOK, common.SuccessPayload(mergeRuleSetRefs(rules, rows)))
}

// ruleSetRefRow 是 core_data_sources 里一条「数据源 → 规则集」引用的扫描载体。
type ruleSetRefRow struct {
	RuleID uint
	Source string
}

// ruleSetWithRefs 在规则集上追加引用面:管理员修改/删除规则集前,先看到影响哪些数据源。
type ruleSetWithRefs struct {
	model.CoreRules
	RefCount   int      `json:"ref_count"`
	RefSources []string `json:"ref_sources"`
}

// mergeRuleSetRefs 把引用行聚合进规则集列表:保持输入顺序,悬空引用(指向已删
// 规则集)忽略,无引用的 RefSources 输出空数组而非 null(与前端契约一致)。
func mergeRuleSetRefs(rules []model.CoreRules, rows []ruleSetRefRow) []ruleSetWithRefs {
	refs := make(map[uint][]string, len(rules))
	for _, r := range rows {
		refs[r.RuleID] = append(refs[r.RuleID], r.Source)
	}
	out := make([]ruleSetWithRefs, 0, len(rules))
	for _, r := range rules {
		item := ruleSetWithRefs{CoreRules: r, RefSources: []string{}}
		if len(refs[r.ID]) > 0 {
			item.RefSources = refs[r.ID]
			item.RefCount = len(refs[r.ID])
		}
		out = append(out, item)
	}
	return out
}

func SuperRolesAdd(c yee.Context) (err error) {
	u := new(model.CoreRules)
	if err = c.Bind(u); err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	if err := normalizeMongoRuleSet(u); err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	model.DB().Create(u)
	writeRuleSetHistory(u.ID, nil, u, operatorName(c), "新增规则集")
	return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.INFO_RULE_IS_ADD)))
}

func SuperRoleDelete(c yee.Context) (err error) {
	u := new(model.CoreRules)
	if err = c.Bind(u); err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	// 被数据源引用的规则集不能删：删掉之后这些数据源检测时读不到规则集，会直接报错。
	var used []string
	model.DB().Model(model.CoreDataSource{}).Where("rule_id =?", u.ID).Pluck("source", &used)
	if len(used) > 0 {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(
			fmt.Sprintf(i18n.DefaultLang.Load(i18n.ERR_RULE_SET_IN_USE), strings.Join(used, "、"))))
	}
	// 删除是破坏性操作：记一条历史（回滚可恢复），便于审计与反悔。
	before, _ := loadRuleSetRecord(u.ID)
	model.DB().Model(model.CoreRules{}).Where("id =?", u.ID).Delete(&model.CoreRules{})
	writeRuleSetHistory(u.ID, before, nil, operatorName(c), "删除规则集")
	return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.INFO_RULE_SET_DELETE)))
}

func SuperRoleUpdate(c yee.Context) (err error) {
	u := new(model.CoreRules)
	if err = c.Bind(u); err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	if err := normalizeMongoRuleSet(u); err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	var after engine.AuditRole
	if err = u.AuditRole.UnmarshalToJSON(&after); err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	before, _ := loadRuleSetRecord(u.ID)
	beforeAudit, _ := loadRuleSet(u.ID)
	if shouldRejectEmptyRuleSet(&beforeAudit, &after, c.QueryParam("confirm") == "true") {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_RULE_SET_EMPTY)))
	}
	model.DB().Model(model.CoreRules{}).Where("id =?", u.ID).Updates(&model.CoreRules{
		AuditRole: u.AuditRole, Desc: u.Desc, Definition: u.Definition, Dialect: u.Dialect, Scope: u.Scope,
	})
	writeRuleSetHistory(u.ID, before, u, operatorName(c), "更新规则集")
	return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.INFO_RULE_IS_UPDATED)))
}

func SuperRoleProfile(c yee.Context) (err error) {
	u := new(model.CoreRules)
	if err = c.Bind(u); err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	var rule model.CoreRules
	model.DB().Model(model.CoreRules{}).Where("id =?", u.ID).Find(&rule)
	return c.JSON(http.StatusOK, common.SuccessPayload(rule))
}

// SuperRoleHistory 返回某个规则集的变更历史（rule_id=0 为全局规则），新的在前。
func SuperRoleHistory(c yee.Context) (err error) {
	u := new(model.CoreRuleSetHistory)
	if err = c.Bind(u); err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	var list []model.CoreRuleSetHistory
	model.DB().Model(model.CoreRuleSetHistory{}).Where("rule_id = ?", u.RuleId).Order("id desc").Limit(50).Find(&list)
	return c.JSON(http.StatusOK, common.SuccessPayload(list))
}

// SuperRoleRollback 把规则集恢复到某条历史记录的变更前状态（回滚返回恢复后的规则集，
// 便于前端直接刷新），并再记一条历史 —— 回滚本身同样可追溯、可再回滚。
func SuperRoleRollback(c yee.Context) (err error) {
	u := new(model.CoreRuleSetHistory)
	if err = c.Bind(u); err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	var h model.CoreRuleSetHistory
	if errors.Is(model.DB().Model(model.CoreRuleSetHistory{}).Where("id =?", u.ID).First(&h).Error, gorm.ErrRecordNotFound) || len(h.Before) == 0 {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_RULE_HISTORY_NOT_FOUND)))
	}
	snapshot, err := decodeRuleSetHistorySnapshot(h.Before)
	if err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	beforeAudit, exists := loadRuleSet(h.RuleId)
	current, _ := loadRuleSetRecord(h.RuleId)
	restored, err := restoreRuleSetFromHistory(current, snapshot)
	if err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	if h.RuleId == 0 {
		if !exists {
			return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_RULE_HISTORY_NOT_FOUND)))
		}
		applyRuleSet(0, restored.AuditRole)
		var target engine.AuditRole
		_ = restored.AuditRole.UnmarshalToJSON(&target)
		writeRuleHistory(0, beforeAudit, target, operatorName(c), fmt.Sprintf("回滚到历史 #%d", h.ID))
		return c.JSON(http.StatusOK, common.SuccessPayload(restored))
	}
	if !exists {
		// 规则集已被删除（删除同样记历史）：按快照重建，让删除也能反悔
		model.DB().Create(&model.CoreRules{
			ID:         h.RuleId,
			Desc:       restored.Desc,
			AuditRole:  restored.AuditRole,
			Definition: restored.Definition,
			Dialect:    restored.Dialect,
			Scope:      restored.Scope,
		})
	} else {
		applyRestoredRuleSet(h.RuleId, restored)
	}
	writeRuleSetHistory(h.RuleId, current, restored, operatorName(c), fmt.Sprintf("回滚到历史 #%d", h.ID))
	return c.JSON(http.StatusOK, common.SuccessPayload(restored))
}

// shouldRejectEmptyRuleSet 判断本次写入是否属于「疑似空载荷」。
//
// c.Bind 解析成功但规则整体为零值时，绝大多数情况是前端漏传字段，而不是真要清空规则集：
// 全零会把强制类规则全关、禁止类规则全开，等于静默改变审核口径，因此要求显式
// confirm=true 才放行；before 本来就是零值（首次配置）不算异常。
func shouldRejectEmptyRuleSet(before, after *engine.AuditRole, confirm bool) bool {
	if confirm || after == nil || !reflect.ValueOf(*after).IsZero() {
		return false
	}
	return before != nil && !reflect.ValueOf(*before).IsZero()
}

// loadRuleSet 读取规则集当前值：ruleId=0 为全局规则（内存缓存），>0 为 core_rules.id。
func loadRuleSet(ruleId uint) (engine.AuditRole, bool) {
	var out engine.AuditRole
	if ruleId == 0 {
		if p := model.GloRole.Load(); p != nil {
			return *p, true
		}
		return out, false
	}
	var r model.CoreRules
	if errors.Is(model.DB().Model(model.CoreRules{}).Where("id =?", ruleId).First(&r).Error, gorm.ErrRecordNotFound) {
		return out, false
	}
	if err := r.AuditRole.UnmarshalToJSON(&out); err != nil {
		return out, false
	}
	return out, true
}

// loadRuleSetRecord 读取规则集当前行。回滚 MongoDB v2 时必须知道当前 dialect，
// 因为空 v2 规则集的兼容层可能没有任何 Mongo 开关字段。
func loadRuleSetRecord(ruleId uint) (*model.CoreRules, bool) {
	if ruleId == 0 {
		return nil, false
	}
	var rule model.CoreRules
	if errors.Is(model.DB().Model(model.CoreRules{}).Where("id =?", ruleId).First(&rule).Error, gorm.ErrRecordNotFound) {
		return nil, false
	}
	return &rule, true
}

const ruleSetHistoryVersion = 1

type ruleSetHistorySnapshot struct {
	HistoryVersion int        `json:"history_version"`
	ID             uint       `json:"id,omitempty"`
	Desc           string     `json:"desc"`
	AuditRole      model.JSON `json:"audit_role"`
	Definition     model.JSON `json:"definition"`
	Dialect        string     `json:"dialect"`
	Scope          string     `json:"scope"`
}

func ruleSetHistorySnapshotFromRule(rule *model.CoreRules) ruleSetHistorySnapshot {
	if rule == nil {
		return ruleSetHistorySnapshot{HistoryVersion: ruleSetHistoryVersion}
	}
	return ruleSetHistorySnapshot{
		HistoryVersion: ruleSetHistoryVersion,
		ID:             rule.ID,
		Desc:           rule.Desc,
		AuditRole:      rule.AuditRole,
		Definition:     rule.Definition,
		Dialect:        rule.Dialect,
		Scope:          rule.Scope,
	}
}

func emptyRuleSetHistoryField(raw json.RawMessage) bool {
	value := strings.TrimSpace(string(raw))
	return value == "" || value == "null"
}

func decodeRuleSetHistorySnapshot(data []byte) (model.JSON, error) {
	var probe struct {
		HistoryVersion int `json:"history_version"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("规则集历史快照非法: %w", err)
	}
	switch probe.HistoryVersion {
	case ruleSetHistoryVersion:
		var presence struct {
			AuditRole  json.RawMessage `json:"audit_role"`
			Definition json.RawMessage `json:"definition"`
		}
		if err := json.Unmarshal(data, &presence); err != nil {
			return nil, fmt.Errorf("规则集历史快照非法: %w", err)
		}
		if emptyRuleSetHistoryField(presence.AuditRole) && emptyRuleSetHistoryField(presence.Definition) {
			return nil, errors.New("规则集历史快照为空，无法回滚")
		}
		var snapshot ruleSetHistorySnapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			return nil, fmt.Errorf("规则集历史快照非法: %w", err)
		}
	case 0:
		var legacy map[string]json.RawMessage
		if err := json.Unmarshal(data, &legacy); err != nil {
			return nil, fmt.Errorf("旧规则集历史快照非法: %w", err)
		}
	default:
		return nil, fmt.Errorf("不支持的规则集历史快照版本: %d", probe.HistoryVersion)
	}
	return model.JSON(data), nil
}

// restoreRuleSetFromHistory supports both the versioned full CoreRules snapshot and the
// legacy AuditRole-only snapshot. Legacy MongoDB history is still normalized to v2.
func restoreRuleSetFromHistory(current *model.CoreRules, snapshot model.JSON) (*model.CoreRules, error) {
	var versioned ruleSetHistorySnapshot
	if err := json.Unmarshal(snapshot, &versioned); err == nil && versioned.HistoryVersion == ruleSetHistoryVersion {
		return &model.CoreRules{
			ID:         versioned.ID,
			Desc:       versioned.Desc,
			AuditRole:  versioned.AuditRole,
			Definition: versioned.Definition,
			Dialect:    versioned.Dialect,
			Scope:      versioned.Scope,
		}, nil
	}

	out := &model.CoreRules{AuditRole: snapshot}
	if current == nil {
		return out, nil
	}
	isMongo := current.Dialect == string(ruleset.DialectMongoDB)
	if !isMongo {
		isMongo = looksLikeMongoAuditRole(out)
	}
	if isMongo {
		out.Dialect = string(ruleset.DialectMongoDB)
		if err := normalizeMongoRuleSet(out); err != nil {
			return nil, fmt.Errorf("恢复 MongoDB v2 规则集失败: %w", err)
		}
	}
	return out, nil
}

// applyRestoredRuleSet 使用 map 更新，确保回滚能把 dialect/definition/scope 清空或覆盖。
func applyRestoredRuleSet(ruleId uint, restored *model.CoreRules) {
	model.DB().Model(model.CoreRules{}).Where("id =?", ruleId).Updates(map[string]interface{}{
		"desc":       restored.Desc,
		"audit_role": restored.AuditRole,
		"definition": restored.Definition,
		"dialect":    restored.Dialect,
		"scope":      restored.Scope,
	})
}

// applyRuleSet 写入规则集；全局规则同时刷新内存缓存（与原有的即时生效语义一致）。
func applyRuleSet(ruleId uint, rules model.JSON) {
	if ruleId == 0 {
		model.DB().Model(model.CoreGlobalConfiguration{}).Where("1=1").Updates(&model.CoreGlobalConfiguration{AuditRole: rules})
		var v engine.AuditRole
		if err := rules.UnmarshalToJSON(&v); err == nil {
			model.GloRole.Store(&v)
		}
		return
	}
	model.DB().Model(model.CoreRules{}).Where("id =?", ruleId).Updates(&model.CoreRules{AuditRole: rules})
}

// writeRuleHistory 记录一次规则集变更，供审计与一键回滚。
func writeRuleHistory(ruleId uint, before, after engine.AuditRole, operator, note string) {
	b, _ := json.Marshal(before)
	a, _ := json.Marshal(after)
	model.DB().Create(&model.CoreRuleSetHistory{
		RuleId:    ruleId,
		Before:    b,
		After:     a,
		Operator:  operator,
		Note:      note,
		CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
	})
}

// writeRuleSetHistory stores the complete CoreRules snapshot, preserving unknown v2 rules
// and settings that are not represented by the legacy AuditRole form.
func writeRuleSetHistory(ruleId uint, before, after *model.CoreRules, operator, note string) {
	b, _ := json.Marshal(ruleSetHistorySnapshotFromRule(before))
	a, _ := json.Marshal(ruleSetHistorySnapshotFromRule(after))
	model.DB().Create(&model.CoreRuleSetHistory{
		RuleId:    ruleId,
		Before:    b,
		After:     a,
		Operator:  operator,
		Note:      note,
		CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
	})
}

// operatorName 取当前登录用户名；解析失败留痕为 unknown，不阻断变更本身。
func operatorName(c yee.Context) string {
	if u := new(factory.Token).JwtParse(c).Username; u != "" {
		return u
	}
	return "unknown"
}
