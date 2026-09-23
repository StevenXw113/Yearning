package record

import (
	"Yearning-go/src/handler/common"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/model"
	"encoding/json"
	"fmt"
	"github.com/cookieY/yee"
	"golang.org/x/net/websocket"
	"io"
	"net/http"
	"time"
)

type groupBy struct {
	C    int    `json:"count"`
	Time string `json:"time"`
	Type string `json:"type"`
}

func timeAdd(add string) string {
	m, _ := time.ParseDuration(fmt.Sprintf("%sh", add))
	return time.Now().Add(m).Format("2006-01-02")
}

func RecordDashAxis(c yee.Context) (err error) {
	var order []groupBy
	model.DB().Model(model.CoreSqlOrder{}).Select("substring(date,1,10) as `time`, count(*) as c,type").Where("date > ?", timeAdd("-2160")).Group("substring(date,1,10) ,`type`").Scan(&order)
	return c.JSON(http.StatusOK, common.SuccessPayload(order))
}

func RecordOrderList(c yee.Context) (err error) {
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
			if err != nil {
				c.Logger().Error(err)
				break
			}
			// 项目级工单按项目聚合分页（同批子工单不跨页），见 common.GroupedOrderPage
			common.GroupedOrderPage(&u, common.QueryField,
				common.AccordingToAllOrderType(u.Expr.Type),
				common.AccordingToAllOrderState(u.Expr.Status),
				common.AccordingToDate(u.Expr.Picker),
				common.AccordingToText(u.Expr.Text),
			)
			if err = websocket.Message.Send(ws, factory.ToJson(u.ToMessage())); err != nil {
				c.Logger().Error(err)
				break
			}
		}
	}).ServeHTTP(c.Response(), c.Request())
	return nil
}
