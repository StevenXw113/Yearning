package audit

import (
	"Yearning-go/src/engine"
	"Yearning-go/src/handler/common"
	"Yearning-go/src/handler/manage/flow"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/calls"
	"Yearning-go/src/lib/enc"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/lib/metrics"
	"Yearning-go/src/lib/permission"
	"Yearning-go/src/lib/pusher"
	"Yearning-go/src/lib/vars"
	"Yearning-go/src/model"
	"context"
	"encoding/json"
	enginev1 "engine/gen/engine/v1"
	"errors"
	"fmt"
	"github.com/cookieY/yee/logger"
	"gorm.io/gorm"
	"strings"
	"time"
)

type Confirm struct {
	WorkId   string `json:"work_id"`
	BatchId  string `json:"batch_id"`
	Page     int    `json:"page"`
	Flag     int    `json:"flag"`
	Text     string `json:"text"`
	Tp       string `json:"tp"`
	SourceId string `json:"source_id"`
	Delay    string `json:"delay"`
}

func (e *Confirm) GetTPL() []flow.Tpl {
	var order model.CoreSqlOrder
	var s model.CoreDataSource
	var tpl []flow.Tpl
	var flow model.CoreWorkflowTpl
	// 必须从工单自身查出 source_id：请求体里的 source_id 由客户端提供，
	// 用它来决定审批链会让攻击者为任意工单挑选审批人
	model.DB().Model(model.CoreSqlOrder{}).Select("source_id").Where("work_id =?", e.WorkId).First(&order)
	model.DB().Model(model.CoreDataSource{}).Select("flow_id").Where("source_id =?", order.SourceId).First(&s)
	model.DB().Model(model.CoreWorkflowTpl{}).Where("id =?", s.FlowID).First(&flow)
	_ = json.Unmarshal(flow.Steps, &tpl)
	return tpl
}

// claimExecution 原子抢占执行权：把工单置为「执行中」(status=3) 并记录开始时间。
// from 为允许抢占的起始状态（末级审批刚通过的 2、等待执行的 5）；
// 并发触发/状态已变化时 UPDATE 命中 0 行，返回 false，调用方必须放弃执行——
// 这是防止同一工单被重复执行（双击审批、人工执行与定时 cron 撞车）的唯一闸门。
func claimExecution(workId string, from ...int) bool {
	if workId == "" || len(from) == 0 {
		return false
	}
	res := model.DB().Model(&model.CoreSqlOrder{}).
		Where("work_id =? AND `status` IN ?", workId, from).
		Updates(map[string]interface{}{
			"status":       model.OrderStatusExecuting,
			"execute_time": time.Now().Format("2006-01-02 15:04"),
		})
	return res.Error == nil && res.RowsAffected == 1
}

// FinalizeExecution 执行收尾：已抢占（status=3）的工单按结果置 1（完成）或 4（失败）。
// 带 status=3 守卫，避免覆盖执行期间被其他操作改写的状态。
// 导出供 cron（src/service）延迟调度共用。
func FinalizeExecution(workId string, execErr error) {
	status := model.OrderStatusFinished
	if execErr != nil {
		status = model.OrderStatusFailed
	}
	model.DB().Model(&model.CoreSqlOrder{}).
		Where("work_id =? AND `status` =?", workId, model.OrderStatusExecuting).
		Updates(map[string]interface{}{"status": status})
}

// executeClaimedOrder 抢占并执行。所有执行触发点（人工执行、末级审批立即执行、
// 定时到期、批次执行）必须经过这里，绕开即失去防重复执行保护。
// 返回 oscAsync=true 表示 gh-ost 已在后台启动（此时 err 必为 nil），调用方不得再动
// 工单状态（收尾由 OSC 的后台 goroutine 完成），应向用户返回「已启动」类文案。
func executeClaimedOrder(order *model.CoreSqlOrder, actor string, from ...int) (oscAsync bool, err error) {
	if !claimExecution(order.WorkId, from...) {
		return false, errors.New(i18n.DefaultLang.Load(i18n.ER_ORDER_EXECUTING_CONFLICT))
	}
	oscAsync, err = ExecuteWorkOrder(order, actor)
	if !oscAsync {
		FinalizeExecution(order.WorkId, err)
	}
	return oscAsync, err
}

