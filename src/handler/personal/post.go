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
	"Yearning-go/src/lib/dbengine"
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
	"gorm.io/gorm"
	"net/http"
	"strings"
	"time"
)

func Post(c yee.Context) (err error) {
	switch c.Params("tp") {
	case "post":
		return sqlOrderPost(c)
	case "batch":
		// 项目级工单（批量提交）
		return BatchOrderPost(c)
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
	// 提交时先做硬保底校验（不读规则集、不依赖引擎在线），别等审批完才发现内容不合规；
	// 完整规则集审核在申请页「检测」与执行前复检两处由引擎完成。硬保底已收敛到 dbengine。
	if order.Type == vars.DML || order.Type == vars.DDL {
		var src model.CoreDataSource
		model.DB().Model(model.CoreDataSource{}).Where("source_id =?", order.SourceId).First(&src)
		if err := dbengine.For(src.DBType).ValidateChange(&src, order.SQL); err != nil {
			return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
		}
	}
	step, err := wrapperPostOrderInfo(order, c)
	if err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	order.ID = 0
	// 创建 + 编号回填 + 流水写入放同一事务：工单编号 = 自增 id，必须先落库拿到 id 再回填，
	// 两步分离的话中途失败会留下没有编号的孤儿工单（列表/详情都找不到它）
	err = model.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(order).Error; err != nil {
			return err
		}
		order.WorkId = factory.OrderNo(order.ID, 0)
		if err := tx.Model(order).Updates(map[string]interface{}{
			"work_id":   order.WorkId,
			"group_key": order.WorkId, // 普通工单分组键=自身工单号，见 CoreSqlOrder.GroupKey
		}).Error; err != nil {
			return err
		}
		return tx.Create(&model.CoreWorkflowDetail{
			WorkId:   order.WorkId,
			Username: user,
			Action:   i18n.DefaultLang.Load(i18n.INFO_SUBMITTED),
			Time:     time.Now().Format("2006-01-02 15:04"),
		}).Error
	})
	if err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	// 邮件推送 / autoTask 是外部副作用，不参与事务回滚，放在提交之后
	pusher.NewMessagePusher(order.WorkId).Order().OrderBuild(pusher.SummitStatus).Push()

	if order.Type == vars.DML {
		goAutoTask(order, step)
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
	// 工单编号由自增 id 生成，见 factory.OrderNo；这里先留空，落库后回填
	order.WorkId = ""
	order.Username = user.Username
	order.RealName = user.RealName
	order.Date = time.Now().Format("2006-01-02 15:04")
	order.Status = 2
	order.CurrentStep = 1
	// 「人工处理」执行方式已并入「人工执行」：平台外自行处理的场景 =
	// 人工执行 + 完成后「标记完成」（必填说明留痕），不再单独设模式。
	// 提交侧归一化旧值，存量 handled 工单的读路径（审批分支/cron 排除/展示）保留兼容。
	if order.Delay == "handled" {
		order.Delay = "manual"
	}
	order.Assigned = strings.Join(step[1].Auditor, ",")
	order.Relevant = factory.JsonStringify(decodeRelation(order.SourceId))
	order.File = sanitizeFileName(order.File)
	return len(step), nil
}

// sanitizeFileName 收敛客户端提交的文件名：只保留文件名本体（去掉路径），
// 清理控制字符并按 rune 截断到 file 列长度(varchar 200)。
// 单工单与项目工单两条创建链路都经过这里。
func sanitizeFileName(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	if r := []rune(name); len(r) > 200 {
		name = string(r[:200])
	}
	return name
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
