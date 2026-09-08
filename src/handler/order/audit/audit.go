package audit

import (
	"Yearning-go/src/handler/common"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/lib/pusher"
	"Yearning-go/src/model"
	"encoding/json"
	"github.com/cookieY/yee"
	"github.com/golang-jwt/jwt"
	"golang.org/x/net/websocket"
	"io"
	"net/http"
	"time"
)

const QueryField = "work_id, username, text, backup, date, real_name, `status`, `type`, `delay`, `source`, `source_id`,`id_c`,`data_base`,`table`,`execute_time`,assigned,current_step,relevant"

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
			return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_USER_NO_PERMISSION)))
		}
		pusher.NewMessagePusher(u.WorkId).Order().OrderBuild(pusher.UndoStatus).Push()
		model.DB().Model(model.CoreSqlOrder{}).Where("work_id =? AND `status` =?", u.WorkId, 2).Updates(&model.CoreSqlOrder{Status: 6})
		return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.INFO_ORDER_IS_UNDO)))
	case "agree":
		return c.JSON(http.StatusOK, MultiAuditOrder(u, user.Username))
	case "reject":
		return c.JSON(http.StatusOK, RejectOrder(u, user.Username))
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
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_USER_NO_PERMISSION)))
	}
	// 延迟调度在 Yearning 侧完成：'none'/空 表示立即执行，否则重排到新时间点由 cron 到点触发。
	if u.Delay == "" || u.Delay == "none" {
		var od model.CoreSqlOrder
		model.DB().Model(model.CoreSqlOrder{}).Where("work_id =?", u.WorkId).First(&od)
		if od.WorkId == "" {
			return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ORDER_NOT_SEARCH)))
		}
		if err := ExecuteWorkOrder(&od, user.Username); err != nil {
			return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
		}
		model.DB().Model(model.CoreSqlOrder{}).Where("work_id =?", u.WorkId).
			Updates(map[string]interface{}{"status": 1, "delay": "none", "execute_time": time.Now().Format("2006-01-02 15:04")})
		return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.INFO_ORDER_DELAY_SUCCESS)))
	}
	model.DB().Model(model.CoreSqlOrder{}).Where("work_id =?", u.WorkId).
		Updates(map[string]interface{}{"delay": u.Delay, "status": 5})
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
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_USER_NO_PERMISSION)))
	}
	model.DB().Create(&model.CoreWorkflowDetail{
		WorkId:   u.WorkId,
		Username: user.Username,
		Time:     time.Now().Format("2006-01-02 15:04"),
		Action:   i18n.DefaultLang.Load(i18n.ORDER_KILL_STATE),
	})
	return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(delayKill(u.WorkId)))
}

func FetchAuditOrder(c yee.Context) (err error) {
	websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()
		var u common.PageList[[]model.CoreSqlOrder]
		var b []byte
		for {
			if err := websocket.Message.Receive(ws, &b); err != nil {
				if err != io.EOF {
					c.Logger().Error(err)
				}
				break
			}
			if string(b) == "ping" {
				continue
			}
			if err := json.Unmarshal(b, &u); err != nil {
				c.Logger().Error(err)
				break
			}
			token, err := factory.WsTokenParse(ws.Request().Header.Get("Sec-WebSocket-Protocol"))
			if err != nil || token == nil || !token.Valid {
				c.Logger().Error(err)
				break
			}
			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				break
			}
			user, ok := claims["name"].(string)
			if !ok {
				break
			}
			u.Paging().OrderBy("(status = 2) DESC, date DESC").Select(QueryField).Query(common.AccordingToAllOrderState(u.Expr.Status),
				common.AccordingToAllOrderType(u.Expr.Type),
				common.AccordingToRelevant(user),
				common.AccordingToText(u.Expr.Text),
				common.AccordingToUsername(u.Expr.Username),
				common.AccordingToDate(u.Expr.Picker),
				common.AccordingToWorkId(u.Expr.WorkId),
			)
			if err = websocket.Message.Send(ws, factory.ToJson(u.ToMessage())); err != nil {
				c.Logger().Error(err)
				break
			}
		}
	}).ServeHTTP(c.Response(), c.Request())
	return nil
}

func FetchOSCAPI(c yee.Context) (err error) {
	workId := c.QueryParam("work_id")
	if !common.IsOrderRelated(workId, new(factory.Token).JwtParse(c).Username) {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_USER_NO_PERMISSION)))
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

func AuditOrderApis(c yee.Context) (err error) {
	switch c.Params("tp") {
	case "state":
		return AuditOrderState(c)
	case "kill":
		return DelayKill(c)
	case "scheduled":
		return ScheduledChange(c)
	default:
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE)))
	}
}

func AuditOrRecordOrderFetchApis(c yee.Context) (err error) {
	switch c.Params("tp") {
	//case "list":
	//	return FetchAuditOrder(c)
	//case "record":
	//	return FetchRecord(c)
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
	default:
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE)))
	}
}