// isAuditor 精确匹配审批人（trim 后整名相等）。
// 原实现用 strings.Contains(所有审批人拼接串, user) 做子串匹配，
// 用户名是他人审批人前缀/子串时会越权通过校验。
func isAuditor(auditors []string, user string) bool {
	for _, a := range auditors {
		if strings.TrimSpace(a) == user {
			return true
		}
	}
	return false
}

func ExecuteOrder(u *Confirm, user string) common.Resp {
	var order model.CoreSqlOrder
	model.DB().Where("work_id =?", u.WorkId).First(&order)

	if order.WorkId == "" {
		return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ORDER_NOT_SEARCH))
	}
	// 对外执行入口只受理「已审批通过、等待执行」(status=5)的工单。
	// 不允许 status=2（审核中）从这里执行——那等于让申请人绕过审批直接执行；
	// 末级审批通过后的立即执行在 MultiAuditOrder 内部以 from=2 走 executeClaimedOrder。
	if order.Status != model.OrderStatusWaiting {
		return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_ORDER_NOT_EXECUTABLE))
	}
	oscAsync, err := executeClaimedOrder(&order, user, model.OrderStatusWaiting)
	if err != nil {
		logger.DefaultLogger.Error(err)
		return common.ERR_COMMON_MESSAGE(err)
	}
	if oscAsync {
		return common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.ORDER_OSC_STARTED))
	}
	return common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.ORDER_EXECUTE_STATE))
}

// withTxMode 工单级事务模式覆写：tx=1 时强制 DML 走事务（引擎按 DmlTransaction
// 开事务），优先级高于规则集的同名开关，覆盖执行/重试/定时/批次全链路。
// 必须返回拷贝：ruleId=0 时 CheckDataSourceRule 返回全局规则单例（model.GloRole.Load()），
// 原地修改会把覆写泄漏给之后的所有请求。浅拷贝即可，RuleLevel map 只读共享。
func withTxMode(rule *engine.AuditRole, tx uint) *engine.AuditRole {
	if rule == nil || tx != 1 {
		return rule
	}
	copied := *rule
	copied.DMLTransaction = true
	return &copied
}

// execContext 构造引擎执行用的上下文：超时取 [General].ExecTimeoutMinutes
// （分钟），未配置默认 5 分钟，0 = 不限制。大批量 DML / 未走 OSC 的大 DDL
// 可能需要远超 5 分钟——被超时切断的 DDL 回滚代价极高，按环境放宽。
func execContext() (context.Context, context.CancelFunc) {
	timeout := time.Duration(model.C.General.ExecTimeoutMinutes) * time.Minute
	if model.C.General.ExecTimeoutMinutes == 0 {
		return context.WithCancel(context.Background())
	}
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return context.WithTimeout(context.Background(), timeout)
}

