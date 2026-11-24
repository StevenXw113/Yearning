package audit

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"Yearning-go/src/handler/common"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/lib/permission"
	"Yearning-go/src/model"

	"github.com/cookieY/yee"
	"gorm.io/gorm"
)

type DBAApprovalRequest struct {
	WorkId string `json:"work_id"`
	Action string `json:"action"`
	Reason string `json:"reason"`
}

func DBAApprovalOrder(c yee.Context) error {
	req := new(DBAApprovalRequest)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE("DBA 特批请求非法"))
	}
	user := new(factory.Token).JwtParse(c).Username
	if !canApproveDBA(user) {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE("仅超级管理员或 DBA 部门可执行特批"))
	}
	action, err := normalizeDBAAction(req.Action)
	if err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(err.Error()))
	}
	if err := validateDBAApprovalInput(action, req.Reason); err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(err.Error()))
	}

	var order model.CoreSqlOrder
	err = model.DB().Model(model.CoreSqlOrder{}).
		Select("work_id, `status`, requires_dba_approval, dba_approval_status").
		Where("work_id =?", strings.TrimSpace(req.WorkId)).First(&order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE("工单不存在"))
	}
	if err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	if !order.RequiresDBAApproval {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE("该工单不需要 DBA 特批"))
	}
	if order.Status != model.OrderStatusAuditing && order.Status != model.OrderStatusWaiting {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE("仅审核中或待执行工单可进行 DBA 特批"))
	}
	if order.DBAApprovalStatus == "approved" || order.DBAApprovalStatus == "rejected" {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE("该工单 DBA 特批已完成，不能重复操作"))
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	updates := dbaApprovalUpdates(action, user, strings.TrimSpace(req.Reason), now)
	res := model.DB().Model(model.CoreSqlOrder{}).
		Where("work_id =? AND requires_dba_approval =? AND dba_approval_status IN ?", order.WorkId, true, []string{"none", "pending"}).
		Updates(updates)
	if res.Error != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(res.Error))
	}
	if res.RowsAffected != 1 {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE("DBA 特批状态已变化，请刷新后重试"))
	}

	model.DB().Create(&model.CoreWorkflowDetail{
		WorkId:   order.WorkId,
		Username: user,
		Time:     now,
		Action:   fmt.Sprintf("MongoDB 高危命令 DBA %s：%s", action, strings.TrimSpace(req.Reason)),
	})
	if action == "approve" {
		return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage("MongoDB 高危命令 DBA 特批已通过"))
	}
	return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage("MongoDB 高危命令 DBA 特批已驳回"))
}

func canApproveDBA(user string) bool {
	if permission.IsSuperUser(user) {
		return true
	}
	var account model.CoreAccount
	if err := model.DB().Model(model.CoreAccount{}).Select("department").
		Where("username =?", user).First(&account).Error; err != nil {
		return false
	}
	return dbaDepartmentAllowed(account.Department)
}

func dbaDepartmentAllowed(department string) bool {
	return strings.EqualFold(strings.TrimSpace(department), "DBA")
}

func normalizeDBAAction(action string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "approve":
		return "approve", nil
	case "reject":
		return "reject", nil
	default:
		return "", errors.New("DBA 特批动作必须为 approve 或 reject")
	}
}

func validateDBAApprovalInput(action, reason string) error {
	if action != "approve" && action != "reject" {
		return errors.New("DBA 特批动作必须为 approve 或 reject")
	}
	if strings.TrimSpace(reason) == "" {
		return errors.New("DBA 特批理由不能为空")
	}
	return nil
}

func dbaApprovalUpdates(action, approver, reason, now string) map[string]interface{} {
	status := "approved"
	if action == "reject" {
		status = "rejected"
	}
	updates := map[string]interface{}{
		"dba_approval_status": status,
		"dba_approver":        approver,
		"dba_approval_reason": reason,
		"dba_approval_time":   now,
	}
	if action == "reject" {
		updates["status"] = model.OrderStatusRejected
	}
	return updates
}
