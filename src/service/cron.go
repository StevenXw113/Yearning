package service

import (
	"Yearning-go/src/handler/order/audit"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/model"
	"github.com/cookieY/yee/logger"
	"github.com/robfig/cron/v3"
	"time"
)

// cronTabDelayOrder 每分钟扫描到期的延迟执行工单(status=5)并触发执行。
// 原子置位(status 5→1)避免并发/重复触发；执行失败置回 4。
func cronTabDelayOrder() {
	crontab := cron.New()
	if _, err := crontab.AddFunc("* * * * *", func() {
		var orders []model.CoreSqlOrder
		now := time.Now().Format("2006-01-02 15:04")
		// delay 为 'YYYY-MM-DD HH:MM' 到点即执行
		model.DB().Model(model.CoreSqlOrder{}).
			Where("`status` =? AND `delay` != 'none' AND `delay` <= ?", 5, now).
			Find(&orders)
		for _, o := range orders {
			// 原子占位(status 5→1)，防止重复触发；仅当抢占成功才执行。
			res := model.DB().Model(model.CoreSqlOrder{}).
				Where("work_id =? AND `status` =?", o.WorkId, 5).
				Updates(map[string]interface{}{"status": 1, "execute_time": time.Now().Format("2006-01-02 15:04")})
			if res.RowsAffected != 1 {
				continue
			}
			if err := audit.ExecuteWorkOrder(&o, "DelayScheduler"); err != nil {
				logger.DefaultLogger.Errorf("延迟工单 %s 执行失败: %v", o.WorkId, err)
				// 执行失败置回失败态 4
				model.DB().Model(model.CoreSqlOrder{}).Where("work_id =?", o.WorkId).
					Updates(map[string]interface{}{"status": 4})
			}
		}
	}); err != nil {
		logger.DefaultLogger.Error(err)
	}
	crontab.Start()
}

func cronTabMaskQuery() {
	crontab := cron.New()
	if _, err := crontab.AddFunc("* * * * *", func() {
		var queryOrder []model.CoreQueryOrder
		model.DB().Model(model.CoreQueryOrder{}).Where("`status` =?", 2).Find(&queryOrder)
		for _, i := range queryOrder {
			if factory.TimeDifference(i.ApprovalTime) {
				model.DB().Model(model.CoreQueryOrder{}).Where("work_id =?", i.WorkId).Updates(&model.CoreQueryOrder{Status: 3})
			}
		}
	}); err != nil {
		logger.DefaultLogger.Error(err)
	}
	crontab.Start()
}

func cronTabTotalTickets() {
	crontab := cron.New()
	if _, err := crontab.AddFunc("15 2 * * *", func() {
		var totalOrder int64
		var totalQuery int64
		model.DB().Model(model.CoreSqlOrder{}).Where("DATE(date) = CURDATE() - INTERVAL 1 DAY").Count(&totalOrder)
		model.DB().Model(model.CoreQueryOrder{}).Where("DATE(date) = CURDATE() - INTERVAL 1 DAY").Count(&totalQuery)
		model.DB().Model(model.CoreTotalTickets{}).Create(&model.CoreTotalTickets{TotalOrder: totalOrder, TotalQuery: totalQuery, Date: time.Now().Format("2006-01-02")})
	}); err != nil {
		logger.DefaultLogger.Error(err)
	}
	crontab.Start()
}