// ExecuteWorkOrder 执行单个工单并落明细/回滚/流水（引擎 Exec 或 gh-ost 二选一）。
// 调用方（人工执行、末级审批立即执行、定时 cron、批次执行、autoTask）必须先完成
// 状态校验与原子抢占（executeClaimedOrder / cron 的 5→3），本函数自身不动工单状态。
// 返回 oscAsync=true 表示本次由 gh-ost 在后台执行（收尾在其 goroutine），调用方
// 不得再调 FinalizeExecution；actor 为操作者标识（审批人用户名、DelayScheduler 或
// AutoTask Robot）。
func ExecuteWorkOrder(order *model.CoreSqlOrder, actor string) (oscAsync bool, err error) {
	var source model.CoreDataSource
	if order == nil || order.WorkId == "" {
		return false, errors.New(i18n.DefaultLang.Load(i18n.ORDER_NOT_SEARCH))
	}
	model.DB().Model(model.CoreDataSource{}).Where("source_id =?", order.SourceId).First(&source)
	// 所有数据源统一走引擎 Exec：MongoDB / Redis 由引擎的 execCommand 分支执行
	// （硬保底 → 连接 → 前镜像 → 命令执行），SQL 由引擎的拆分执行路径处理；
	// 落库（core_sql_records / core_rollbacks / 流程明细）由本函数按 Records/Rollback 统一写。
	rule, err := factory.CheckDataSourceRule(source.RuleId)
	if err != nil || rule == nil {
		return false, err
	}
	// 工单勾选了事务模式 → 覆写规则的 DMLTransaction（见 withTxMode 的拷贝说明）
	rule = withTxMode(rule, order.TxMode)
	p := enc.Decrypt(model.C.General.SecretKey, source.Password)
	if p == "" {
		return false, errors.New(i18n.DefaultLang.Load(i18n.ER_KEY_DECRYPTION_FAILED))
	}

	// 大表 ALTER 由 gh-ost 接管做在线变更（进度写入 osc_info 供详情页展示）：
	// gh-ost 在后台 goroutine 里跑并自行收尾，本调用立即返回；
	// 不满足接管条件时返回 false，按原链路交给引擎执行
	if handled, _ := RunOSC(order, &source, p, actor, rule); handled {
		return true, nil
	}

	client, err := calls.Client()
	if err != nil {
		return false, err
	}
	start := time.Now()
	ctx, cancel := execContext()
	defer cancel()
	rep, err := client.Exec(ctx, &enginev1.ExecRequest{
		Order: calls.OrderToProto(order),
		Rules: engine.AuditRoleToProto(rule),
		Source: &enginev1.DataSource{
			Ip:       source.IP,
			Port:     int32(source.Port),
			Username: source.Username,
			Password: p,
			Ca:       source.CAFile,
			Cert:     source.Cert,
			Key:      source.KeyFile,
			Kind:     calls.DataSourceKind(source.DBType),
			AuthDb:   source.AuthDatabase,
		},
	})
	if rep == nil {
		return false, calls.CombineReplyErr(rep, err)
	}
	// 先落执行明细与回滚语句：即使工单中途失败（部分语句已生效），也必须留下可回滚的依据。
	// 逐条 Insert 收敛为一个事务的批量写入——半途失败会留下残缺明细，且逐条往返在
	// 大工单下明显更慢。落库失败不推翻已发生的执行结果，记日志供排查。
	records := make([]model.CoreSqlRecord, 0, len(rep.Records))
	rollbacks := make([]model.CoreRollback, 0, len(rep.Rollback))
	for _, r := range rep.Records {
		rec := engine.RecordFromProto(r)
		records = append(records, model.CoreSqlRecord{
			WorkId:    order.WorkId,
			SQL:       rec.SQL,
			State:     rec.Status,
			Affectrow: rec.AffectRows,
			Time:      time.Now().Format("2006-01-02 15:04"),
			Error:     rec.Error,
		})
		// 执行成功的语句若带回了回滚语句，落到 core_rollbacks 供详情页「回滚语句」面板展示/重提
		if rec.Status == "已执行" && rec.RollBack != "" {
			rollbacks = append(rollbacks, model.CoreRollback{WorkId: order.WorkId, SQL: rec.RollBack})
		}
	}
	// binlog 抓取到的整单回滚语句（覆盖多表/无主键/全表等全部场景），按语句逐条落库
	for _, s := range rep.Rollback {
		if s != "" {
			rollbacks = append(rollbacks, model.CoreRollback{WorkId: order.WorkId, SQL: s})
		}
	}
	if len(records) > 0 || len(rollbacks) > 0 {
		if txErr := model.DB().Transaction(func(tx *gorm.DB) error {
			if len(records) > 0 {
				if err := tx.Create(&records).Error; err != nil {
					return err
				}
			}
			if len(rollbacks) > 0 {
				return tx.Create(&rollbacks).Error
			}
			return nil
		}); txErr != nil {
			logger.DefaultLogger.Errorf("工单 %s 执行明细/回滚语句落库失败: %v", order.WorkId, txErr)
		}
	}
	// 执行指标：成败与耗时（OSC 后台路径不计——耗时在其 goroutine 内不可测）
	tp := "ddl"
	if order.Type == vars.DML {
		tp = "dml"
	}
	if rep.Ok {
		metrics.OrdersTotal.With(tp, "success").Inc()
	} else {
		metrics.OrdersTotal.With(tp, "failed").Inc()
	}
	metrics.OrderExecDuration.Observe(time.Since(start))
	if !rep.Ok {
		return false, calls.CombineReplyErr(rep, err)
	}
	model.DB().Create(&model.CoreWorkflowDetail{
		WorkId:   order.WorkId,
		Username: actor,
		Time:     time.Now().Format("2006-01-02 15:04"),
		Action:   i18n.DefaultLang.Load(i18n.ORDER_EXECUTE_STATE),
	})
	return false, nil
}

