package personal

import (
	"Yearning-go/src/handler/common"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/model"
	"encoding/json"
	"github.com/cookieY/yee"
	"github.com/golang-jwt/jwt"
	"golang.org/x/net/websocket"
	"io"
	"net/http"
)

func PersonalFetchMyOrder(c yee.Context) (err error) {
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
			u.Paging().OrderBy("(status = 2) DESC, date DESC").Select(common.QueryField).Query(
				common.AccordingToAllOrderType(u.Expr.Type),
				common.AccordingToAllOrderState(u.Expr.Status),
				common.AccordingToUsernameEqual(user),
				common.AccordingToDate(u.Expr.Picker),
				common.AccordingToText(u.Expr.Text),
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

func editPersonalUser(c yee.Context) (err error) {
	// 创建一个新的 CoreAccount 结构体实例
	userDetails := new(model.CoreAccount)

	// 绑定请求数据到 CoreAccount 结构体
	if err = c.Bind(userDetails); err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}

	// 准备更新的数据
	updateFields := &model.CoreAccount{
		Email:      userDetails.Email,
		RealName:   userDetails.RealName,
		Department: userDetails.Department,
	}

	// 如果密码不为空，则进行加密并更新
	if userDetails.Password != "" {
		updateFields.Password = factory.DjangoEncrypt(userDetails.Password, string(factory.GetRandom()))
	}

	// 更新数据库中的用户数据
	model.DB().Model(&model.CoreAccount{}).Where("username = ?", new(factory.Token).JwtParse(c).Username).Updates(updateFields)

	// 返回成功的消息
	return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.CUSTOM_PASSWORD_SUCCESS)))
}

func GET(c yee.Context) (err error) {
	switch c.Params("tp") {
	case "list":
		return PersonalFetchMyOrder(c)
	default:
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE)))
	}
}
