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
	"Yearning-go/src/handler/manage/flow"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/lib/permission"
	"Yearning-go/src/lib/pusher"
	"Yearning-go/src/lib/vars"
	"Yearning-go/src/model"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cookieY/yee"
	"github.com/cookieY/yee/logger"
	"net/http"
	"strings"
	"time"
)

func Post(c yee.Context) (err error) {
	switch c.Params("tp") {
	case "post":
		return sqlOrderPost(c)
	case "edit":
		return editPersonalUser(c)
	}
	return err
}

func sqlOrderPost(c yee.Context) (err error) {
	order := new(model.CoreSqlOrder)
	user := new(factory.Token).JwtParse(c).Username
	if err = c.Bind(order); err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}

	if !permission.NewPermissionService(model.DB()).Equal(&permission.Control{User: user, Kind: order.Type, SourceId: order.SourceId, WorkId: order.WorkId}) {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(fmt.Errorf(i18n.DefaultLang.Load(i18n.ER_USER_NO_PERMISSION), user, order.SourceId)))
	}
	step, err := wrapperPostOrderInfo(order, c)
	if err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	order.ID = 0
	model.DB().Create(order)
	model.DB().Create(&model.CoreWorkflowDetail{
		WorkId:   order.WorkId,
		Username: user,
		Action:   i18n.DefaultLang.Load(i18n.INFO_SUBMITTED),
		Time:     time.Now().Format("2006-01-02 15:04"),
	})
	pusher.NewMessagePusher(order.WorkId).Order().OrderBuild(pusher.SummitStatus).Push()

	if order.Type == vars.DML {
		autoTask(order, step)
	}

	return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.ORDER_POST_SUCCESS)))
}

func wrapperPostOrderInfo(order *model.CoreSqlOrder, y yee.Context) (length int, err error) {
	var from model.CoreWorkflowTpl
	var flowId model.CoreDataSource
	var step []flow.Tpl
	model.DB().Model(model.CoreDataSource{}).Where("source_id = ?", order.SourceId).First(&flowId)
	model.DB().Model(model.CoreWorkflowTpl{}).Where("id =?", flowId.FlowID).Find(&from)
	err = json.Unmarshal(from.Steps, &step)
	if err != nil {
		y.Logger().Error(err)
		return 0, err
	}
	// 流程至少要有提交级与一个审批级，否则工单会没有审批人
	if len(step) < 2 {
		y.Logger().Error("approval flow is incomplete")
		return 0, errors.New(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE))
	}
	user := new(factory.Token).JwtParse(y)
	if order.Source == "" {
		order.Source = flowId.Source
	}
	if order.IDC == "" {
		order.IDC = flowId.IDC
	}
	order.WorkId = factory.GenWorkId()
	order.Username = user.Username
	order.RealName = user.RealName
	order.Date = time.Now().Format("2006-01-02 15:04")
	order.Status = 2
	order.CurrentStep = 1
	order.Assigned = strings.Join(step[1].Auditor, ",")
	order.Relevant = factory.JsonStringify(decodeRelation(order.SourceId))
	return len(step), nil
}

func decodeRelation(sourceId string) []string {
	var relevant []string
	r, err := flow.OrderRelation(sourceId)
	if err != nil {
		logger.DefaultLogger.Error(err)
		return []string{}
	}
	for _, i := range r {
		relevant = append(relevant, i.Auditor...)
	}
	return relevant
}