// remainingSQL 计算失败工单重试时应执行的剩余 SQL。
// 前提：core_sql_records 里 state=「已执行」的记录按 id 升序，必然对应工单 SQL 被
// 引擎拆分后的一个前缀（引擎逐条执行、首错即停；各执行器写入的 Record.Sql 都是
// 原文的连续子串——SQL 引擎是去首尾空白/分号的切片，Mongo/Redis 是整段命令）。
// 因此逐条在原文中**顺序**定位已执行语句，定位终点之后的原文即剩余部分；
// 任意一条定位不到说明记录与工单内容对不上（如人工改库过），宁可拒绝重试也
// 不能重复执行已生效的语句。
func remainingSQL(sql string, executed []model.CoreSqlRecord) (string, error) {
	pos := 0
	for _, rec := range executed {
		idx := strings.Index(sql[pos:], rec.SQL)
		if idx < 0 || rec.SQL == "" {
			return "", errors.New(i18n.DefaultLang.Load(i18n.ER_ORDER_RETRY_MISMATCH))
		}
		pos += idx + len(rec.SQL)
	}
	// 引擎记录的语句文本不带结尾分号，剩余部分要先跳过语句分隔符再判空，
	// 否则「update a;update b;」匹配完 a 后剩余是「;update b;」
	remain := strings.TrimSpace(sql[pos:])
	remain = strings.TrimSpace(strings.TrimLeft(remain, ";"))
	if remain == "" {
		return "", errors.New(i18n.DefaultLang.Load(i18n.ER_ORDER_RETRY_ALL_EXECUTED))
	}
	return remain, nil
}

// remainingSQLAfter 计算「跳过失败语句继续执行」时应执行的剩余 SQL。
// 与 remainingSQL 的区别：失败语句视为已由人工处理（在库上修好或确认放弃），
// 定位终点推进到最后一条记录（含失败记录）之后。
// 同一语句可能有多条记录（先失败后重试成功），顺序定位时从当前位置向后找不到、
// 但在全文中存在的记录视为「已消费语句的重复记录」跳过；全文中都找不到才是
// 记录与工单内容不匹配，必须拒绝。
func remainingSQLAfter(sql string, records []model.CoreSqlRecord) (string, error) {
	pos := 0
	for _, rec := range records {
		if rec.SQL == "" {
			return "", errors.New(i18n.DefaultLang.Load(i18n.ER_ORDER_RETRY_MISMATCH))
		}
		idx := strings.Index(sql[pos:], rec.SQL)
		if idx < 0 {
			if strings.Contains(sql, rec.SQL) {
				continue // 重复记录：该语句此前已定位过（如先失败、重试后成功的两次记录）
			}
			return "", errors.New(i18n.DefaultLang.Load(i18n.ER_ORDER_RETRY_MISMATCH))
		}
		pos += idx + len(rec.SQL)
	}
	remain := strings.TrimSpace(sql[pos:])
	remain = strings.TrimSpace(strings.TrimLeft(remain, ";"))
	if remain == "" {
		return "", errors.New(i18n.DefaultLang.Load(i18n.ER_ORDER_SKIP_EMPTY))
	}
	return remain, nil
}

