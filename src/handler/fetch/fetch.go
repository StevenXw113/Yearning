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

package fetch

import (
	"Yearning-go/src/engine"
	"Yearning-go/src/handler/common"
	"Yearning-go/src/handler/manage/flow"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/calls"
	"Yearning-go/src/lib/enc"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/lib/permission"
	"Yearning-go/src/lib/pusher"
	"Yearning-go/src/model"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cookieY/yee"
	"golang.org/x/net/websocket"
	"gorm.io/gorm"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"
)

func FetchIDC(c yee.Context) (err error) {
	return c.JSON(http.StatusOK, common.SuccessPayload(model.GloOther.Load().IDC))

}

func FetchIsQueryAudit(c yee.Context) (err error) {
	return c.JSON(http.StatusOK, common.SuccessPayload(map[string]interface{}{
		"status": model.GloOther.Load().Query,
		"export": model.GloOther.Load().Export,
	}))
}

func FetchQueryStatus(c yee.Context) (err error) {
	var check model.CoreQueryOrder
	t := new(factory.Token).JwtParse(c)
	model.DB().Model(model.CoreQueryOrder{}).Where("username =?", t.Username).Last(&check)
	if check.Status == 2 {
		isExpire := factory.TimeDifference(check.ApprovalTime)
		if isExpire {
			model.DB().Model(model.CoreQueryOrder{}).Where("work_id =?", check.WorkId).Updates(&model.CoreQueryOrder{Status: 3})
		}
		return c.JSON(http.StatusOK, common.SuccessPayload(isExpire))
	}

	return c.JSON(http.StatusOK, common.SuccessPayload(true))
}

