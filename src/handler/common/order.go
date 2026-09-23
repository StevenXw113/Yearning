package common

import (
	"Yearning-go/src/lib/permission"
	"Yearning-go/src/lib/vars"
	"Yearning-go/src/model"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// IsOrderRelated 判断用户是否与工单相关：提交人、当前审批人之一，或审批链上的历史审批人之一。
// 工单不存在时返回 false，避免通过响应差异枚举工单号；超级管理员与任何工单都相关。
func IsOrderRelated(workId, user string) bool {
	var o model.CoreSqlOrder
	if err := model.DB().Model(model.CoreSqlOrder{}).
		Select("username,assigned,relevant").
		Where("work_id =?", workId).First(&o).Error; err != nil {
		return false
	}
	// 超级管理员拥有所有权限，可操作任意工单
	if permission.IsSuperUser(user) {
		return true
	}
	if o.Username == user {
		return true
	}
	for _, auditor := range strings.Split(o.Assigned, ",") {
		if strings.TrimSpace(auditor) == user {
			return true
		}
	}
	var relevant []string
	if o.Relevant != nil {
		_ = o.Relevant.UnmarshalToJSON(&relevant)
	}
	for _, r := range relevant {
		if r == user {
			return true
		}
	}
	return false
}

// HasSourcePermission 判断用户对数据源是否具备指定类型（vars.DDL/DML/QUERY）的权限
func HasSourcePermission(user, sourceId string, kind int) bool {
	return permission.NewPermissionService(model.DB()).Equal(&permission.Control{User: user, Kind: kind, SourceId: sourceId})
}

// HasAnySourcePermission 判断用户对数据源是否具备任意一种权限，用于查看库表结构等低敏操作
func HasAnySourcePermission(user, sourceId string) bool {
	for _, kind := range []int{vars.DDL, vars.DML, vars.QUERY} {
		if HasSourcePermission(user, sourceId, kind) {
			return true
		}
	}
	return false
}

// GroupedOrderPage 按「项目(批次)」分页的工单列表，供我的工单 / 工单审核 / 记录三处列表共用：
// 普通工单一单一组，项目级工单整批一组；同一批次的子工单必然落在同一页，
// 否则前端按 batch_id 聚合时会在两页里各显示一组项目。
// 本页各批次的全部子工单会一并查出（因此返回行数可能多于每页条数），前端聚合即得完整项目。
// 筛选条件只决定「哪些项目进本页」，项目内的子工单一律给全。
// ponytail: 分组键是表达式，COUNT/GROUP BY 用不上索引（全表扫 + 临时表）；
// 工单量上万后再物化一个 group_key 列（batch_id 或 id）并加索引。
func GroupedOrderPage(p *PageList[[]model.CoreSqlOrder], fields string, scopes ...func(*gorm.DB) *gorm.DB) {
	pageSize := p.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	offset := (p.Current - 1) * pageSize
	if offset < 0 {
		offset = 0
	}
	// 分组键：batch_id 为空按自身 id 各成一组（普通工单），否则整批一组
	const groupKey = "IF(batch_id = '', id, batch_id)"
	_ = model.DB().Model(model.CoreSqlOrder{}).
		Select("COUNT(DISTINCT " + groupKey + ")").Scopes(scopes...).Row().Scan(&p.Page)

	// 别名排序：待审批的项目排前面，与列表默认排序保持一致
	var groups []struct {
		Mid     int64
		Bid     string // MIN(batch_id)：同组 batch_id 相同，用聚合以兼容 ONLY_FULL_GROUP_BY
		Pending int
	}
	model.DB().Model(model.CoreSqlOrder{}).
		Select("MIN(id) AS mid, MIN(batch_id) AS bid, MAX(`status` = 2) AS pending").
		Scopes(scopes...).Group(groupKey).Order("pending DESC, mid DESC").
		Offset(offset).Limit(pageSize).Scan(&groups)

	p.Data = nil
	if len(groups) == 0 {
		return
	}
	mids := make([]int64, 0, len(groups))
	bids := make([]string, 0, len(groups))
	for _, g := range groups {
		mids = append(mids, g.Mid)
		if g.Bid != "" {
			bids = append(bids, g.Bid)
		}
	}
	q := model.DB().Model(model.CoreSqlOrder{}).Select(fields).Where("id IN ?", mids)
	if len(bids) > 0 {
		q = q.Or("batch_id IN ?", bids)
	}
	// 列表固定排序：待审批优先，再按提交时间倒序
	q.Order("(status = 2) DESC, date DESC").Find(&p.Data)
	p.Data = groupBatchRows(p.Data)
}

// groupBatchRows 把同一批次(batch_id)的子工单收拢到该批次首次出现的位置，并按 id 升序排列
// （创建顺序即「项目号-1、-2、-3…」）。列表的排序是「待审优先 + 时间」，会把同一项目的子工单
// 打散（-2 排到 -1 前面），而项目级工单在界面上是一个整体，必须严格按序号排列。
// 普通工单保持原有顺序与位置。
func groupBatchRows(rows []model.CoreSqlOrder) []model.CoreSqlOrder {
	grouped := map[string][]model.CoreSqlOrder{}
	for _, r := range rows {
		if r.BatchId != "" {
			grouped[r.BatchId] = append(grouped[r.BatchId], r)
		}
	}
	seen := map[string]bool{}
	out := make([]model.CoreSqlOrder, 0, len(rows))
	for _, r := range rows {
		if r.BatchId == "" {
			out = append(out, r)
			continue
		}
		if seen[r.BatchId] {
			continue
		}
		seen[r.BatchId] = true
		g := grouped[r.BatchId]
		sort.Slice(g, func(i, j int) bool { return g[i].ID < g[j].ID })
		out = append(out, g...)
	}
	return out
}
