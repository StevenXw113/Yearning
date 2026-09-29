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

package personal

import (
	"Yearning-go/src/handler/common"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/enc"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/lib/permission"
	"Yearning-go/src/lib/pusher"
	"Yearning-go/src/lib/vars"
	"Yearning-go/src/model"
	"errors"
	"github.com/cookieY/sqlx"
	"github.com/cookieY/yee"
	"github.com/golang-jwt/jwt"
	"github.com/vmihailenco/msgpack/v5"
	"golang.org/x/net/websocket"
	"gorm.io/gorm"
	"io"
	"net/http"
	"net/url"
	"time"
)

type queryResults struct {
	Export    bool     `msgpack:"export"`
	Error     string   `msgpack:"error"`
	Results   []*Query `msgpack:"results"`
	QueryTime int      `msgpack:"query_time"`
	Status    bool     `msgpack:"status"`
	HeartBeat string   `msgpack:"heartbeat"`
	IsOnly    bool     `msgpack:"is_only"`
	// Audit 查询侧规则命中的提示（只有 MongoDB 查询有）：不影响结果，前端以提示形式展示。
	// 命中拦截级规则时走 Error，这里只承载 warn / observe。
	Audit string `msgpack:"audit"`
}

type queryArgs struct {
	SourceId string `json:"source_id"`
}

type queryCore struct {
	db               *sqlx.DB
	insulateWordList string
	source           string
}

func boolToUint(flag bool) uint {
	if flag {
		return 1
	}
	return 0
}

func ReferQueryOrder(c yee.Context, user *factory.Token) (err error) {
	var t model.CoreQueryOrder
	d := new(common.QueryOrder)
	if err = c.Bind(d); err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	other := model.GloOther.Load()
	if !other.Query {
		o := model.CoreQueryOrder{
			Username:     user.Username,
			Date:         time.Now().Format("2006-01-02 15:04"),
			Export:       boolToUint(other.Export),
			Status:       2,
			RealName:     user.RealName,
			Text:         i18n.DefaultLang.Load(i18n.INFO_QUERY_AUDIT_DISABLED),
			Assigned:     "admin",
			ApprovalTime: time.Now().Format("2006-01-02 15:04"),
		}
		model.DB().Create(&o)
		// 编号 = 自增 id：先落库拿到 id 再回填
		model.DB().Model(&o).Update("work_id", factory.OrderNo(o.ID, 0))
		// 必须有回包：原先直接 return，前端拿到空响应，表现为「点了没反应」
		return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.INFO_QUERY_AUDIT_DISABLED)))
	}

	// 去重只针对「待审核」的申请：status=1 才是待审核，status=2 是已通过的查询权限
	// （SocketQueryResults 依据它判定该用户可查询哪个数据源）。用 status=2 去重会让
	// 曾经查询过的用户（含审核关闭时自动生成的工单）永远无法再提交申请。
	if err := model.DB().Model(model.CoreQueryOrder{}).Where("username =? and status =?", user.Username, 1).First(&t).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		var principal model.CoreDataSource
		model.DB().Model(model.CoreDataSource{}).Where("source_id = ?", d.SourceId).First(&principal)
		// 数据源未配置查询审批人时兜底给 admin：assigned 为空会让审批页按 assigned 过滤时
		// 谁都匹配不到，工单变成无人可见、无法审批的孤儿单。
		assigned := principal.Principal
		if assigned == "" {
			assigned = "admin"
		}
		o := model.CoreQueryOrder{
			Username: user.Username,
			Date:     time.Now().Format("2006-01-02 15:04"),
			Text:     d.Text,
			Export:   d.Export,
			Status:   1,
			SourceId: d.SourceId,
			Assigned: assigned,
			RealName: user.RealName,
		}
		model.DB().Create(&o)
		// 编号 = 自增 id：先落库拿到 id 再回填
		o.WorkId = factory.OrderNo(o.ID, 0)
		model.DB().Model(&o).Update("work_id", o.WorkId)
		pusher.NewMessagePusher(o.WorkId).Query().QueryBuild(pusher.SummitStatus).Push()
		return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.INFO_ORDER_IS_CREATE)))
	}
	return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.INFO_ORDER_IS_DUP)))
}