func FetchSource(c yee.Context) (err error) {
	u := new(_FetchBind)
	if err := c.Bind(u); err != nil {
		fmt.Println(err)
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	if reflect.DeepEqual(u, _FetchBind{}) {
		return
	}

	var grained model.CoreGrained
	var groupIDs []string
	var source []model.CoreDataSource

	user := new(factory.Token).JwtParse(c).Username
	model.DB().Where("username =?", user).First(&grained)
	if err := grained.Group.UnmarshalToJSON(&groupIDs); err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	permission := permission.NewPermissionService(model.DB()).CreatePermissionListFromGroups(groupIDs)
	switch u.Tp {
	case "count":
		return c.JSON(http.StatusOK, common.SuccessPayload(map[string]interface{}{"ddl": len(permission.DDLSource), "dml": len(permission.DMLSource), "query": len(permission.QuerySource)}))
	case "dml":
		model.DB().Select("source,id_c,source_id").Where("source_id IN (?)", permission.DMLSource).Find(&source)
	case "ddl":
		model.DB().Select("source,id_c,source_id").Where("source_id IN (?)", permission.DDLSource).Find(&source)
	case "query":
		var ord model.CoreQueryOrder
		// 如果打开查询审核,判断该用户是否存在查询中的工单.如果存在则直接返回该查询工单允许的数据源
		if model.GloOther.Load().Query && !errors.Is(model.DB().Model(model.CoreQueryOrder{}).Where("username =? and `status` =2", user).Last(&ord).Error, gorm.ErrRecordNotFound) {
			model.DB().Select("source,id_c,source_id").Where("source_id =?", ord.SourceId).Find(&source)
		} else {
			model.DB().Select("source,id_c,source_id").Where("source_id IN (?)", permission.QuerySource).Find(&source)
		}
	case "all":
		model.DB().Select("source,id_c,source_id").Find(&source)
	case "idc":
		model.DB().Select("source,source_id").Where("id_c = ?", u.IDC).Find(&source)
	}
	return c.JSON(http.StatusOK, common.SuccessPayload(source))
}

type StepInfo struct {
	flow.Tpl
	model.CoreWorkflowDetail
}

func FetchAuditSteps(c yee.Context) (err error) {
	u := c.QueryParam("source_id")
	workId := c.QueryParam("work_id")
	if workId != "" && !checkOrderPerm(c, workId) {
		return deny(c)
	}
	var order model.CoreSqlOrder
	var s []model.CoreWorkflowDetail
	var steps []StepInfo
	model.DB().Where("work_id = ?", workId).Find(&s)
	model.DB().Select("status").Where("work_id = ?", c.QueryParam("work_id")).First(&order)
	if order.Status == 2 || order.Status == 3 || order.Status == 5 || workId == "" {
		unescape, _ := url.QueryUnescape(u)
		whoIsAuditor, err := flow.OrderRelation(unescape)
		if err != nil {
			return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
		}

		for _, v := range whoIsAuditor {
			steps = append(steps, StepInfo{Tpl: v})
		}
		// 流程模板可能在工单创建后被改短，明细条数可能多于模板步数，直接索引会越界
		for i, v := range s {
			if i < len(steps) {
				steps[i].CoreWorkflowDetail = v
			} else {
				steps = append(steps, StepInfo{CoreWorkflowDetail: v})
			}
		}

	} else {
		for _, i := range s {
			steps = append(steps, StepInfo{Tpl: flow.Tpl{Desc: i.Action, Auditor: []string{i.Username}}, CoreWorkflowDetail: i})
		}
	}
	return c.JSON(http.StatusOK, common.SuccessPayload(steps))

}

func FetchHighLight(c yee.Context) (err error) {
	sourceId := c.QueryParam("source_id")
	if !checkSourcePerm(c, sourceId) {
		return deny(c)
	}
	var s model.CoreDataSource
	model.DB().Where("source_id =?", sourceId).First(&s)
	return c.JSON(http.StatusOK, common.SuccessPayload(common.Highlight(&s, c.QueryParam("is_field"), c.QueryParam("schema"))))
}

func FetchBase(c yee.Context) (err error) {

	u := new(_FetchBind)
	if err := c.Bind(u); err != nil {
		return err
	}
	if reflect.DeepEqual(u, _FetchBind{}) {
		return
	}
	unescape, _ := url.QueryUnescape(u.SourceId)
	if !checkSourcePerm(c, unescape) {
		return deny(c)
	}

	var s model.CoreDataSource

	model.DB().Where("source_id =?", unescape).First(&s)

	result, err := common.ScanDataRows(s, "", "SHOW DATABASES;", "Schema", false, false)
	if err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	if u.Hide {
		var _t []string
		mp := factory.MapOn(strings.Split(s.ExcludeDbList, ","))
		for _, i := range result.Results {
			if _, ok := mp[i]; !ok {
				_t = append(_t, i)
			}
		}
		model.DefaultLogger.Debugf("hide db list: %v ", _t)
		result.Results = _t
	}
	return c.JSON(http.StatusOK, common.SuccessPayload(result.Results))
}

func FetchTable(c yee.Context) (err error) {
	u := new(_FetchBind)
	if err = c.Bind(u); err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	unescape, _ := url.QueryUnescape(u.SourceId)
	if !checkSourcePerm(c, unescape) {
		return deny(c)
	}
	var s model.CoreDataSource
	model.DB().Where("source_id =?", unescape).First(&s)

	result, err := common.ScanDataRows(s, u.DataBase, "SHOW TABLES;", "Table", false, false)

	if err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	return c.JSON(http.StatusOK, common.SuccessPayload(result.Results))
}

func FetchTableInfo(c yee.Context) (err error) {
	u := new(_FetchBind)
	if err = c.Bind(u); err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}

	if !checkSourcePerm(c, u.SourceId) {
		return deny(c)
	}
	if u.DataBase != "" && u.Table != "" {
		if err := u.FetchTableFieldsOrIndexes(); err != nil {
			c.Logger().Critical(err.Error())
		}
		return c.JSON(http.StatusOK, common.SuccessPayload(map[string]interface{}{"rows": u.Rows, "idx": u.Idx}))
	}
	return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(errors.New(i18n.DefaultLang.Load(i18n.INFO_LIBRARY_NAME_TABLE_NAME))))
}

func FetchSQLTest(c yee.Context) (err error) {
	u := new(common.SQLTest)
	if err = c.Bind(u); err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	user := new(factory.Token).JwtParse(c).Username
	if !common.HasAnySourcePermission(user, u.SourceId) {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_USER_NO_PERMISSION)))
	}
	var s model.CoreDataSource
	model.DB().Where("source_id =?", u.SourceId).First(&s)
	rule, err := factory.CheckDataSourceRule(s.RuleId)
	if err != nil || rule == nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	p := enc.Decrypt(model.C.General.SecretKey, s.Password)
	if p == "" {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_KEY_DECRYPTION_FAILED)))
	}
	var rs []engine.Record
	client, err := calls.NewRpc()
	if err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	defer client.Close()
	if err := client.Call("Engine.Check", engine.CheckArgs{
		SQL:      u.SQL,
		Schema:   u.Database,
		IP:       s.IP,
		Username: s.Username,
		Port:     s.Port,
		Password: p,
		CA:       s.CAFile,
		Cert:     s.Cert,
		Key:      s.KeyFile,
		Kind:     u.Kind,
		Lang:     model.C.General.Lang,
		Rule:     *rule,
	}, &rs); err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	return c.JSON(http.StatusOK, common.SuccessPayload(rs))
}

