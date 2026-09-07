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

package model

import (
	"Yearning-go/src/engine"
	"Yearning-go/src/lib/enc"
	"github.com/cookieY/yee/logger"
	"sync/atomic"
	"time"
)

var mappingLevel = map[string]uint8{
	"critical": 0,
	"error":    1,
	"warning":  2,
	"info":     3,
	"debug":    4,
}

type mysql struct {
	Host     string
	User     string
	Password string
	Db       string
	Port     string
}

type general struct {
	SecretKey string
	Host      string
	Hours     time.Duration
	RpcAddr   string
	LogLevel  string
	Lang      string
}

type DbInfo struct {
	Host     string
	User     string
	Password string
	Port     string
	Db       string
}

type oidc struct {
	Enable       bool
	ClientId     string
	ClientSecret string
	Scope        string
	AuthUrl      string
	TokenUrl     string
	UserUrl      string
	RedirectUrL  string
	SessionKey   string

	UserNameKey string
	RealNameKey string
	EmailKey    string
}

type Config struct {
	General general
	Mysql   mysql
	Oidc    oidc
}

var C Config

var DefaultLogger logger.Logger

var SecretKey = ""

var GloPer CoreGlobalConfiguration

// 全局配置会在运行时被管理端接口修改，同时被请求路径并发读取。
// 使用原子指针整体替换，避免结构体（内含 slice）赋值时读到撕裂值。
var (
	GloAI      atomic.Pointer[AI]
	GloOther   atomic.Pointer[Other]
	GloMessage atomic.Pointer[Message]
	GloRole    atomic.Pointer[engine.AuditRole]
)

func init() {
	GloAI.Store(&AI{})
	GloOther.Store(&Other{})
	GloMessage.Store(&Message{})
	GloRole.Store(&engine.AuditRole{})
}

// JwtSigningKey 派生专用于 JWT 签名的密钥。
// 与数据源口令加密密钥分离，避免一处泄露同时危及令牌与数据库凭据。
func JwtSigningKey() []byte {
	return enc.DeriveKey(C.General.SecretKey, "yearning-jwt-hs256-v1", 32)
}

func TransferLogLevel() uint8 {
	v, ok := mappingLevel[C.General.LogLevel]
	if !ok {
		return 3
	}
	return v
}
