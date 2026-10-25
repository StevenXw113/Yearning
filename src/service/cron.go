package service

import (
	"Yearning-go/src/handler/order/audit"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/model"
	"fmt"
	"github.com/cookieY/yee/logger"
	"github.com/robfig/cron/v3"
	"sync"
	"sync/atomic"
	"time"
)

// cronOnce 分钟级 cron 的防重入保护：robfig/cron 每次触发起独立 goroutine
// （startJob），上一轮没跑完下一轮照发——延迟工单执行一轮可能超过 1 分钟
// （大工单），无保护时并发度随重入轮数无界增长。
type cronOnce struct{ inFlight atomic.Bool }

func (o *cronOnce) begin() bool { return o.inFlight.CompareAndSwap(false, true) }
func (o *cronOnce) end()        { o.inFlight.Store(false) }

var (
	delayCronOnce    cronOnce
	selfHealCronOnce cronOnce
	maskQueryOnce    cronOnce
)

// cronTabDelayOrder 每分钟扫描到期的延迟执行工单(status=5)并触发执行。
// 原子抢占(status 5→3「执行中」)避免与人工执行/重复触发并发；执行结束由
// FinalizeExecution 置 1（成功）或 4（失败）。
//
// 工单间并发执行：抢占闸门保证同一工单绝不被重复执行，因此逐条 goroutine
// 并发是安全的——此前串行循环里一条慢工单会阻塞当天后续全部延迟工单。
// 并发度取 min(4, len(orders))，避免审批通过瞬间大批工单同时压向引擎与目标库。
func cronTabDelayOrder() {
	crontab := cron.New()
	if _, err := crontab.AddFunc("* * * * *", func() {
		if !delayCronOnce.begin() {
			return // 上一轮仍在执行，本轮跳过
		}
		defer delayCronOnce.end()
		var orders []model.CoreSqlOrder
		now := time.Now().Format("2006-01-02 15:04")
		// delay 为 'YYYY-MM-DD HH:MM' 到点即执行；delay='manual' 是人工执行、
		// 'handled' 是人工处理模式（DBA 自行处理），都必须排除
		model.DB().Model(model.CoreSqlOrder{}).
			Where("`status` =? AND `delay` != 'none' AND `delay` != 'manual' AND `delay` != 'handled' AND `delay` <= ?", model.OrderStatusWaiting, now).
			Find(&orders)
		var wg sync.WaitGroup
		sem := make(chan struct{}, 4)
		for _, o := range orders {
			// 原子抢占(status 5→3)，防止与人工执行/重复触发并发；仅当抢占成功才执行。
			// 抢占必须在 goroutine 内做：串行抢占+并发执行会让「抢到但还没跑」的
			// 工单在慢工单占满信号量时被饿死在待执行队列里。
			res := model.DB().Model(model.CoreSqlOrder{}).
				Where("work_id =? AND `status` =?", o.WorkId, model.OrderStatusWaiting).
				Updates(map[string]interface{}{"status": model.OrderStatusExecuting, "execute_time": time.Now().Format("2006-01-02 15:04")})
			if res.RowsAffected != 1 {
				continue
			}
			wg.Add(1)
			sem <- struct{}{}
			go func(order model.CoreSqlOrder) {
				defer wg.Done()
				defer func() { <-sem }()
				// ExecuteWorkOrder 可能返回 oscAsync=true（gh-ost 后台执行，收尾在其 goroutine），
				// 此时这里不能再动状态
				oscAsync, execErr := audit.ExecuteWorkOrder(&order, "DelayScheduler")
				if execErr != nil {
					logger.DefaultLogger.Errorf("延迟工单 %s 执行失败: %v", order.WorkId, execErr)
				}
				if !oscAsync {
					audit.FinalizeExecution(order.WorkId, execErr)
				}
			}(o)
		}
		wg.Wait()
	}); err != nil {
		logger.DefaultLogger.Error(err)
	}
	crontab.Start()
}

// cronTabSelfHeal 执行中(status=3)工单的僵尸自愈：进程崩溃会留下永远「执行中」
// 的工单（既不能重试也不能标记），超过 SelfHealHours 小时即置为失败并留痕，
// 让 DBA 可见、可用重试/跳过通道接管。
//
// 安全前提：
//   - 单实例互斥（instancelock）保证本进程是唯一主实例——没有它，本 cron 会
//     把其他实例正在执行的工单误杀；
//   - gh-ost 仍在本进程运行的工单豁免（IsOSCRunning）：大表在线变更跑数小时
//     是常态，它们有自己的收尾 goroutine；
//   - 状态迁移带 status=3 守卫：与收尾竞态时（刚好跑完）最多漏一次自愈，不会误改。
//
// SelfHealHours=0 关闭（默认 6 小时）。
func cronTabSelfHeal() {
	crontab := cron.New()
	if _, err := crontab.AddFunc("* * * * *", func() {
		if !selfHealCronOnce.begin() {
			return
		}
		defer selfHealCronOnce.end()
		hours := model.C.General.SelfHealHours
		if hours <= 0 {
			return
		}
		cutoff := time.Now().Add(-time.Duration(hours) * time.Hour).Format("2006-01-02 15:04")
		var orders []model.CoreSqlOrder
		// 只取所需列:整行会拖出 SQL/OSCInfo 等 longtext 大列
		model.DB().Model(model.CoreSqlOrder{}).Select("work_id", "status", "execute_time").
			Where("`status` =? AND `execute_time` != '' AND `execute_time` <= ?", model.OrderStatusExecuting, cutoff).
			Find(&orders)
		for _, o := range orders {
			if audit.IsOSCRunning(o.WorkId) {
				continue // 长时 OSC 在本进程正常执行中，豁免
			}
			res := model.DB().Model(&model.CoreSqlOrder{}).
				Where("work_id =? AND `status` =?", o.WorkId, model.OrderStatusExecuting).
				Updates(map[string]interface{}{"status": model.OrderStatusFailed})
			if res.Error != nil || res.RowsAffected != 1 {
				continue // 刚被收尾（竞态）或已被处理
			}
			detail := fmt.Sprintf(i18n.DefaultLang.Load(i18n.ORDER_SELF_HEAL_MESSAGE), hours)
			model.DB().Create(&model.CoreWorkflowDetail{
				WorkId:   o.WorkId,
				Username: "SelfHealScheduler",
				Time:     time.Now().Format("2006-01-02 15:04"),
				Action:   detail,
			})
			logger.DefaultLogger.Errorf("工单 %s 执行超过 %d 小时仍为「执行中」，已自愈置为失败（进程可能崩溃过）", o.WorkId, hours)
		}
	}); err != nil {
		logger.DefaultLogger.Error(err)
	}
	crontab.Start()
}

func cronTabMaskQuery() {
	crontab := cron.New()
	if _, err := crontab.AddFunc("* * * * *", func() {
		if !maskQueryOnce.begin() {
			return
		}
		defer maskQueryOnce.end()
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
