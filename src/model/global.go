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
	RpcAddr   string
	LogLevel  string
	Lang      string
	// RpcToken 审核引擎 gRPC 的共享访问令牌，须与引擎侧启动参数
	// (-auth-token 或环境变量 ENGINE_AUTH_TOKEN) 一致；空 = 引擎未启用认证。
	// 引擎是「拿到数据源凭据即可执行 SQL」的代理，跨不完全可信网络部署必须配置。
	RpcToken string
	// DisableInstanceLock 关闭单实例互斥（默认开启）。cron 统计、OSC 中止、
	// 执行中超时自愈等均假设同一元数据库只有一个主实例在跑，多实例会互相破坏；
	// 互斥基于 MySQL GET_LOCK，仅特殊部署（如主动外部调度）才应关闭。
	DisableInstanceLock bool
	// ExecTimeoutMinutes 单工单引擎执行超时（分钟），默认 5。0 = 不限制——
	// 大批量 DML / 未走 OSC 的大 DDL 可能需要更久，被 5 分钟切断的 DDL 回滚代价很高。
	ExecTimeoutMinutes int
	// SelfHealHours 执行中(status=3)工单的僵尸自愈阈值（小时），默认 6。
	// 进程崩溃会留下永远「执行中」的工单；超过阈值且 gh-ost 不在本进程运行
	// （豁免长时 OSC）即置为失败并留痕。0 = 关闭自愈。
	SelfHealHours int
	// ArchiveDays 历史归档阈值（天），0 = 关闭（默认）。
	// 终态（驳回/完成/失败/撤销）且创建时间早于 N 天的工单连同全部明细、
	// 以及早于 N 天的查询审计日志，由每日归档 cron 迁移到同构 *_archive 表。
	// 列表因此只保留活跃数据；归档数据经审计记录的归档接口查询。
	ArchiveDays int
	// GhOstPath gh-ost 可执行文件路径；留空则从 PATH 中查找 gh-ost
	GhOstPath string
	// OscMinRows 触发在线表结构变更(OSC)的表行数估算下限；<=0 表示关闭 OSC（全部走引擎执行）
	OscMinRows int64
	// GhOstArgs gh-ost 附加参数（空格分隔），用于适配环境差异。
	// 例如数据源账号没有 performance_schema 权限时需加 "--skip-metadata-lock-check"
	GhOstArgs string
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
