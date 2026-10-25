package audit

import (
	"Yearning-go/src/handler/common"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/lib/permission"
	"Yearning-go/src/lib/pusher"
	"Yearning-go/src/model"
	"github.com/cookieY/yee"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/net/websocket"
	"gorm.io/gorm"
	"net/http"
	"time"
)

// QueryField 审批列表字段。id 用于把项目的子工单按创建顺序排列；
// batch_id 供前端按项目（批次）聚合展示：先看项目，展开后逐条审核。
const QueryField = "id, work_id, username, text, backup, date, real_name, `status`, `type`, `delay`, `source`, `source_id`,`id_c`,`data_base`,`table`,`execute_time`,assigned,current_step,relevant,batch_id,tx_mode"

func AuditOrderState(c yee.Context) (err error) {
	u := new(Confirm)
	user := new(factory.Token).JwtParse(c)
	if err = c.Bind(u); err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}

	switch u.Tp {
	case "undo":
		if !hasOrderPermission(u.WorkId, user.Username) {
			return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_ORDER_NOT_RELATED)))
		}
		// 仅审核中(2)可撤销；0 行命中说明状态已被并发操作改变，不再推送/报成功
		res := model.DB().Model(model.CoreSqlOrder{}).
			Where("work_id =? AND `status` =?", u.WorkId, model.OrderStatusAuditing).
			Updates(&model.CoreSqlOrder{Status: model.OrderStatusUndo})
		if res.Error != nil || res.RowsAffected != 1 {
			return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.UNDO_MESSAGE_ERROR)))
		}
		pusher.NewMessagePusher(u.WorkId).Order().OrderBuild(pusher.UndoStatus).Push()
		return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.INFO_ORDER_IS_UNDO)))
	case "agree":
		return c.JSON(http.StatusOK, MultiAuditOrder(u, user.Username))
	case "reject":
		return c.JSON(http.StatusOK, RejectOrder(u, user.Username))
	case "execute":
		// 人工执行：仅工单相关人（申请人 / 审批链上的人）可触发；
		// 已通过但停在「等待执行」的工单（人工执行模式或延迟执行）由此处落地执行。
		if !hasOrderPermission(u.WorkId, user.Username) {
			return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_ORDER_NOT_RELATED)))
		}
		return c.JSON(http.StatusOK, ExecuteOrder(u, user.Username))
	case "retry":
		// 失败工单重试：重执未成功过的语句，不新建工单；相关人即可触发（与执行一致）
		if !hasOrderPermission(u.WorkId, user.Username) {
			return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_ORDER_NOT_RELATED)))
		}
		return c.JSON(http.StatusOK, RetryOrder(u, user.Username))
	case "skip":
		// 人工介入后跳过失败语句、继续执行后续语句（数据订正场景，与重试互补）
		if !hasOrderPermission(u.WorkId, user.Username) {
			return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_ORDER_NOT_RELATED)))
		}
		return c.JSON(http.StatusOK, SkipAndContinue(u, user.Username))
	case "mark":
		// 手动标记完成：SQL 在平台外人工处理后的回填出口。
		// 权限比执行更严（当前审批人/超管），在 MarkOrderFinished 内部校验
		return c.JSON(http.StatusOK, MarkOrderFinished(u, user.Username))
	default:
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE)))
	}
}

func ScheduledChange(c yee.Context) (err error) {
	u := new(Confirm)
	if err = c.Bind(u); err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	user := new(factory.Token).JwtParse(c)
	if !hasOrderPermission(u.WorkId, user.Username) {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_ORDER_NOT_RELATED)))
	}
	var od model.CoreSqlOrder
	model.DB().Model(model.CoreSqlOrder{}).Where("work_id =?", u.WorkId).First(&od)
	if od.WorkId == "" {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ORDER_NOT_SEARCH)))
	}
	// 仅「等待执行」(status=5，已审批通过)的工单可改期/立即执行。
	// 审核中或已终结的工单一律拒绝——立即执行分支会直接落库执行，不设状态门禁
	// 等于允许任何工单相关人绕过审批触发执行。
	if od.Status != 5 {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_ORDER_NOT_EXECUTABLE)))
	}
	// 延迟调度在 Yearning 侧完成：'none'/空 表示立即执行，否则重排到新时间点由 cron 到点触发。
	if u.Delay == "" || u.Delay == "none" {
		oscAsync, err := executeClaimedOrder(&od, user.Username, model.OrderStatusWaiting)
		if err != nil {
			return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
		}
		model.DB().Model(model.CoreSqlOrder{}).Where("work_id =?", u.WorkId).Update("delay", "none")
		if oscAsync {
			return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.ORDER_OSC_STARTED)))
		}
		return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.INFO_ORDER_DELAY_SUCCESS)))
	}
	res := model.DB().Model(model.CoreSqlOrder{}).
		Where("work_id =? AND `status` =?", u.WorkId, model.OrderStatusWaiting).
		Updates(map[string]interface{}{"delay": u.Delay})
	if res.Error != nil || res.RowsAffected != 1 {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_ORDER_NOT_EXECUTABLE)))
	}
	return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.INFO_ORDER_DELAY_SUCCESS)))
}