// SkipAndContinue 数据订正场景的人工介入路径：执行中断后，人工已在数据库侧处理完
// 失败语句（或确认放弃），从这里**跳过它继续执行后续语句**——与「重试」（把失败
// 语句再跑一遍）互补。共用抢占(4→3)/收尾闸门与重试预览的定位逻辑。
func SkipAndContinue(u *Confirm, user string) common.Resp {
	var order model.CoreSqlOrder
	model.DB().Where("work_id =?", u.WorkId).First(&order)
	if order.WorkId == "" {
		return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ORDER_NOT_SEARCH))
	}
	if order.Status != model.OrderStatusFailed {
		return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_ORDER_NOT_RETRYABLE))
	}
	// 全量记录（已执行 + 失败，含历次尝试）：失败语句视为已处理，定位到它之后
	var records []model.CoreSqlRecord
	model.DB().Model(model.CoreSqlRecord{}).Select("`sql`").
		Where("work_id =?", u.WorkId).
		Order("id asc").Find(&records)
	remain, err := remainingSQLAfter(order.SQL, records)
	if err != nil {
		return common.ERR_COMMON_MESSAGE(err)
	}
	continueOrder := order
	continueOrder.SQL = remain
	oscAsync, err := executeClaimedOrder(&continueOrder, user, model.OrderStatusFailed)
	if err != nil {
		logger.DefaultLogger.Error(err)
		return common.ERR_COMMON_MESSAGE(err)
	}
	model.DB().Create(&model.CoreWorkflowDetail{
		WorkId:   order.WorkId,
		Username: user,
		Time:     time.Now().Format("2006-01-02 15:04"),
		Action:   i18n.DefaultLang.Load(i18n.ORDER_SKIP_MESSAGE),
	})
	if oscAsync {
		return common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.ORDER_OSC_STARTED))
	}
	return common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.ORDER_EXECUTE_STATE))
}

// RetryOrder 失败工单重试：不新建工单，直接对 status=4 的工单重执**未成功执行过的
// 语句**（见 remainingSQL），已生效的语句不会重复落库。抢占走 4→3，与其它执行
// 触发点共用防重复闸门；OSC 接管的工单重试即重新走 gh-ost（其自带影子表清理）。
func RetryOrder(u *Confirm, user string) common.Resp {
	var order model.CoreSqlOrder
	model.DB().Where("work_id =?", u.WorkId).First(&order)
	if order.WorkId == "" {
		return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ORDER_NOT_SEARCH))
	}
	if order.Status != model.OrderStatusFailed {
		return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_ORDER_NOT_RETRYABLE))
	}
	var executed []model.CoreSqlRecord
	model.DB().Model(model.CoreSqlRecord{}).Select("`sql`").
		Where("work_id =? AND state =?", u.WorkId, "已执行").
		Order("id asc").Find(&executed)
	remain, err := remainingSQL(order.SQL, executed)
	if err != nil {
		return common.ERR_COMMON_MESSAGE(err)
	}
	// 只替换 SQL 内容重走统一执行链路；工单本身仍是原工单（编号/审批流/权限不变）
	retry := order
	retry.SQL = remain
	oscAsync, err := executeClaimedOrder(&retry, user, model.OrderStatusFailed)
	if err != nil {
		logger.DefaultLogger.Error(err)
		return common.ERR_COMMON_MESSAGE(err)
	}
	model.DB().Create(&model.CoreWorkflowDetail{
		WorkId:   order.WorkId,
		Username: user,
		Time:     time.Now().Format("2006-01-02 15:04"),
		Action:   i18n.DefaultLang.Load(i18n.ORDER_RETRY_MESSAGE),
	})
	if oscAsync {
		return common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.ORDER_OSC_STARTED))
	}
	return common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.ORDER_EXECUTE_STATE))
}

