package personal

import (
	"Yearning-go/src/handler/common"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/lib/permission"
	"Yearning-go/src/lib/pusher"
	"Yearning-go/src/lib/vars"
	"Yearning-go/src/model"
	"fmt"
	"github.com/cookieY/yee"
	"net/http"
	"time"
)

type batchItem struct {
	SourceId string `json:"source_id"`
	DataBase string `json:"data_base"`
	File     string `json:"file"`
	SQL      string `json:"sql"`
}

type batchOrderReq struct {
	Type   int         `json:"type"`
	Backup uint        `json:"backup"`
	Delay  string      `json:"delay"`
	Text   string      `json:"text"`
	Items  []batchItem `json:"items"`
}

// BatchOrderPost 项目级工单（批量提交）：一次提交多条明细（每个数据源/库一条），
// 每条明细生成一个独立子工单，共享同一 batch_id；各子工单沿用其数据源已有的审批流。
func BatchOrderPost(c yee.Context) (err error) {
	req := new(batchOrderReq)
	user := new(factory.Token).JwtParse(c).Username
	if err = c.Bind(req); err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	if len(req.Items) == 0 {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(fmt.Errorf("工单明细不能为空")))
	}
	// 先整批预检（权限 + 审批流），全部通过才落库，避免提交一半
	orders := make([]*model.CoreSqlOrder, 0, len(req.Items))
	steps := make([]int, 0, len(req.Items))
	for i, item := range req.Items {
		if item.SourceId == "" || item.DataBase == "" || item.SQL == "" {
			return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(fmt.Errorf("第 %d 条明细缺少数据源 / 目标库 / SQL", i+1)))
		}
		if !permission.NewPermissionService(model.DB()).Equal(&permission.Control{User: user, Kind: req.Type, SourceId: item.SourceId}) {
			return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(fmt.Errorf(i18n.DefaultLang.Load(i18n.ER_USER_NO_PERMISSION), user, item.SourceId)))
		}
		order := &model.CoreSqlOrder{
			Type:     req.Type,
			Backup:   req.Backup,
			Delay:    req.Delay,
			Text:     req.Text,
			SourceId: item.SourceId,
			DataBase: item.DataBase,
			File:     item.File,
			SQL:      item.SQL,
		}
		step, werr := wrapperPostOrderInfo(order, c)
		if werr != nil {
			return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(fmt.Errorf("第 %d 条明细: %v", i+1, werr)))
		}
		orders = append(orders, order)
		steps = append(steps, step)
	}
	// 先整批落库拿到自增 id（此时编号留空）：第一条子工单的 id 即项目工单号，
	// 各子工单编号为「项目号-序号」，如 123-1、123-2
	for _, order := range orders {
		model.DB().Create(order)
	}
	projectNo := factory.OrderNo(orders[0].ID, 0)
	for i, order := range orders {
		order.BatchId = projectNo
		order.WorkId = factory.OrderNo(orders[0].ID, i+1)
		model.DB().Model(order).Updates(map[string]interface{}{"work_id": order.WorkId, "batch_id": projectNo})
		model.DB().Create(&model.CoreWorkflowDetail{
			WorkId:   order.WorkId,
			Username: user,
			Action:   i18n.DefaultLang.Load(i18n.INFO_SUBMITTED),
			Time:     time.Now().Format("2006-01-02 15:04"),
		})
		pusher.NewMessagePusher(order.WorkId).Order().OrderBuild(pusher.SummitStatus).Push()
		if order.Type == vars.DML {
			autoTask(order, steps[i])
		}
	}
	return c.JSON(http.StatusOK, common.SuccessPayload(projectNo))
}

// BatchOrders 查询一个项目级工单（批次）下的所有子工单状态
func BatchOrders(c yee.Context) (err error) {
	batchId := c.QueryParam("batch_id")
	if batchId == "" {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE)))
	}
	var orders []model.CoreSqlOrder
	model.DB().
		Select("work_id, username, real_name, text, assigned, current_step, `status`, `type`, `source`, `source_id`, `data_base`, `date`, `execute_time`, `delay`, batch_id").
		Where("batch_id =?", batchId).Order("id").Find(&orders)
	return c.JSON(http.StatusOK, common.SuccessPayload(orders))
}
