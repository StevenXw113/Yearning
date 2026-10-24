package record

import (
	"net/http"

	"Yearning-go/src/handler/common"
	"Yearning-go/src/model"

	"github.com/cookieY/yee"
)

// 归档数据查询（数据被 ArchiveDays 归档 cron 移入 *_archive 表后，
// 审计追溯从这里走）。挂在 /record 组（SuperRecorderGroup 权限）。

// ArchiveOrderList 归档工单分页列表：按日期范围（或工单号）查主归档表。
// query: page=N&size=M&date_from=YYYY-MM-DD&date_to=YYYY-MM-DD&work_id=xxx
func ArchiveOrderList(c yee.Context) (err error) {
	page, size := pageParams(c)
	q := model.DB().Model(&model.CoreSqlOrderArchive{})
	if wid := c.QueryParam("work_id"); wid != "" {
		q = q.Where("work_id =?", wid)
	}
	if v := c.QueryParam("date_from"); v != "" {
		q = q.Where("date >=?", v)
	}
	if v := c.QueryParam("date_to"); v != "" {
		q = q.Where("date <=?", v+" 23:59")
	}
	var total int64
	q.Count(&total)
	var orders []model.CoreSqlOrderArchive
	if err = q.Order("id desc").Offset((page - 1) * size).Limit(size).Find(&orders).Error; err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	return c.JSON(http.StatusOK, common.SuccessPayload(map[string]interface{}{
		"total":   total,
		"page":    page,
		"size":    size,
		"payload": orders,
	}))
}

// ArchiveOrderDetail 归档工单的完整明细：执行记录 / 回滚语句 / 评论 / 流水。
func ArchiveOrderDetail(c yee.Context) (err error) {
	wid := c.QueryParam("work_id")
	if wid == "" {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE("work_id 不能为空"))
	}
	var order model.CoreSqlOrderArchive
	if err = model.DB().Where("work_id =?", wid).First(&order).Error; err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE("归档中未找到该工单（可能未到归档阈值，请查活跃列表）"))
	}
	var records []model.CoreSqlRecordArchive
	model.DB().Where("work_id =?", wid).Order("id asc").Find(&records)
	var rollbacks []model.CoreRollbackArchive
	model.DB().Where("work_id =?", wid).Order("id asc").Find(&rollbacks)
	var comments []model.CoreOrderCommentArchive
	model.DB().Where("work_id =?", wid).Order("id asc").Find(&comments)
	var details []model.CoreWorkflowDetailArchive
	model.DB().Where("work_id =?", wid).Order("id asc").Find(&details)
	return c.JSON(http.StatusOK, common.SuccessPayload(map[string]interface{}{
		"order":     order,
		"records":   records,
		"rollbacks": rollbacks,
		"comments":  comments,
		"details":   details,
	}))
}

func pageParams(c yee.Context) (page, size int) {
	page = atoiDefault(c.QueryParam("page"), 1)
	size = atoiDefault(c.QueryParam("size"), 20)
	if size > 200 {
		size = 200
	}
	if page < 1 {
		page = 1
	}
	return page, size
}

func atoiDefault(s string, d int) int {
	if s == "" {
		return d
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return d
		}
		n = n*10 + int(r-'0')
	}
	if n == 0 {
		return d
	}
	return n
}
