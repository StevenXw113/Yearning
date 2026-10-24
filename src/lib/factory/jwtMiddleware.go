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

package factory

import (
	"errors"
	"net/http"

	"github.com/cookieY/yee"
	jwt "github.com/golang-jwt/jwt/v5"
)

// authContextKey 认证中间件往 context 注入令牌用的键(与 yee 版中间件一致)。
const authContextKey = "auth"

// bearerScheme Authorization 头的方案前缀(yee 版按区分大小写精确匹配)。
const bearerScheme = "Bearer"

// JwtMiddleware 认证中间件,替代 yee 自带的 middleware.JWTWithConfig:
// yee 的实现内部锁定 golang-jwt v3(把 *jwtv3.Token 存进 context),是主模块
// 无法退役 v3 的唯一原因。本实现语义与 yee 版逐条对齐:
//   - 令牌提取:Authorization 头须带 "Bearer " 前缀;ws 升级请求浏览器会清掉
//     自定义头,从 Sec-WebSocket-Protocol 取裸令牌;
//   - 显式限定 HS256,防 alg 混淆;
//   - 提取失败 → 400,解析/校验失败 → 401(yee 版同款分档,前端按 401 登出);
//   - 成功后把 *jwt.Token(v5) 放进 context("auth"),JwtParse 按同版本断言。
func JwtMiddleware(signingKey interface{}) yee.HandlerFunc {
	keyFunc := func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok || token.Method.Alg() != "HS256" {
			return nil, errors.New("unexpected jwt signing method")
		}
		return signingKey, nil
	}
	return func(c yee.Context) (err error) {
		auth := c.Request().Header.Get(yee.HeaderAuthorization)
		if len(auth) > len(bearerScheme) && auth[:len(bearerScheme)] == bearerScheme {
			auth = auth[len(bearerScheme)+1:]
		} else if wsProto := c.Request().Header.Get(yee.HeaderSecWebSocketProtocol); wsProto != "" {
			auth = wsProto
		} else {
			return c.JSON(http.StatusBadRequest, "missing or malformed jwt")
		}
		token, perr := jwt.Parse(auth, keyFunc)
		if perr != nil || token == nil || !token.Valid {
			return c.JSON(http.StatusUnauthorized, "invalid or expired jwt")
		}
		c.Put(authContextKey, token)
		return
	}
}
