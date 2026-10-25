package common

import (
	"encoding/json"
	"io"

	"github.com/cookieY/yee"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/net/websocket"

	"Yearning-go/src/lib/factory"
)

// WsPageList 是列表页共用的 websocket 分页循环：收包 → 跳过 ping → 解码分页请求
// →（authed 时）解析 Subprotocol 头里的 JWT 取用户名 → 执行查询 → 回包。
// record 页是审计公共视图，不走鉴权（authed=false，claims/user 为零值）。
func WsPageList[T any](c yee.Context, authed bool, handle func(user string, claims jwt.MapClaims, u *PageList[T])) {
	websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()
		var u PageList[T]
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
			var user string
			var claims jwt.MapClaims
			if authed {
				token, err := factory.WsTokenParse(ws.Request().Header.Get("Sec-WebSocket-Protocol"))
				if err != nil || token == nil || !token.Valid {
					c.Logger().Error(err)
					break
				}
				var ok bool
				if claims, ok = token.Claims.(jwt.MapClaims); !ok {
					break
				}
				if user, ok = claims["name"].(string); !ok {
					break
				}
			}
			handle(user, claims, &u)
			if err := websocket.Message.Send(ws, factory.ToJson(u.ToMessage())); err != nil {
				c.Logger().Error(err)
				break
			}
		}
	}).ServeHTTP(c.Response(), c.Request())
}
