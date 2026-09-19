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
	"time"

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
	return c.JSON(http.StatusOK, common.SuccessPayload(rules))
}

func SuperRolesAdd(c yee.Context) (err error) {
	u := new(model.CoreRules)
	if err = c.Bind(u); err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	var after engine.AuditRole
	_ = u.AuditRole.UnmarshalToJSON(&after)
	model.DB().Create(u)
	writeRuleHistory(u.ID, engine.AuditRole{}, after, operatorName(c), "新增规则集")
	return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.INFO_RULE_IS_ADD)))
}

func SuperRoleDelete(c yee.Context) (err error) {
	u := new(model.CoreRules)
	if err = c.Bind(u); err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	model.DB().Model(model.CoreRules{}).Where("id =?", u.ID).Delete(&model.CoreRules{})
	return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.RULE_IS_DELETE)))
}

func SuperRoleUpdate(c yee.Context) (err error) {
	u := new(model.CoreRules)
	if err = c.Bind(u); err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	var after engine.AuditRole
	if err = u.AuditRole.UnmarshalToJSON(&after); err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	before, _ := loadRuleSet(u.ID)
	if shouldRejectEmptyRuleSet(&before, &after, c.QueryParam("confirm") == "true") {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_RULE_SET_EMPTY)))
	}
	model.DB().Model(model.CoreRules{}).Where("id =?", u.ID).Updates(&model.CoreRules{AuditRole: u.AuditRole, Desc: u.Desc})
	writeRuleHistory(u.ID, before, after, operatorName(c), "更新规则集")
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
	var target engine.AuditRole
	before, ok := loadRuleSet(h.RuleId)
	if !ok || h.Before.UnmarshalToJSON(&target) != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_RULE_HISTORY_NOT_FOUND)))
	}
	applyRuleSet(h.RuleId, h.Before)
	writeRuleHistory(h.RuleId, before, target, operatorName(c), fmt.Sprintf("回滚到历史 #%d", h.ID))
	return c.JSON(http.StatusOK, common.SuccessPayload(target))
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

// operatorName 取当前登录用户名；解析失败留痕为 unknown，不阻断变更本身。
func operatorName(c yee.Context) string {
	if u := new(factory.Token).JwtParse(c).Username; u != "" {
		return u
	}
	return "unknown"
}
