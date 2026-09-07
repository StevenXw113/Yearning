package audit

import (
	"Yearning-go/src/engine"
	"Yearning-go/src/handler/common"
	"Yearning-go/src/handler/manage/flow"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/calls"
	"Yearning-go/src/lib/enc"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/lib/pusher"
	"Yearning-go/src/model"
	"encoding/json"
	"fmt"
	"github.com/cookieY/yee/logger"
	"strings"
	"time"
)

type ExecArgs struct {
	Order         *model.CoreSqlOrder
	Rules         engine.AuditRole
	IP            string
	Port          int
	Username      string
	Password      string
	CA            string
	Cert          string
	Key           string
	Message       model.Message
	MaxAffectRows uint
}

type Confirm struct {
	WorkId   string `json:"work_id"`
	Page     int    `json:"page"`
	Flag     int    `json:"flag"`
	Text     string `json:"text"`
	Tp       string `json:"tp"`
	SourceId string `json:"source_id"`
	Delay    string `json:"delay"`
}

func (e *Confirm) GetTPL() []flow.Tpl {
	var order model.CoreSqlOrder
	var s model.CoreDataSource
	var tpl []flow.Tpl
	var flow model.CoreWorkflowTpl
	// 必须从工单自身查出 source_id：请求体里的 source_id 由客户端提供，
	// 用它来决定审批链会让攻击者为任意工单挑选审批人
	model.DB().Model(model.CoreSqlOrder{}).Select("source_id").Where("work_id =?", e.WorkId).First(&order)
	model.DB().Model(model.CoreDataSource{}).Select("flow_id").Where("source_id =?", order.SourceId).First(&s)
	model.DB().Model(model.CoreWorkflowTpl{}).Where("id =?", s.FlowID).First(&flow)
	_ = json.Unmarshal(flow.Steps, &tpl)
	return tpl
}

func ExecuteOrder(u *Confirm, user string) common.Resp {
	var order model.CoreSqlOrder
	var source model.CoreDataSource
	model.DB().Where("work_id =?", u.WorkId).First(&order)

	if order.Status != 2 && order.Status != 5 {
		return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ORDER_NOT_SEARCH))
	}
	order.Assigned = user

	model.DB().Model(model.CoreDataSource{}).Where("source_id =?", order.SourceId).First(&source)
	rule, err := factory.CheckDataSourceRule(source.RuleId)
	if err != nil || rule == nil {
		logger.DefaultLogger.Error(err)
		return common.ERR_COMMON_MESSAGE(err)
	}

	p := enc.Decrypt(model.C.General.SecretKey, source.Password)
	if p == "" {
		return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_KEY_DECRYPTION_FAILED))
	}

	var isCall bool
	client, err := calls.NewRpc()
	if err != nil {
		logger.DefaultLogger.Error(err)
		return common.ERR_COMMON_MESSAGE(err)
	}
	defer client.Close()
	if err := client.Call("Engine.Exec", &ExecArgs{
		Order:    &order,
		Rules:    *rule,
		IP:       source.IP,
		Port:     source.Port,
		Username: source.Username,
		Password: p,
		CA:       source.CAFile,
		Cert:     source.Cert,
		Key:      source.KeyFile,
		Message:  *model.GloMessage.Load(),
	}, &isCall); err != nil {
		return common.ERR_COMMON_MESSAGE(err)
	}
	model.DB().Create(&model.CoreWorkflowDetail{
		WorkId:   u.WorkId,
		Username: user,
		Time:     time.Now().Format("2006-01-02 15:04"),
		Action:   i18n.DefaultLang.Load(i18n.ORDER_EXECUTE_STATE),
	})
	return common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.ORDER_EXECUTE_STATE))
}

func MultiAuditOrder(req *Confirm, user string) common.Resp {
	if assigned, isExecute, ok := isNotIdempotent(req, user); ok {
		if isExecute {
			return ExecuteOrder(req, user)
		}
		model.DB().Model(model.CoreSqlOrder{}).Where("work_id = ?", req.WorkId).Updates(&model.CoreSqlOrder{CurrentStep: req.Flag + 1, Assigned: strings.Join(assigned, ",")})
		model.DB().Create(&model.CoreWorkflowDetail{
			WorkId:   req.WorkId,
			Username: user,
			Time:     time.Now().Format("2006-01-02 15:04"),
			Action:   fmt.Sprintf(i18n.DefaultLang.Load(i18n.ORDER_AGREE_MESSAGE), strings.Join(assigned, " ")),
		})
		pusher.NewMessagePusher(req.WorkId).Order().OrderBuild(pusher.NextStepStatus).Push()
		return common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.ORDER_AGREE_STATE))
	}
	return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ORDER_NOT_SEARCH))
}

func RejectOrder(req *Confirm, user string) common.Resp {
	// 驳回与同意同样需要校验：当前用户必须是该工单当前层级的审批人
	if _, _, ok := isNotIdempotent(req, user); !ok {
		return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_USER_NO_PERMISSION))
	}
	model.DB().Model(&model.CoreSqlOrder{}).Where("work_id =? AND `status` =?", req.WorkId, 2).Updates(map[string]interface{}{"status": 0})
	model.DB().Create(&model.CoreWorkflowDetail{
		WorkId:   req.WorkId,
		Username: user,
		Time:     time.Now().Format("2006-01-02 15:04"),
		Action:   i18n.DefaultLang.Load(i18n.ORDER_REJECT_MESSAGE),
	})
	model.DB().Create(&model.CoreOrderComment{
		WorkId:   req.WorkId,
		Username: user,
		Content:  fmt.Sprintf("驳回理由: %s", req.Text),
		Time:     time.Now().Format("2006-01-02 15:04"),
	})
	pusher.NewMessagePusher(req.WorkId).Order().OrderBuild(pusher.RejectStatus).Push()
	return common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.ORDER_REJECT_STATE))
}

func delayKill(workId string) string {
	model.DB().Model(&model.CoreSqlOrder{}).Where("work_id =? AND `status` =?", workId, 2).Updates(map[string]interface{}{"status": 4, "execute_time": time.Now().Format("2006-01-02 15:04")})
	return i18n.DefaultLang.Load(i18n.ORDER_DELAY_KILL_DETAIL)
}

// hasOrderPermission 判断用户是否有权操作该工单
func hasOrderPermission(workId, user string) bool {
	return common.IsOrderRelated(workId, user)
}

func isNotIdempotent(r *Confirm, user string) ([]string, bool, bool) {
	if r.Flag < 0 {
		return nil, false, false
	}
	tpl := r.GetTPL()
	if len(tpl) > r.Flag {
		pList := strings.Join(tpl[r.Flag].Auditor, ",")
		if !strings.Contains(pList, user) {
			return nil, false, false
		}
		if r.Flag+1 == len(tpl) {
			return tpl[r.Flag].Auditor, true, true
		}
		return tpl[r.Flag+1].Auditor, false, true
	}
	return nil, false, false
}
