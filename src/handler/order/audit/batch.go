package audit

import (
	"Yearning-go/src/handler/common"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/model"
	"github.com/cookieY/yee"
	"net/http"
	"time"
)

type batchExecuteResult struct {
	WorkId   string `json:"work_id"`
	Source   string `json:"source"`
	DataBase string `json:"data_base"`
	Ok       bool   `json:"ok"`
	Msg      string `json:"msg"`
}

// BatchExecute 项目级工单批量执行：按提交顺序串行执行批次内所有
// 已审批通过、等待执行(status=5)的子工单；任一失败即停止（后续 SQL 可能依赖前面的变更）。
func BatchExecute(c yee.Context) (err error) {
	u := new(Confirm)
	user := new(factory.Token).JwtParse(c)
	if err = c.Bind(u); err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	if u.BatchId == "" {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE)))
	}
	var orders []model.CoreSqlOrder
	model.DB().Where("batch_id =?", u.BatchId).Order("id").Find(&orders)
	if len(orders) == 0 {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ORDER_NOT_SEARCH)))
	}
	// 权限先整批校验：避免执行到一半才发现某条无权操作
	for i := range orders {
		if !common.IsOrderRelated(orders[i].WorkId, user.Username) {
			return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_ORDER_NOT_RELATED)))
		}
	}
	results := make([]batchExecuteResult, 0, len(orders))
	for i := range orders {
		o := &orders[i]
		r := batchExecuteResult{WorkId: o.WorkId, Source: o.Source, DataBase: o.DataBase, Ok: true}
		if o.Status != 5 {
			r.Msg = "已跳过（未到执行阶段或已终结）"
			results = append(results, r)
			continue
		}
		if err := ExecuteWorkOrder(o, user.Username); err != nil {
			r.Ok = false
			r.Msg = err.Error()
			results = append(results, r)
			// 失败即停：版本迭代的 SQL 往往存在前后依赖
			break
		}
		model.DB().Model(model.CoreSqlOrder{}).Where("work_id =?", o.WorkId).
			Updates(map[string]interface{}{"status": 1, "execute_time": time.Now().Format("2006-01-02 15:04")})
		results = append(results, r)
	}
	return c.JSON(http.StatusOK, common.SuccessPayload(results))
}