func FetchQueryDatabaseInfo(c yee.Context) (err error) {
	var u model.CoreDataSource

	model.DB().Where("source_id =?", c.QueryParam("source_id")).First(&u)

	// MongoDB 没有 SHOW DATABASES 这类元数据语句，走独立实现
	if u.DBType == model.DBTypeMongoDB {
		list, mongoErr := FetchMongoDatabaseInfo(&u)
		if mongoErr != nil {
			c.Logger().Error(mongoErr.Error())
			return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(mongoErr))
		}
		return c.JSON(http.StatusOK, common.SuccessPayload(list))
	}

	result, err := common.ScanDataRows(u, "", "SHOW DATABASES;", "Schema", true, false)

	if err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	return c.JSON(http.StatusOK, common.SuccessPayload(result.QueryList))
}

func FetchQueryTableInfo(c yee.Context) (err error) {

	t := c.QueryParam("schema")
	// todo source改方法 不然中文无法识别
	source := c.QueryParam("source_id")
	unescape, _ := url.QueryUnescape(source)

	var u model.CoreDataSource

	model.DB().Where("source_id =?", unescape).First(&u)

	if u.DBType == model.DBTypeMongoDB {
		list, mongoErr := FetchMongoCollectionInfo(&u, t)
		if mongoErr != nil {
			c.Logger().Error(mongoErr.Error())
			return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(mongoErr))
		}
		return c.JSON(http.StatusOK, common.SuccessPayload(map[string]interface{}{"table": list}))
	}

	result, err := common.ScanDataRows(u, t, "SHOW TABLES;", "Table", true, true)
	if err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	return c.JSON(http.StatusOK, common.SuccessPayload(map[string]interface{}{"table": result.QueryList}))

}

