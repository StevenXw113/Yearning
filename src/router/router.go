// Copyright 2019 HenryYee.
//
// Licensed under the AGPL, Version 3.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    https://www.gnu.org/licenses/agpl-3.0.en.html
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package router

import (
	"Yearning-go/src/apis"
	"Yearning-go/src/handler/fetch"
	"Yearning-go/src/handler/login"
	"Yearning-go/src/handler/manage"
	autoTask2 "Yearning-go/src/handler/manage/autoTask"
	db2 "Yearning-go/src/handler/manage/db"
	"Yearning-go/src/handler/manage/flow"
	group2 "Yearning-go/src/handler/manage/group"
	roles2 "Yearning-go/src/handler/manage/roles"
	"Yearning-go/src/handler/manage/settings"
	user2 "Yearning-go/src/handler/manage/user"
	audit2 "Yearning-go/src/handler/order/audit"
	query2 "Yearning-go/src/handler/order/query"
	"Yearning-go/src/handler/order/record"
	"Yearning-go/src/handler/personal"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/model"
	"net/http"

	"github.com/cookieY/yee"
	"github.com/cookieY/yee/middleware"
	"github.com/golang-jwt/jwt"
)

func SuperManageGroup() yee.HandlerFunc {
	return func(c yee.Context) (err error) {
		role := new(factory.Token).JwtParse(c)
		if role.Username == "admin" || focalPoint(c) {
			return
		}
		return c.ServerError(http.StatusForbidden, "非法越权操作！")
	}
}

func focalPoint(c yee.Context) bool {
	// 必须按路径精确比对：使用 strings.Contains 会被查询串注入绕过
	// （例如 /api/v2/manage/setting?x=/api/v2/manage/group）
	path := c.Request().URL.Path
	method := c.Request().Method

	if path == "/api/v2/manage/flow" && method == http.MethodPut {
		return true
	}

	if path == "/api/v2/manage/group" && method == http.MethodGet {
		return true
	}
	return false
}

func SuperRecorderGroup() yee.HandlerFunc {
	return func(c yee.Context) (err error) {
		// websocket 请求无法走 JWT 中间件注入的 claims，需自行解析 Sec-WebSocket-Protocol 上的令牌
		if c.IsWebsocket() {
			token, terr := factory.WsTokenParse(c.Request().Header.Get(yee.HeaderSecWebSocketProtocol))
			if terr != nil || token == nil || !token.Valid {
				return c.ServerError(http.StatusForbidden, "Non-authorized operation！")
			}
			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				return c.ServerError(http.StatusForbidden, "Non-authorized operation！")
			}
			if isRecord, ok := claims["is_record"].(bool); ok && isRecord {
				return nil
			}
			return c.ServerError(http.StatusForbidden, "Non-authorized operation！")
		}
		role := new(factory.Token).JwtParse(c)
		if role.IsRecord {
			return
		}
		return c.ServerError(http.StatusForbidden, "Non-authorized operation！")
	}
}

func AddRouter(e *yee.Core) {
	e.POST("/login", login.UserGeneralLogin)
	e.POST("/register", login.UserRegister)
	e.GET("/fetch", login.UserReqSwitch)
	e.GET("/lang", login.SystemLang)
	e.GET("/oidc/_token-login", login.OidcLogin)
	e.GET("/oidc/state", login.OidcState)
	// 聊天页用一次性交换码换取 JWT：接口不带 /api/v2 前缀、不受应用 JWT 组保护，以便跨源内嵌页可达。
	// 安全靠随机 + 单次 + 短时效（60s）交换码，而非把长期 token 放进 URL。
	e.POST("/chatbot/session", fetch.ChatSessionExchange)
	// 校验 key 必须与 factory.JwtAuth 的签名 key 一致：两者都用派生自 SecretKey 的
	// JwtSigningKey()，否则登录签发的 token 会被判 signature is invalid，导致所有 /api/v2 请求 401
	r := e.Group("/api/v2", middleware.JWTWithConfig(middleware.JwtConfig{SigningKey: model.JwtSigningKey(), TokenLookup: []string{yee.HeaderAuthorization, yee.HeaderSecWebSocketProtocol}}))
	r.POST("/chat", fetch.AiChat)
	// 已登录主应用调用：为内嵌聊天页签发一次性交换码（不放 JWT 进 URL）
	r.POST("/chat/session", fetch.ChatSessionIssue)
	r.Restful("/common/:tp", personal.PersonalRestFulAPis())
	r.Restful("/dash/:tp", apis.YearningDashApis())
	r.Restful("/fetch/:tp", apis.YearningFetchApis())
	r.Restful("/query/:tp", apis.YearningQueryApis())
	r.GET("/board/get", manage.GeneralGetBoard)

	audit := r.Group("/audit")
	audit.Restful("/order/:tp", audit2.AuditRestFulAPis())
	//audit.Restful("/osc/:work_id", osc.AuditOSCFetchStateApis())
	audit.Restful("/query/:tp", query2.AuditQueryRestFulAPis())

	re := r.Group("/record", SuperRecorderGroup())
	re.GET("/axis", record.RecordDashAxis)
	re.GET("/list", record.RecordOrderList)

	manager := r.Group("/manage", SuperManageGroup())
	manager.POST("/board/post", manage.GeneralPostBoard)
	manager.GET("/board/get", manage.GeneralGetBoard)

	db := manager.Group("/db")
	db.Restful("", db2.ManageDbApi())

	account := manager.Group("/user")
	account.Restful("", user2.SuperUserApi())

	tpl := manager.Group("/tpl")
	tpl.Restful("", flow.TplRestApis())

	group := manager.Group("/policy")
	group.Restful("", group2.GroupsApis())
	group.GET("/source", group2.SuperGetRuseSource)

	setting := manager.Group("/setting")
	setting.Restful("", settings.SettingsApis())

	roles := manager.Group("/roles/:tp")
	roles.Restful("", roles2.RolesApis())

	autoTask := manager.Group("/task")
	autoTask.Restful("", autoTask2.SuperAutoTaskApis())
}