func FetchOrderDetailList(c yee.Context) (err error) {
	expr := new(PageSizeRef)
	if err := c.Bind(expr); err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	if !checkOrderPerm(c, expr.WorkId) {
		return deny(c)
	}
	var record []model.CoreSqlRecord
	var count int64
	start, end := factory.Paging(expr.Page, expr.PageSize)
	model.DB().Model(&model.CoreSqlRecord{}).Where("work_id =?", expr.WorkId).Count(&count).Offset(start).Limit(end).Find(&record)
	return c.JSON(http.StatusOK, common.SuccessPayload(map[string]interface{}{"record": record, "count": count}))
}

func FetchOrderDetailRollSQL(c yee.Context) (err error) {
	workId := c.QueryParam("work_id")
	if !checkOrderPerm(c, workId) {
		return deny(c)
	}
	var roll []model.CoreRollback
	var count int64
	model.DB().Select("`sql`").Model(model.CoreRollback{}).Where("work_id =?", workId).Count(&count).Order("id desc").Find(&roll)
	return c.JSON(http.StatusOK, common.SuccessPayload(map[string]interface{}{"sql": roll, "count": count}))
}

func FetchUndo(c yee.Context) (err error) {
	u := c.QueryParam("work_id")
	user := new(factory.Token).JwtParse(c)
	var order model.CoreSqlOrder
	if err := model.DB().Where(UNDO_EXPR, user.Username, u, 2).First(&order).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.UNDO_MESSAGE_ERROR)))
	}
	pusher.NewMessagePusher(order.WorkId).Order().OrderBuild(pusher.UndoStatus).Push()
	model.DB().Where(UNDO_EXPR, user.Username, u, 2).Delete(&model.CoreSqlOrder{})
	return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.UNDO_MESSAGE_SUCCESS)))
}

func FetchMergeDDL(c yee.Context) error {
	req := new(referOrder)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	var optimizeSQL string
	if req.SQLs != "" {
		client, err := calls.NewRpc()
		if err != nil {
			return c.JSON(http.StatusOK, common.ERR_SOAR_ALTER_MERGE())
		}
		defer client.Close()
		if err := client.Call("Engine.MergeAlterTables", req.SQLs, &optimizeSQL); err != nil {
			return c.JSON(http.StatusOK, common.ERR_SOAR_ALTER_MERGE())
		}
		return c.JSON(http.StatusOK, common.SuccessPayload(optimizeSQL))
	}
	return c.JSON(http.StatusOK, common.ERR_SOAR_ALTER_MERGE())

}

func FetchSQLInfo(c yee.Context) (err error) {
	workId := c.QueryParam("work_id")
	if !checkOrderPerm(c, workId) {
		return deny(c)
	}
	var sql model.CoreSqlOrder
	model.DB().Select("`sql`").Where("work_id =?", workId).First(&sql)
	return c.JSON(http.StatusOK, common.SuccessPayload(map[string]interface{}{"sqls": sql.SQL}))
}

func FetchStepsProfile(c yee.Context) (err error) {
	workId := c.QueryParam("work_id")
	if !checkOrderPerm(c, workId) {
		return deny(c)
	}
	var s []model.CoreWorkflowDetail
	model.DB().Where("work_id = ?", workId).Find(&s)
	return c.JSON(http.StatusOK, common.SuccessPayload(s))
}

func FetchBoard(c yee.Context) (err error) {
	var board model.CoreGlobalConfiguration
	model.DB().Select("board").First(&board)
	return c.JSON(http.StatusOK, common.SuccessPayload(board))
}

