package personal

import (
	"Yearning-go/src/engine"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/calls"
	"Yearning-go/src/lib/enc"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/model"
	"context"
	"errors"
	"github.com/cookieY/yee/logger"
	"gorm.io/gorm"
	"time"
	enginev1 "engine/gen/engine/v1"
)

func autoTask(order *model.CoreSqlOrder, length int) {
	// todo 以下代码为autoTask代码
	var autoTask model.CoreAutoTask
	var source model.CoreDataSource
	if err := model.DB().Model(model.CoreAutoTask{}).
		Where("source_id = ? and data_base =? and `table` =?", order.SourceId, order.DataBase, order.Table).
		First(&autoTask).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return
	}
	model.DB().Model(model.CoreDataSource{}).Where("source_id =?", order.SourceId).First(&source)
	rule, err := factory.CheckDataSourceRule(source.RuleId)
	if err != nil || rule == nil {
		logger.DefaultLogger.Error(err)
		return
	}
	p := enc.Decrypt(model.C.General.SecretKey, source.Password)
	if p == "" {
		logger.DefaultLogger.Error(i18n.DefaultLang.Load(i18n.ER_KEY_DECRYPTION_FAILED))
		return
	}
	var isCall bool
	client, conn, err := calls.NewClient()
	if err != nil {
		logger.DefaultLogger.Error(err)
		return
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	rep, err := client.Exec(ctx, &enginev1.ExecRequest{
		Order: calls.OrderToProto(order),
		Rules: engine.AuditRoleToProto(rule),
		Source: &enginev1.DataSource{
			Ip:       source.IP,
			Port:     int32(source.Port),
			Username: source.Username,
			Password: p,
			Ca:       source.CAFile,
			Cert:     source.Cert,
			Key:      source.KeyFile,
			Kind:     calls.DataSourceKind(source.DBType),
		},
		MaxAffectRows: uint32(autoTask.Affectrow),
	})
	if err != nil {
		logger.DefaultLogger.Error(err)
		return
	}
	isCall = rep != nil && rep.Ok
	if isCall {
		model.DB().Create(&model.CoreWorkflowDetail{
			WorkId:   order.WorkId,
			Username: "AutoTask Robot",
			Time:     time.Now().Format("2006-01-02 15:04"),
			Action:   i18n.DefaultLang.Load(i18n.ORDER_EXECUTE_STATE),
		})
		model.DB().Model(model.CoreSqlOrder{}).Where("work_id =?", order.WorkId).Updates(&model.CoreSqlOrder{CurrentStep: length})
	}

}
