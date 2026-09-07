package common

import (
	"Yearning-go/src/lib/permission"
	"Yearning-go/src/lib/vars"
	"Yearning-go/src/model"
	"strings"
)

// IsOrderRelated 判断用户是否与工单相关：提交人、当前审批人之一，或审批链上的历史审批人之一。
// 工单不存在时返回 false，避免通过响应差异枚举工单号。
func IsOrderRelated(workId, user string) bool {
	var o model.CoreSqlOrder
	if err := model.DB().Model(model.CoreSqlOrder{}).
		Select("username,assigned,relevant").
		Where("work_id =?", workId).First(&o).Error; err != nil {
		return false
	}
	if o.Username == user {
		return true
	}
	for _, auditor := range strings.Split(o.Assigned, ",") {
		if strings.TrimSpace(auditor) == user {
			return true
		}
	}
	var relevant []string
	if o.Relevant != nil {
		_ = o.Relevant.UnmarshalToJSON(&relevant)
	}
	for _, r := range relevant {
		if r == user {
			return true
		}
	}
	return false
}

// HasSourcePermission 判断用户对数据源是否具备指定类型（vars.DDL/DML/QUERY）的权限
func HasSourcePermission(user, sourceId string, kind int) bool {
	return permission.NewPermissionService(model.DB()).Equal(&permission.Control{User: user, Kind: kind, SourceId: sourceId})
}

// HasAnySourcePermission 判断用户对数据源是否具备任意一种权限，用于查看库表结构等低敏操作
func HasAnySourcePermission(user, sourceId string) bool {
	for _, kind := range []int{vars.DDL, vars.DML, vars.QUERY} {
		if HasSourcePermission(user, sourceId, kind) {
			return true
		}
	}
	return false
}