// MarkOrderFinished 手动标记完成：SQL 由 DBA 在平台外自行处理（如引擎暂不支持的特殊
// 数据库、需人工介入的修复），在平台里把工单直接置为完成并留痕。这是给 DBA 的灵活性
// 出口，因此权限收紧为**当前审批人或超管**（不含提交人——提交人单方面关单会让审批
// 流失去可见性）；必须填写处理说明，写入流程流水与评论便于审计。
// 允许的起始状态：2 审核中 / 4 失败 / 5 待执行；已终结状态（0/1/6）与执行中（3）不可标记。
func MarkOrderFinished(u *Confirm, user string) common.Resp {
	if strings.TrimSpace(u.Text) == "" {
		return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_ORDER_MARK_NO_TEXT))
	}
	var od model.CoreSqlOrder
	model.DB().Model(model.CoreSqlOrder{}).Select("work_id, status, assigned").
		Where("work_id =?", u.WorkId).First(&od)
	if od.WorkId == "" {
		return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ORDER_NOT_SEARCH))
	}
	if !permission.IsSuperUser(user) && !isAuditor(strings.Split(od.Assigned, ","), user) {
		return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_ORDER_MARK_PERMISSION))
	}
	switch od.Status {
	case model.OrderStatusAuditing, model.OrderStatusFailed, model.OrderStatusWaiting:
	default:
		return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_ORDER_NOT_MARKABLE))
	}
	// 状态迁移带原状态守卫，与撤销/执行等并发操作互斥
	res := model.DB().Model(model.CoreSqlOrder{}).
		Where("work_id =? AND `status` IN ?", u.WorkId, []int{model.OrderStatusAuditing, model.OrderStatusFailed, model.OrderStatusWaiting}).
		Updates(map[string]interface{}{"status": model.OrderStatusFinished, "execute_time": time.Now().Format("2006-01-02 15:04")})
	if res.Error != nil || res.RowsAffected != 1 {
		return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_ORDER_NOT_MARKABLE))
	}
	action := fmt.Sprintf(i18n.DefaultLang.Load(i18n.ORDER_MARK_MESSAGE), u.Text)
	model.DB().Create(&model.CoreWorkflowDetail{
		WorkId:   u.WorkId,
		Username: user,
		Time:     time.Now().Format("2006-01-02 15:04"),
		Action:   action,
	})
	model.DB().Create(&model.CoreOrderComment{
		WorkId:   u.WorkId,
		Username: user,
		Content:  action,
		Time:     time.Now().Format("2006-01-02 15:04"),
	})
	pusher.NewMessagePusher(u.WorkId).Order().OrderBuild(pusher.ExecuteStatus).Push()
	return common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.ORDER_MARK_STATE))
}

// moveToWaiting 末级审批通过后转入「等待执行」(status=5)：带 status=2 守卫，
// 与撤销/驳回/重复同意等并发操作互斥；命中 0 行说明状态已被改变，调用方报幂等错误。
func moveToWaiting(workId string) bool {
	res := model.DB().Model(model.CoreSqlOrder{}).
		Where("work_id =? AND `status` =?", workId, model.OrderStatusAuditing).
		Updates(map[string]interface{}{"status": model.OrderStatusWaiting})
	return res.Error == nil && res.RowsAffected == 1
}

