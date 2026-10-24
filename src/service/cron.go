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
// 原子抢占(status 5→3「执行中」)避免与人工执行/重复触发并发；执行结束由
// FinalizeExecution 置 1（成功）或 4（失败）。
func cronTabDelayOrder() {
	crontab := cron.New()
	if _, err := crontab.AddFunc("* * * * *", func() {
		var orders []model.CoreSqlOrder
		now := time.Now().Format("2006-01-02 15:04")
		// delay 为 'YYYY-MM-DD HH:MM' 到点即执行；delay='manual' 是人工执行、
		// 'handled' 是人工处理模式（DBA 自行处理），都必须排除
		model.DB().Model(model.CoreSqlOrder{}).
			Where("`status` =? AND `delay` != 'none' AND `delay` != 'manual' AND `delay` != 'handled' AND `delay` <= ?", 5, now).
			Find(&orders)
		for _, o := range orders {
			// 原子抢占(status 5→3)，防止与人工执行/重复触发并发；仅当抢占成功才执行。
			res := model.DB().Model(model.CoreSqlOrder{}).
				Where("work_id =? AND `status` =?", o.WorkId, 5).
				Updates(map[string]interface{}{"status": 3, "execute_time": time.Now().Format("2006-01-02 15:04")})
			if res.RowsAffected != 1 {
				continue
			}
			// ExecuteWorkOrder 可能返回 oscAsync=true（gh-ost 后台执行，收尾在其 goroutine），
			// 此时这里不能再动状态
			oscAsync, execErr := audit.ExecuteWorkOrder(&o, "DelayScheduler")
			if execErr != nil {
				logger.DefaultLogger.Errorf("延迟工单 %s 执行失败: %v", o.WorkId, execErr)
			}
			if !oscAsync {
				audit.FinalizeExecution(o.WorkId, execErr)
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
