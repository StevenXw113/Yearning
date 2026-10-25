package personal

import (
	"Yearning-go/src/handler/common"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/dbengine"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/lib/permission"
	"Yearning-go/src/lib/pusher"
	"Yearning-go/src/lib/vars"
	"Yearning-go/src/model"
	"fmt"
	"github.com/cookieY/yee"
	"gorm.io/gorm"
	"net/http"
	"time"
)

type batchItem struct {
	SourceId string `json:"source_id"`
	DataBase string `json:"data_base"`
	File     string `json:"file"`
	SQL      string `json:"sql"`
	// Table 目标表/集合：SQL 侧按需填写；MongoDB 由前端从命令里提取集合名，
	// 落进子工单的 table 字段，便于详情页与审计日志辨识（此前所有引擎都留空）。
	Table string `json:"table"`
}

type batchOrderReq struct {
	Type int `json:"type"`
	// TxMode 工单级事务模式（1=开启）：整批统一，作用于每个子工单
	TxMode uint        `json:"tx_mode"`
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
	if len(req.Items) > maxBatchItems {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(fmt.Errorf("项目级工单最多 %d 条明细，请拆分为多个项目", maxBatchItems)))
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
		// 命令式数据源（MongoDB / Redis）整批预检时就做硬保底校验，定位到第几条明细
		if req.Type == vars.DML || req.Type == vars.DDL {
			var src model.CoreDataSource
			model.DB().Model(model.CoreDataSource{}).Where("source_id =?", item.SourceId).First(&src)
			if err := dbengine.For(src.DBType).ValidateChange(&src, item.SQL); err != nil {
				return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(fmt.Errorf("第 %d 条明细: %v", i+1, err)))
			}
		}
		order := &model.CoreSqlOrder{
			Type:     req.Type,
			Backup:   req.Backup,
			TxMode:   req.TxMode,
			Delay:    req.Delay,
			Text:     req.Text,
			SourceId: item.SourceId,
			DataBase: item.DataBase,
			Table:    item.Table,
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
	// 整批落库放同一事务：先批量 Create 拿到自增 id（此时编号留空），第一条子工单的 id 即
	// 项目工单号，各子工单编号为「项目号-序号」（如 123-1、123-2），编号/批次号回填与
	// 流水写入也进事务——循环中途失败不会留下一半有编号一半没有的批次
	var projectNo string
	err = model.DB().Transaction(func(tx *gorm.DB) error {
		for _, order := range orders {
			if err := tx.Create(order).Error; err != nil {
				return err
			}
		}
		projectNo = factory.OrderNo(orders[0].ID, 0)
		for i, order := range orders {
			order.BatchId = projectNo
			order.WorkId = factory.OrderNo(orders[0].ID, i+1)
			if err := tx.Model(order).Updates(map[string]interface{}{
				"work_id":   order.WorkId,
				"batch_id":  projectNo,
				"group_key": projectNo,
			}).Error; err != nil {
				return err
			}
			if err := tx.Create(&model.CoreWorkflowDetail{
				WorkId:   order.WorkId,
				Username: user,
				Action:   i18n.DefaultLang.Load(i18n.INFO_SUBMITTED),
				Time:     time.Now().Format("2006-01-02 15:04"),
			}).Error; err != nil {
				return err
			}
			if err := insertOrderRelevants(tx, order.WorkId, order.Relevant); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	// 事务外：推送 / autoTask 是外部副作用，不参与回滚；顺序保持提交顺序
	for i, order := range orders {
		pusher.NewMessagePusher(order.WorkId).Order().OrderBuild(pusher.SummitStatus).Push()
		if order.Type == vars.DML {
			goAutoTask(order, steps[i])
		}
	}
	return c.JSON(http.StatusOK, common.SuccessPayload(projectNo))
}

// maxBatchItems 项目级工单的明细数量上限：一个事务里整批落库（每条还有编号回填与流水
// 写入），不设上限的话一次请求就能制造任意大的事务；项目发布的规模也用不到更多
const maxBatchItems = 50

// BatchOrders 查询一个项目级工单（批次）下的所有子工单状态。
// 批次号是连续编号，不设权限校验等于任何登录用户都能枚举读到全公司的变更计划
// （提交人/数据源/目标库/状态）。跨数据源批次只有部分子工单与查询人相关，
// 这里按「任一子工单的相关人（提交人/审批链）或超管」放行——与审核列表对
// 批次的可见口径一致。
func BatchOrders(c yee.Context) (err error) {
	batchId := c.QueryParam("batch_id")
	if batchId == "" {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE)))
	}
	user := new(factory.Token).JwtParse(c).Username
	var orders []model.CoreSqlOrder
	model.DB().
		Select("work_id, username, real_name, text, assigned, current_step, `status`, `type`, `source`, `source_id`, `data_base`, `date`, `execute_time`, `delay`, batch_id, tx_mode").
		Where("batch_id =?", batchId).Order("id").Find(&orders)
	if len(orders) == 0 {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ORDER_NOT_SEARCH)))
	}
	if !permission.IsSuperUser(user) {
		related := false
		for i := range orders {
			if common.IsOrderRelated(orders[i].WorkId, user) {
				related = true
				break
			}
		}
		if !related {
			return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_ORDER_NOT_RELATED)))
		}
	}
	return c.JSON(http.StatusOK, common.SuccessPayload(orders))
}