func FetchOrderComment(c yee.Context) (err error) {
	workId := c.QueryParam("work_id")
	if !checkOrderPerm(c, workId) {
		return deny(c)
	}
	websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()
		var msg string
		for {
			if workId != "" {
				var comment []model.CoreOrderComment
				model.DB().Model(model.CoreOrderComment{}).Where("work_id =?", workId).Find(&comment)
				err := websocket.Message.Send(ws, factory.ToJson(comment))
				if err != nil {
					c.Logger().Error(err)
					break
				}
			}
			if err := websocket.Message.Receive(ws, &msg); err != nil {
				c.Logger().Debugf("receive error: %v", err)
				break
			}
		}

	}).ServeHTTP(c.Response(), c.Request())
	return nil
}

func PostOrderComment(c yee.Context) (err error) {
	u := new(model.CoreOrderComment)
	if err := c.Bind(u); err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	t := new(factory.Token).JwtParse(c)
	u.Time = time.Now().Format("2006-01-02 15:04")
	u.Username = t.Username
	model.DB().Model(model.CoreOrderComment{}).Create(u)
	return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.COMMENT_IS_POST)))
}

func FetchUserGroups(c yee.Context) (err error) {
	user := new(factory.Token).JwtParse(c)
	toUser := c.QueryParam("user")
	if user.Username != "admin" && user.Username != toUser {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(errors.New(i18n.DefaultLang.Load(i18n.ER_ILLEGAL_GET_INFO))))
	}
	var (
		p      model.CoreGrained
		g      []model.CoreRoleGroup
		groups []string
	)
	model.DB().Select("`group`").Where("username =?", toUser).First(&p)
	model.DB().Select("`group_id`,`name`").Find(&g)
	err = json.Unmarshal(p.Group, &groups)
	if err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	return c.JSON(http.StatusOK, common.SuccessPayload(map[string]interface{}{"own": p.Group, "groups": g}))
}

func FetchOrderState(c yee.Context) (err error) {
	workId := c.QueryParam("work_id")
	if !checkOrderPerm(c, workId) {
		return deny(c)
	}
	websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()
		var msg string
		for {
			if workId != "" {
				var order model.CoreSqlOrder
				model.DB().Model(model.CoreSqlOrder{}).Select("status").Where("work_id =?", workId).First(&order)
				err := websocket.Message.Send(ws, factory.ToJson(order.Status))
				if err != nil {
					c.Logger().Error(err)
					break
				}
			}
			if err := websocket.Message.Receive(ws, &msg); err != nil {
				break
			}
		}
	}).ServeHTTP(c.Response(), c.Request())
	return nil
}

func FetchUserInfo(c yee.Context) (err error) {
	t := new(factory.Token).JwtParse(c)
	var userInfo model.CoreAccount
	var sources []model.CoreDataSource
	var grained model.CoreGrained
	var groupIDs []string
	model.DB().Select("department,username,real_name,email").Model(model.CoreAccount{}).Where("username =?", t.Username).First(&userInfo)
	model.DB().Select("`group`").Where("username =?", t.Username).First(&grained)
	_ = grained.Group.UnmarshalToJSON(&groupIDs)
	model.DB().Model(model.CoreDataSource{}).Select("source_id,source").Where("source_id IN ?", permission.NewPermissionService(model.DB()).CreatePermissionListFromGroups(groupIDs).QuerySource).Find(&sources)
	p := userProfile{
		Department: userInfo.Department,
		RealName:   userInfo.RealName,
		Username:   userInfo.Username,
		Email:      userInfo.Email,
	}
	return c.JSON(http.StatusOK, common.SuccessPayload(map[string]interface{}{"user": p, "source": sources}))
}

func FetchSQLAdvisor(c yee.Context) (err error) {
	u := new(advisorFrom)
	if err := c.Bind(u); err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	tables, err := u.Go()
	if err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	adv, err := NewAIAgent()
	if err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))

	}
	resp, err := adv.BuildSQLAdvise(u, tables, "advisor")
	if err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	return c.JSON(http.StatusOK, common.SuccessPayload(resp))
}

func Text2SQL(c yee.Context) (err error) {
	u := new(advisorFrom)
	if err := c.Bind(u); err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	tables, err := u.Go()
	if err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	adv, err := NewAIAgent()
	if err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))

	}
	resp, err := adv.BuildSQLAdvise(u, tables, "text2sql")
	if err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	return c.JSON(http.StatusOK, common.SuccessPayload(resp))
}