func MultiAuditOrder(req *Confirm, user string) common.Resp {
	if assigned, isExecute, ok := isNotIdempotent(req, user); ok {
		if isExecute {
			// 末级审批通过：带延迟执行时间(delay!='none')的工单进入"等待延迟执行"(status=5)，
			// 由延迟调度 cron 到点触发；普通工单立即执行。
			var od model.CoreSqlOrder
			model.DB().Model(model.CoreSqlOrder{}).Where("work_id =?", req.WorkId).First(&od)
			if od.WorkId == "" {
				return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ORDER_NOT_SEARCH))
			}
			// 项目级工单（批量）不默认自动执行：末级审批通过后统一停在「等待执行」(status=5)，
			// 由人工逐条执行或在批次页手动批量执行，避免审批通过瞬间多条子工单同时压向数据库
			if od.BatchId != "" {
				return waitingOrConflict(req.WorkId, i18n.ORDER_AGREE_WAIT_MANUAL)
			}
			// 执行方式由工单填写时的 delay 决定：manual=人工执行、具体时间=定时执行、
			// none/空=审批通过即执行（全局「人工执行」开关打开时也会停在这里等人工）。
			switch {
			case od.Delay == "manual":
				return waitingOrConflict(req.WorkId, i18n.ORDER_AGREE_WAIT_MANUAL)
			case od.Delay == "handled":
				// 人工处理模式：审批通过后不由系统执行，DBA 自行处理 SQL，
				// 完成后用「标记完成」收口（必须填处理说明留痕）
				return waitingOrConflict(req.WorkId, i18n.ORDER_AGREE_WAIT_HANDLED)
			case od.Delay != "" && od.Delay != "none":
				// 定时执行：等延迟调度 cron 到点触发
				return waitingOrConflict(req.WorkId, i18n.ORDER_AGREE_STATE)
			case model.GloOther.Load().ManualExecute:
				return waitingOrConflict(req.WorkId, i18n.ORDER_AGREE_WAIT_MANUAL)
			}
			// 立即执行：此刻工单仍是审核中(status=2)，从这里原子抢占(2→3)后执行。
			// 抢占失败说明状态已被并发操作改变（如撤销/重复同意），直接报冲突。
			oscAsync, err := executeClaimedOrder(&od, user, model.OrderStatusAuditing)
			if err != nil {
				return common.ERR_COMMON_MESSAGE(err)
			}
			if oscAsync {
				return common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.ORDER_OSC_STARTED))
			}
			return common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.ORDER_EXECUTE_STATE))
		}
		// 中间级同意：WHERE 带 status=2（仅审核中的工单可推进）且 current_step=flag
		// （审批层级与请求一致），重放已通过的层级、推进已终结的工单都会命中 0 行
		res := model.DB().Model(model.CoreSqlOrder{}).
			Where("work_id =? AND `status` =? AND current_step =?", req.WorkId, model.OrderStatusAuditing, req.Flag).
			Updates(&model.CoreSqlOrder{CurrentStep: req.Flag + 1, Assigned: strings.Join(assigned, ",")})
		if res.Error != nil || res.RowsAffected != 1 {
			return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ORDER_NOT_SEARCH))
		}
		model.DB().Create(&model.CoreWorkflowDetail{
			WorkId:   req.WorkId,
			Username: user,
			Time:     time.Now().Format("2006-01-02 15:04"),
			Action:   fmt.Sprintf(i18n.DefaultLang.Load(i18n.ORDER_AGREE_MESSAGE), strings.Join(assigned, " ")),
		})
		pusher.NewMessagePusher(req.WorkId).Order().OrderBuild(pusher.NextStepStatus).Push()
		return common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.ORDER_AGREE_STATE))
	}
	return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ORDER_NOT_SEARCH))
}

// waitingOrConflict 末级审批后转入等待执行；状态被并发改变时报幂等错误
func waitingOrConflict(workId string, successKey int) common.Resp {
	if !moveToWaiting(workId) {
		return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ORDER_NOT_SEARCH))
	}
	return common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(successKey))
}

