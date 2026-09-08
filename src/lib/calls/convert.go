package calls

import (
	"Yearning-go/src/model"

	enginev1 "engine/gen/engine/v1"
)

// DataSourceKind 将数据源 DBType 映射为引擎方言标识。
// Yearning 约定：0=mysql。首期仅支持 MySQL，其余返回空串由引擎拒绝。
func DataSourceKind(dbType int) string {
	if dbType == 0 {
		return "mysql"
	}
	return ""
}

// OrderToProto 将本地工单转为引擎 proto 工单。
func OrderToProto(o *model.CoreSqlOrder) *enginev1.Order {
	if o == nil {
		return nil
	}
	return &enginev1.Order{
		WorkId:      o.WorkId,
		Username:    o.Username,
		Status:      uint32(o.Status),
		Type:        int32(o.Type),
		Backup:      uint32(o.Backup),
		Idc:         o.IDC,
		Source:      o.Source,
		SourceId:    o.SourceId,
		DataBase:    o.DataBase,
		Table:       o.Table,
		Date:        o.Date,
		Sql:         o.SQL,
		Text:        o.Text,
		Assigned:    o.Assigned,
		Delay:       o.Delay,
		RealName:    o.RealName,
		ExecuteTime: o.ExecuteTime,
		CurrentStep: int32(o.CurrentStep),
		OscInfo:     o.OSCInfo,
		File:        o.File,
	}
}
