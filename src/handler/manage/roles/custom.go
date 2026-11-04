package roles

import (
	"net/http"
	"regexp"
	"strings"

	"Yearning-go/src/handler/common"
	"Yearning-go/src/model"
	"github.com/cookieY/yee"
)

type customRuleForm struct {
	ID      uint   `json:"id"`
	Name    string `json:"name"`
	Desc    string `json:"desc"`
	Pattern string `json:"pattern"`
	Level   string `json:"level"`
	Enabled uint   `json:"enabled"`
}

// SuperCustomRuleList 自定义审核规则列表（含停用的，页面要能再启用）。
func SuperCustomRuleList(c yee.Context) (err error) {
	var rs []model.CoreCustomRule
	model.DB().Order("id").Find(&rs)
	return c.JSON(http.StatusOK, common.SuccessPayload(rs))
}

// SuperCustomRuleSave 新增或更新（id>0 为更新）。正则必须可编译，否则审核链路会静默跳过。
func SuperCustomRuleSave(c yee.Context) (err error) {
	u := new(customRuleForm)
	if err = c.Bind(u); err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE("请求参数错误"))
	}
	name := strings.TrimSpace(u.Name)
	pattern := strings.TrimSpace(u.Pattern)
	if name == "" || pattern == "" {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE("规则名称与正则表达式不能为空"))
	}
	if _, e := regexp.Compile(pattern); e != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE("正则表达式无效: "+e.Error()))
	}
	level := "error"
	if u.Level == "warning" {
		level = "warning"
	}
	enabled := u.Enabled
	if enabled > 1 {
		enabled = 1
	}
	rule := model.CoreCustomRule{
		Name:    name,
		Desc:    strings.TrimSpace(u.Desc),
		Pattern: pattern,
		Level:   level,
		Enabled: enabled,
	}
	if u.ID > 0 {
		model.DB().Model(&model.CoreCustomRule{}).Where("id = ?", u.ID).Updates(map[string]interface{}{
			"name":    rule.Name,
			"desc":    rule.Desc,
			"pattern": rule.Pattern,
			"level":   rule.Level,
			"enabled": rule.Enabled,
		})
	} else {
		model.DB().Create(&rule)
	}
	return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage("自定义规则已保存"))
}

func SuperCustomRuleDelete(c yee.Context) (err error) {
	u := new(customRuleForm)
	if err = c.Bind(u); err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE("请求参数错误"))
	}
	model.DB().Where("id = ?", u.ID).Delete(&model.CoreCustomRule{})
	return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage("自定义规则已删除"))
}
