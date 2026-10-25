package record

import (
	"Yearning-go/src/handler/common"
	"Yearning-go/src/model"
	"fmt"
	"github.com/cookieY/yee"
	"github.com/golang-jwt/jwt/v5"
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
	// 审计公共视图,不鉴权;项目级工单按项目聚合分页(同批子工单不跨页),见 common.GroupedOrderPage
	common.WsPageList(c, false, func(_ string, _ jwt.MapClaims, u *common.PageList[[]model.CoreSqlOrder]) {
		common.GroupedOrderPage(u, common.QueryField,
			common.AccordingToAllOrderType(u.Expr.Type),
			common.AccordingToAllOrderState(u.Expr.Status),
			common.AccordingToDate(u.Expr.Picker),
			common.AccordingToText(u.Expr.Text),
		)
	})
	return nil
}
