package service

import (
	"fmt"
	"time"

	"Yearning-go/src/model"

	"github.com/cookieY/yee/logger"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

// 历史数据归档（设计评审批次 4）：orders/records/rollbacks/comments/
// workflow_details/query_records 无限增长会让列表与统计渐慢。
// 策略：每日 cron 把「终态且创建早于 ArchiveDays 天」的工单连同全部明细、
// 以及早于 ArchiveDays 天的查询审计日志，迁移到同构 *_archive 表。
//
// 安全设计：
//   - 终态白名单 0 驳回 / 1 完成 / 4 失败 / 6 撤销——2 审核 / 3 执行 / 5 等待
//     绝不触碰（gh-ost 后台收尾也在 3）；
//   - 工单归档逐单独立事务（五表同进同出），中途崩溃不留撕裂；
//   - INSERT IGNORE INTO archive SELECT *：work_id 已归档过就跳过插入、
//     只删原表——重跑收敛（幂等），不会因历史残留死循环失败；
//     DELETE 删除的行必然已在归档表（本轮插入或此前已归档）；
//   - 每轮上限 archiveBatchLimit 单：首轮开启归档不会一口气搬几十万行锁库，
//     没搬完次日继续；
//   - date/time 为固定格式字符串（"2006-01-02 15:04"），字典序即时间序，
//     字符串比较可靠（datetime 迁移牵动全链路，单独评估）。

// archiveBatchLimit 每轮归档的工单数上限 / 查询日志每批行数。
const archiveBatchLimit = 200

// archiveQueryBatch 查询日志归档的每批行数（大 DELETE/INSERT 分段执行防长事务）。
const archiveQueryBatch = 5000

// archiveOrderTables 工单归档涉及的全部表对（原表 → 归档表）。
var archiveOrderTables = []struct{ from, to string }{
	{"core_sql_orders", "core_sql_orders_archive"},
	{"core_sql_records", "core_sql_records_archive"},
	{"core_rollbacks", "core_rollbacks_archive"},
	{"core_order_comments", "core_order_comments_archive"},
	{"core_workflow_details", "core_workflow_details_archive"},
}

// cronTabArchive 每日 03:30 执行归档（避开 TotalTickets 的 02:15）。
func cronTabArchive() {
	crontab := cron.New()
	if _, err := crontab.AddFunc("30 3 * * *", func() {
		_, _ = RunArchiveOnce()
	}); err != nil {
		logger.DefaultLogger.Error(err)
		return
	}
	crontab.Start()
}

// RunArchiveOnce 执行一轮归档，返回归档的工单数与查询日志行数。导出供测试。
func RunArchiveOnce() (orders int, queryRecords int64) {
	days := model.C.General.ArchiveDays
	if days <= 0 {
		return 0, 0
	}
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02 15:04")

	orders = archiveOrders(cutoff)
	queryRecords = archiveQueryRecords(cutoff)
	if orders > 0 || queryRecords > 0 {
		logger.DefaultLogger.Infof("归档完成：工单 %d 单（含明细），查询日志 %d 行（阈值 %d 天）", orders, queryRecords, days)
	}
	return orders, queryRecords
}

// archiveOrders 归档终态工单及其全部明细，返回本轮归档的单数。
func archiveOrders(cutoff string) int {
	// 终态白名单（裸值与 audit 包状态机同源：0/1/4/6，改动状态机时须同步）
	var ids []string
	model.DB().Model(&model.CoreSqlOrder{}).
		Where("`status` IN ? AND `date` <= ?", []int{0, 1, 4, 6}, cutoff).
		Order("id asc").Limit(archiveBatchLimit).
		Pluck("work_id", &ids)
	if len(ids) == 0 {
		return 0
	}

	moved := 0
	for _, wid := range ids {
		if err := archiveOneOrder(wid); err != nil {
			logger.DefaultLogger.Errorf("归档工单 %s 失败（跳过，下轮重试）: %v", wid, err)
			continue
		}
		moved++
	}
	return moved
}

// archiveOneOrder 单工单归档：五表 insert-ignore-select + delete 同一事务。
// 任一步失败整体回滚——要么全在原表、要么全部收进归档表。
func archiveOneOrder(workId string) error {
	return model.DB().Transaction(func(tx *gorm.DB) error {
		for _, t := range archiveOrderTables {
			// IGNORE：该 work_id 曾归档过（人工恢复等边缘场景）时跳过插入、
			// 只删原表，保证重跑收敛
			if err := tx.Exec("INSERT IGNORE INTO `"+t.to+"` SELECT * FROM `"+t.from+"` WHERE work_id = ?", workId).Error; err != nil {
				return fmt.Errorf("迁移 %s: %w", t.from, err)
			}
			if err := tx.Exec("DELETE FROM `"+t.from+"` WHERE work_id = ?", workId).Error; err != nil {
				return fmt.Errorf("删除 %s: %w", t.from, err)
			}
		}
		return nil
	})
}

// archiveQueryRecords 归档过期查询审计日志（与工单无关的独立日志表）。
// 分批「INSERT IGNORE → DELETE 同批」：DELETE 的行必然已在归档表
// （本轮插入的，或更早已归档而被 IGNORE 跳过的），循环到插不进为止。
func archiveQueryRecords(cutoff string) int64 {
	var total int64
	for i := 0; i < 1000; i++ { // 硬上限防意外死循环（1000 批 = 500 万行/天，足够）
		ins := model.DB().Exec(
			"INSERT IGNORE INTO `core_query_records_archive` SELECT * FROM `core_query_records` WHERE `time` <= ? LIMIT ?",
			cutoff, archiveQueryBatch)
		if ins.Error != nil {
			logger.DefaultLogger.Errorf("归档查询日志失败（第 %d 批后停止）: %v", i, ins.Error)
			break
		}
		if ins.RowsAffected == 0 {
			break // 该段已掏空（含全部被 IGNORE 的残留）
		}
		del := model.DB().Exec(
			"DELETE FROM `core_query_records` WHERE `time` <= ? LIMIT ?", cutoff, archiveQueryBatch)
		if del.Error != nil {
			logger.DefaultLogger.Errorf("归档查询日志后删除原表失败（已插入 %d 行，下轮 IGNORE 会跳过）: %v", ins.RowsAffected, del.Error)
			break
		}
		total += ins.RowsAffected
	}
	return total
}
