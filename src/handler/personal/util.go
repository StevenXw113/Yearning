package personal

import (
	"Yearning-go/src/handler/order/audit"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/calls"
	"Yearning-go/src/lib/enc"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/model"
	"errors"
	"github.com/cookieY/yee/logger"
	"gorm.io/gorm"
	"log"
	"time"
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
	var isCall bool
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
	client, err := calls.NewRpc()
	if err != nil {
		logger.DefaultLogger.Error(err)
		return
	}
	defer client.Close()
	if err := client.Call("Engine.Exec", &audit.ExecArgs{
		Order:         order,
		Rules:         *rule,
		IP:            source.IP,
		Port:          source.Port,
		Username:      source.Username,
		Password:      p,
		Message:       *model.GloMessage.Load(),
		MaxAffectRows: autoTask.Affectrow,
	}, &isCall); err != nil {
		log.Println(err)
	}
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
