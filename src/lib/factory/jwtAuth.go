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
	"Yearning-go/src/model"
	"errors"
	"time"

	"github.com/cookieY/yee"
	jwt "github.com/golang-jwt/jwt/v5"
)

type Token struct {
	Username string
	RealName string
	IsRecord bool
}

// JwtAuth 签发登录令牌（HS256，8 小时）。
// v5 产物与 v3 在线上格式上完全一致（JWT 标准），yee 中间件照常解析校验。
func JwtAuth(h Token) (t string, err error) {
	claims := jwt.MapClaims{
		"name":      h.Username,
		"real_name": h.RealName,
		"is_record": h.IsRecord,
		"iat":       time.Now().Unix(),
		"exp":       time.Now().Add(time.Hour * 8).Unix(),
	}
	t, err = jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(model.JwtSigningKey())
	if err != nil {
		return "", errors.New("JWT Generate Failure")
	}
	return t, nil
}

// JwtParse 从请求上下文中提取身份（token 由 JwtMiddleware 注入，v5）。
// 任何一步失败都返回零值 Token，调用方据此判断为未认证，而不是让类型断言 panic。
func (h *Token) JwtParse(c yee.Context) *Token {
	user, ok := c.Get(authContextKey).(*jwt.Token)
	if !ok || user == nil || !user.Valid {
		return h
	}
	claims, ok := user.Claims.(jwt.MapClaims)
	if !ok {
		return h
	}
	if v, ok := claims["name"].(string); ok {
		h.Username = v
	}
	if v, ok := claims["real_name"].(string); ok {
		h.RealName = v
	}
	if v, ok := claims["is_record"].(bool); ok {
		h.IsRecord = v
	}
	return h
}

func (h *Token) IsAdmin() bool {
	return h.Username == "admin"
}

// WsTokenParse 解析 websocket 子协议头中携带的令牌（v5）
func WsTokenParse(token string) (*jwt.Token, error) {
	return jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		// 显式限定 HMAC，防止 alg 混淆（如伪造 alg=none 或 RS256）
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return model.JwtSigningKey(), nil
	})
}