func SocketQueryResults(c yee.Context) (err error) {
	args := new(queryArgs)
	if err = c.Bind(args); err != nil {
		return
	}
	websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()
		var b []byte
		token, err := factory.WsTokenParse(ws.Request().Header.Get("Sec-WebSocket-Protocol"))
		if err != nil || token == nil || !token.Valid {
			c.Logger().Error(err)
			return
		}
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return
		}
		user, ok := claims["name"].(string)
		if !ok {
			return
		}

		// 开启查询审核模式后需判断当前连接的 sourceID用户是否有权限
		if model.GloOther.Load().Query {
			var queryPerm model.CoreQueryOrder
			model.DB().Model(model.CoreQueryOrder{}).Where("username =? AND status =?", user, 2).Last(&queryPerm)
			if queryPerm.SourceId != args.SourceId {
				c.Logger().Criticalf(i18n.DefaultLang.Load(i18n.ER_USER_NO_PERMISSION), user, args.SourceId)
				return
			}
		}

		if !permission.NewPermissionService(model.DB()).Equal(&permission.Control{User: user, Kind: vars.QUERY, SourceId: args.SourceId}) {
			c.Logger().Criticalf(i18n.DefaultLang.Load(i18n.ER_USER_NO_PERMISSION), user, args.SourceId)
			return
		}

		if token.Valid {
			msg := new(QueryDeal)
			core := new(queryCore)
			var u model.CoreDataSource
			model.DB().Where("source_id =?", args.SourceId).First(&u)
			// MongoDB 走独立执行器：连接与结果集都不经过 database/sql
			if u.DBType == model.DBTypeMongoDB {
				socketMongoResults(c, ws, u, user)
				return
			}
			dsn, err := model.InitDSN(model.DSN{
				Username: u.Username,
				Password: enc.Decrypt(model.C.General.SecretKey, u.Password),
				Host:     u.IP,
				Port:     u.Port,
				CA:       u.CAFile,
				Cert:     u.Cert,
				Key:      u.KeyFile,
			})
			if err != nil {
				c.Logger().Error(err)
				_ = websocket.Message.Send(ws, factory.ToMsg(queryResults{Error: err.Error()}))
				return
			}
			core.db, err = sqlx.Connect("mysql", dsn)
			if err != nil {
				c.Logger().Error(err)
				_ = websocket.Message.Send(ws, factory.ToMsg(queryResults{Error: err.Error()}))
				return
			}
			core.insulateWordList = u.InsulateWordList
			core.source = u.Source
			defer core.db.Close()
			for {
				if err := websocket.Message.Receive(ws, &b); err != nil {
					if err != io.EOF {
						c.Logger().Error(err)
					}
					break
				}
				if string(b) == "ping" {
					_ = websocket.Message.Send(ws, factory.ToMsg(queryResults{HeartBeat: common.Pong, IsOnly: model.GloOther.Load().Query}))
					continue
				}
				if err := msgpack.Unmarshal(b, &msg.Ref); err != nil {
					c.Logger().Error(err)
					break
				}
				msg.MultiSQLRunner = []MultiSQLRunner{}
				clock := time.Now()
				// 无已通过的申请、或申请已到期：回 status，由前端提示重新申请
				d, ok := permitQueryOrder(user)
				if !ok {
					if err := websocket.Message.Send(ws, factory.ToMsg(queryResults{Status: true})); err != nil {
						c.Logger().Error(err)
					}
					continue
				}

				var queryData []*Query

				if err := msg.PreCheck(core.insulateWordList); err != nil {
					if err := websocket.Message.Send(ws, factory.ToMsg(queryResults{Error: err.Error()})); err != nil {
						c.Logger().Error(err)
					}
					continue
				}

				for _, i := range msg.MultiSQLRunner {
					result, err := i.Run(core.db, msg.Ref.Schema)
					if err != nil {
						if err := websocket.Message.Send(ws, factory.ToMsg(queryResults{Error: err.Error()})); err != nil {
							c.Logger().Error(err)
						}
						continue
					}

					queryData = append(queryData, result)
				}

				queryTime := int(time.Since(clock).Seconds() * 1000)
				// SQL 查询侧没有规则审核（引擎 QueryRequest 不带规则集），留痕为空
				saveQueryRecord(d, msg.Ref.Sql, core.source, msg.Ref.Schema, queryTime, "")
				if err := websocket.Message.Send(ws, factory.ToMsg(queryResults{Export: d.Export == 1, Results: queryData, QueryTime: queryTime})); err != nil {
					c.Logger().Error(err)
				}
			}
		}

	}).ServeHTTP(c.Response(), c.Request())
	return nil
}

func UndoQueryOrder(c yee.Context) (err error) {
	user := new(factory.Token).JwtParse(c)
	model.DB().Model(model.CoreQueryOrder{}).Where("username =?", user.Username).Updates(map[string]interface{}{"status": 3})
	return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.INFO_ORDER_IS_END)))
}

// permitQueryOrder 取当前用户已通过的查询申请（status=2）。
// 申请超期时顺手置为已结束并返回 false；SQL 与 MongoDB 两条查询路径共用。
func permitQueryOrder(user string) (model.CoreQueryOrder, bool) {
	var d model.CoreQueryOrder
	if err := model.DB().Where("username =? AND status =?", user, 2).Last(&d).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return d, false
	}
	if factory.TimeDifference(d.ApprovalTime) {
		model.DB().Model(model.CoreQueryOrder{}).Where("username =?", user).Updates(&model.CoreQueryOrder{Status: 3})
		return d, false
	}
	return d, true
}

// saveQueryRecord 落一条查询审计记录。同步写：异步时进程异常退出会丢掉这条日志，
// 而查询日志本身就是审计依据，单行插入的开销可以忽略。
// audit 是查询侧规则审核的命中说明（SQL 路径传空串）。
func saveQueryRecord(d model.CoreQueryOrder, sql, source, schema string, cost int, audit string) {
	model.DB().Create(&model.CoreQueryRecord{
		WorkId: d.WorkId,
		SQL:    sql,
		ExTime: cost,
		Time:   time.Now().Format("2006-01-02 15:04"),
		Source: source,
		Schema: schema,
		Audit:  audit,
	})
}