// RejectOrder 驳回：仅当前层级审批人可驳回「审核中」(status=2)的工单。
// 审批层级取工单自身的 current_step——历史实现取请求体的 flag，而前端驳回请求不带 flag
// （恒为 0），校验落在流程第 0 步「提交人」占位文案上，驳回实际从未生效过。
// 状态迁移带 status=2 守卫并检查命中行数，已终结工单不再产生幻影流水/评论/邮件。
func RejectOrder(req *Confirm, user string) common.Resp {
	var od model.CoreSqlOrder
	model.DB().Model(model.CoreSqlOrder{}).Select("work_id, status, current_step").
		Where("work_id =?", req.WorkId).First(&od)
	if od.WorkId == "" || od.Status != model.OrderStatusAuditing {
		return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ORDER_NOT_SEARCH))
	}
	tpl := req.GetTPL()
	if od.CurrentStep < 1 || od.CurrentStep >= len(tpl) || !isAuditor(tpl[od.CurrentStep].Auditor, user) {
		return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ORDER_NOT_SEARCH))
	}
	res := model.DB().Model(&model.CoreSqlOrder{}).
		Where("work_id =? AND `status` =?", req.WorkId, model.OrderStatusAuditing).
		Updates(map[string]interface{}{"status": model.OrderStatusRejected})
	if res.Error != nil || res.RowsAffected != 1 {
		return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ORDER_NOT_SEARCH))
	}
	model.DB().Create(&model.CoreWorkflowDetail{
		WorkId:   req.WorkId,
		Username: user,
		Time:     time.Now().Format("2006-01-02 15:04"),
		Action:   i18n.DefaultLang.Load(i18n.ORDER_REJECT_MESSAGE),
	})
	model.DB().Create(&model.CoreOrderComment{
		WorkId:   req.WorkId,
		Username: user,
		Content:  fmt.Sprintf("驳回理由: %s", req.Text),
		Time:     time.Now().Format("2006-01-02 15:04"),
	})
	pusher.NewMessagePusher(req.WorkId).Order().OrderBuild(pusher.RejectStatus).Push()
	return common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.ORDER_REJECT_STATE))
}

// delayKill 终止延迟工单：审核中(2)或等待执行(5)的延迟工单直接置为失败(4)。
// 正在执行(3)的工单不经这里置状态——OSC 由 KillOSC 发 SIGTERM 后经执行链路
// 的 FinalizeExecution 落到 4，普通引擎执行没有中断手段，保持原状态由收尾决定。
// 返回是否确实终止了工单（0 行命中说明状态不符，调用方不应再写流水/返回成功）。
func delayKill(workId string) (string, bool) {
	res := model.DB().Model(&model.CoreSqlOrder{}).
		Where("work_id =? AND `status` IN ?", workId, []int{model.OrderStatusAuditing, model.OrderStatusWaiting}).
		Updates(map[string]interface{}{"status": model.OrderStatusFailed, "execute_time": time.Now().Format("2006-01-02 15:04")})
	if res.Error != nil || res.RowsAffected != 1 {
		return "", false
	}
	return i18n.DefaultLang.Load(i18n.ORDER_DELAY_KILL_DETAIL), true
}

// hasOrderPermission 判断用户是否有权操作该工单
func hasOrderPermission(workId, user string) bool {
	return common.IsOrderRelated(workId, user)
}

// isNotIdempotent 校验当前用户是请求层级(req.Flag)的审批人，并返回下一层审批人与
// 是否末级。注意：这里只做「人是否对」的校验，「工单状态/层级是否还能被推进」由
// MultiAuditOrder 的 UPDATE 守卫（status=2 AND current_step=flag）兜底。
func isNotIdempotent(r *Confirm, user string) ([]string, bool, bool) {
	if r.Flag < 0 {
		return nil, false, false
	}
	tpl := r.GetTPL()
	if len(tpl) > r.Flag {
		if !isAuditor(tpl[r.Flag].Auditor, user) {
			return nil, false, false
		}
		if r.Flag+1 == len(tpl) {
			return tpl[r.Flag].Auditor, true, true
		}
		return tpl[r.Flag+1].Auditor, false, true
	}
	return nil, false, false
}