// DelayKill will stop delay order
func DelayKill(c yee.Context) (err error) {
	u := new(Confirm)
	if err = c.Bind(u); err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	user := new(factory.Token).JwtParse(c)
	if !hasOrderPermission(u.WorkId, user.Username) {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_ORDER_NOT_RELATED)))
	}
	// 终止两类目标：
	//  - 审核中/等待执行(2/5)的延迟工单：直接置为失败(4)
	//  - 正在执行的 OSC(gh-ost)：发 SIGTERM，gh-ost 自行清理影子表；工单状态由执行链路
	//    的 FinalizeExecution 落到 4，这里不抢写
	msg, killed := delayKill(u.WorkId)
	if KillOSC(u.WorkId) {
		killed = true
		msg = "已向 gh-ost 发送中止指令，gh-ost 退出后工单将标记为执行失败"
	}
	if !killed {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_ORDER_NOT_TERMINABLE)))
	}
	model.DB().Create(&model.CoreWorkflowDetail{
		WorkId:   u.WorkId,
		Username: user.Username,
		Time:     time.Now().Format("2006-01-02 15:04"),
		Action:   i18n.DefaultLang.Load(i18n.ORDER_KILL_STATE),
	})
	return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(msg))
}

func FetchAuditOrder(c yee.Context) (err error) {
	// 项目级工单以项目为分页单位（同一批的子工单不跨页），见 common.GroupedOrderPage
	common.WsPageList(c, true, func(user string, _ jwt.MapClaims, u *common.PageList[[]model.CoreSqlOrder]) {
		scopes := []func(*gorm.DB) *gorm.DB{
			common.AccordingToAllOrderState(u.Expr.Status),
			common.AccordingToAllOrderType(u.Expr.Type),
		}
		// 超级管理员拥有所有权限：可审所有工单，不按「相关人」过滤
		if !permission.IsSuperUser(user) {
			scopes = append(scopes, common.AccordingToRelevant(user))
		}
		scopes = append(scopes,
			common.AccordingToText(u.Expr.Text),
			common.AccordingToUsername(u.Expr.Username),
			common.AccordingToDate(u.Expr.Picker),
			common.AccordingToWorkId(u.Expr.WorkId),
			common.AccordingToSource(u.Expr.Source),
		)
		common.GroupedOrderPage(u, QueryField, scopes...)
	})
	return nil
}
func FetchOSCAPI(c yee.Context) (err error) {
	workId := c.QueryParam("work_id")
	if !common.IsOrderRelated(workId, new(factory.Token).JwtParse(c).Username) {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_ORDER_NOT_RELATED)))
	}
	websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()
		var msg string
		for {
			if workId != "" {
				var osc model.CoreSqlOrder
				model.DB().Model(model.CoreSqlOrder{}).Where("work_id =?", workId).Find(&osc)
				err := websocket.Message.Send(ws, osc.OSCInfo)
				if err != nil {
					c.Logger().Error(err)
					break
				}
			}
			if err := websocket.Message.Receive(ws, &msg); err != nil {
				break
			}
		}
	}).ServeHTTP(c.Response(), c.Request())
	return nil
}

// FetchRetryPreview 重试预览（调试视图）：展示失败语句明细与重试将要执行的剩余 SQL，
// 让操作者在点「重试」前看清「错在哪、会重跑什么」。相关人可查，仅失败(4)工单有内容。
func FetchRetryPreview(c yee.Context) (err error) {
	workId := c.QueryParam("work_id")
	if workId == "" {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE)))
	}
	user := new(factory.Token).JwtParse(c)
	if !hasOrderPermission(workId, user.Username) {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_ORDER_NOT_RELATED)))
	}
	var od model.CoreSqlOrder
	model.DB().Where("work_id =?", workId).First(&od)
	if od.WorkId == "" {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ORDER_NOT_SEARCH)))
	}
	if od.Status != model.OrderStatusFailed {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_ORDER_NOT_RETRYABLE)))
	}
	// 全量执行记录（含历次重试）按时间排序：失败明细直接展示；
	// 剩余 SQL 用与 RetryOrder 完全相同的定位逻辑，预览即所得
	var records []model.CoreSqlRecord
	model.DB().Model(model.CoreSqlRecord{}).Where("work_id =?", workId).Order("id asc").Find(&records)
	var executed, failed []model.CoreSqlRecord
	for _, r := range records {
		if r.State == "已执行" {
			executed = append(executed, r)
		} else {
			failed = append(failed, r)
		}
	}
	remain, remainErr := remainingSQL(od.SQL, executed)
	// 「跳过并继续」视图：失败语句视为已人工处理，取其后的剩余语句（remainingSQLAfter）
	skipRemain, skipErr := remainingSQLAfter(od.SQL, records)
	payload := map[string]interface{}{
		"remaining":      remain,
		"remaining_skip": skipRemain,
		"executed":       executed,
		"failed":         failed,
	}
	if remainErr != nil {
		// 定位失败不拦接口：把原因带给前端，弹窗里说明并禁用重试按钮
		payload["error"] = remainErr.Error()
	}
	if skipErr == nil {
		payload["skip_available"] = true
	} else {
		payload["skip_available"] = false
	}
	return c.JSON(http.StatusOK, common.SuccessPayload(payload))
}

func AuditOrderApis(c yee.Context) (err error) {
	switch c.Params("tp") {
	case "state":
		return AuditOrderState(c)
	case "kill":
		return DelayKill(c)
	case "scheduled":
		return ScheduledChange(c)
	case "batch":
		return BatchExecute(c)
	default:
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE)))
	}
}

func AuditFetchApis(c yee.Context) (err error) {
	switch c.Params("tp") {
	case "osc":
		return FetchOSCAPI(c)
	case "kill":
		return nil
	case "list":
		return FetchAuditOrder(c)
	case "retry_preview":
		return FetchRetryPreview(c)
	default:
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE)))
	}
}
