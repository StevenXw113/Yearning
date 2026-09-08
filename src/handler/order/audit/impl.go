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
	"context"
	"encoding/json"
	enginev1 "engine/gen/engine/v1"
	"errors"
	"fmt"
	"github.com/cookieY/yee/logger"
	"strings"
	"time"
)

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
	model.DB().Where("work_id =?", u.WorkId).First(&order)

	if order.Status != 2 && order.Status != 5 {
		return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ORDER_NOT_SEARCH))
	}
	if err := ExecuteWorkOrder(&order, user); err != nil {
		logger.DefaultLogger.Error(err)
		return common.ERR_COMMON_MESSAGE(err)
	}
	return common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.ORDER_EXECUTE_STATE))
}

// ExecuteWorkOrder 执行单个已通过审批/待执行(status 2 或 5)的工单。
// 被 HTTP 人工执行与延迟调度 cron 共用：读取数据源与规则，调用引擎 Exec，
// 并将逐条明细回写 core_sql_records 与 core_workflow_detail。
// actor 为操作者标识（审批人用户名，或延迟触发时的执行者）。
func ExecuteWorkOrder(order *model.CoreSqlOrder, actor string) error {
	var source model.CoreDataSource
	if order == nil || order.WorkId == "" {
		return errors.New(i18n.DefaultLang.Load(i18n.ORDER_NOT_SEARCH))
	}
	model.DB().Model(model.CoreDataSource{}).Where("source_id =?", order.SourceId).First(&source)
	rule, err := factory.CheckDataSourceRule(source.RuleId)
	if err != nil || rule == nil {
		return err
	}
	p := enc.Decrypt(model.C.General.SecretKey, source.Password)
	if p == "" {
		return errors.New(i18n.DefaultLang.Load(i18n.ER_KEY_DECRYPTION_FAILED))
	}

	client, conn, err := calls.NewClient()
	if err != nil {
		return err
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
	})
	if rep == nil || !rep.Ok {
		return calls.CombineReplyErr(rep, err)
	}
	// 将引擎返回的逐条执行明细回写到 core_sql_records，供工单详情展示。
	for _, r := range rep.Records {
		rec := engine.RecordFromProto(r)
		model.DB().Create(&model.CoreSqlRecord{
			WorkId:    order.WorkId,
			SQL:       rec.SQL,
			State:     rec.Status,
			Affectrow: rec.AffectRows,
			Time:      time.Now().Format("2006-01-02 15:04"),
			Error:     rec.Error,
		})
	}
	model.DB().Create(&model.CoreWorkflowDetail{
		WorkId:   order.WorkId,
		Username: actor,
		Time:     time.Now().Format("2006-01-02 15:04"),
		Action:   i18n.DefaultLang.Load(i18n.ORDER_EXECUTE_STATE),
	})
	return nil
}

func MultiAuditOrder(req *Confirm, user string) common.Resp {
	if assigned, isExecute, ok := isNotIdempotent(req, user); ok {
		if isExecute {
			// 末级审批通过：带延迟执行时间(delay!='none')的工单进入"等待延迟执行"(status=5)，
			// 由延迟调度 cron 到点触发；普通工单立即执行。
			var od model.CoreSqlOrder
			model.DB().Model(model.CoreSqlOrder{}).Select("delay").Where("work_id =?", req.WorkId).First(&od)
			if od.Delay != "" && od.Delay != "none" {
				model.DB().Model(model.CoreSqlOrder{}).Where("work_id =?", req.WorkId).Updates(map[string]interface{}{"status": 5})
				return common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.ORDER_AGREE_STATE))
			}
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
