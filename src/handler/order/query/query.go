package query

import (
	"Yearning-go/src/handler/common"
	"Yearning-go/src/handler/order/audit"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/lib/pusher"
	"Yearning-go/src/model"
	"encoding/json"
	"errors"
	"github.com/cookieY/yee"
	"github.com/golang-jwt/jwt"
	"golang.org/x/net/websocket"
	"gorm.io/gorm"
	"io"
	"net/http"
	"time"
)

func FetchQueryOrder(c yee.Context) (err error) {
	websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()
		var u common.PageList[[]model.CoreQueryOrder]
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
			is_record, _ := claims["is_record"].(bool)
			name, ok := claims["name"].(string)
			if !ok {
				break
			}

			u.Paging().OrderBy("(status = 2) DESC, date DESC").Query(
				common.AccordingQueryToAssigned(c.QueryParam("tp") != "record" && is_record, name),
				common.AccordingToUsername(u.Expr.Username),
				common.AccordingToRealName(u.Expr.RealName),
				common.AccordingToDate(u.Expr.Picker),
				common.AccordingToWorkId(u.Expr.WorkId),
				common.AccordingToAllQueryOrderState(u.Expr.Status),
			)
			if err = websocket.Message.Send(ws, factory.ToJson(u.ToMessage())); err != nil {
				c.Logger().Error(err)
				break
			}
		}
	}).ServeHTTP(c.Response(), c.Request())
	return nil
}

func FetchQueryRecordProfile(c yee.Context) (err error) {
	u := new(audit.Confirm)
	if err = c.Bind(u); err != nil {
		return
	}
	// 查询明细含用户执行过的全部 SQL，仅工单归属人、审批人或审计员可查看
	token := new(factory.Token).JwtParse(c)
	var order model.CoreQueryOrder
	if err := model.DB().Model(model.CoreQueryOrder{}).Select("username,assigned").Where("work_id =?", u.WorkId).First(&order).Error; err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE)))
	}
	if order.Username != token.Username && order.Assigned != token.Username && !token.IsRecord {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_USER_NO_PERMISSION)))
	}
	start, end := factory.Paging(u.Page, 15)
	l := new(common.GeneralList[[]model.CoreQueryRecord])
	model.DB().Model(&model.CoreQueryRecord{}).Where("work_id =?", u.WorkId).Count(&l.Page).Offset(start).Limit(end).Find(&l.Data)
	return c.JSON(http.StatusOK, l.ToMessage())
}

func QueryDeleteEmptyRecord(c yee.Context) (err error) {
	var j []model.CoreQueryOrder
	model.DB().Select("work_id").Where("`status` =?", 3).Find(&j)
	for _, i := range j {
		var k model.CoreQueryRecord
		if err := model.DB().Where("work_id =?", i.WorkId).First(&k).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			model.DB().Where("work_id =?", i.WorkId).Delete(&model.CoreQueryOrder{})
		}
	}
	return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.INFO_ORDER_IS_CLEAR)))
}

func QueryHandlerSets(c yee.Context) (err error) {
	u := new(common.QueryOrder)
	if err = c.Bind(u); err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	token := new(factory.Token).JwtParse(c)
	empty := new(model.CoreQueryOrder)
	found := model.DB().Where("work_id=? AND status=? AND assigned = ?", u.WorkId, 1, token.Username).Find(empty).Error
	switch c.Params("tp") {
	case "agreed":
		if !errors.Is(found, gorm.ErrRecordNotFound) {
			model.DB().Model(model.CoreQueryOrder{}).Where("work_id =?", u.WorkId).Updates(&model.CoreQueryOrder{Status: 2, ApprovalTime: time.Now().Format("2006-01-02 15:04")})
			pusher.NewMessagePusher(u.WorkId).Query().QueryBuild(pusher.AgreeStatus).Push()
			return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.INFO_ORDER_IS_AGREE)))
		}
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE)))
	case "reject":
		if !errors.Is(found, gorm.ErrRecordNotFound) {
			model.DB().Model(model.CoreQueryOrder{}).Where("work_id =?", u.WorkId).Updates(&model.CoreQueryOrder{Status: 4})
			pusher.NewMessagePusher(u.WorkId).Query().QueryBuild(pusher.RejectStatus).Push()
			return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.INFO_ORDER_IS_REJECT)))
		}
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE)))
	case "undo":
		t := new(factory.Token)
		t.JwtParse(c)
		var order model.CoreQueryOrder
		model.DB().Model(model.CoreQueryOrder{}).Select("work_id").Where("username =?", t.Username).Last(&order)
		model.DB().Model(model.CoreQueryOrder{}).Where("work_id =?", order.WorkId).Updates(&model.CoreSqlOrder{Status: 3})
		return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.INFO_ORDER_IS_END)))
	case "stop":
		// 仅能终止自己提交或指派给自己的查询工单，且不得影响他人
		model.DB().Model(model.CoreQueryOrder{}).
			Where("work_id =? AND (username =? OR assigned =?)", u.WorkId, token.Username, token.Username).
			Updates(&model.CoreQueryOrder{Status: 3})
		return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.INFO_ORDER_IS_END)))
	case "cancel":
		// 必须限定到当前用户，否则会把全表查询工单置为结束
		model.DB().Model(model.CoreQueryOrder{}).Where("username =?", token.Username).Updates(&model.CoreQueryOrder{Status: 3})
		return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.INFO_ORDER_IS_ALL_END)))
	default:
		return
	}
}

func AuditQueryOrderProfileFetchApis(c yee.Context) (err error) {
	switch c.Params("tp") {
	case "profile":
		return FetchQueryRecordProfile(c)
	default:
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE)))
	}
}

func AuditQueryOrderApis(c yee.Context) (err error) {
	return FetchQueryOrder(c)
}
