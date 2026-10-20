package audit

import (
	"context"
	"errors"
	"time"

	"Yearning-go/src/engine"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/calls"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/lib/rediscmd"
	"Yearning-go/src/model"

	enginev1 "engine/gen/engine/v1"
)

// redisExecTimeout 单条 Redis 变更工单的执行超时。
// 比 Mongo 短得多：Redis 命令是毫秒级的，需要长时间执行的那些
// （KEYS 全量遍历之类）本来就被审核规则挡在外面。
const redisExecTimeout = 2 * time.Minute

// redisRecheckTimeout 执行前规则复检的超时（与申请页检测一致）
const redisRecheckTimeout = 30 * time.Second

// executeRedisOrder 执行一条 Redis 变更工单。
//
// 与 SQL 侧的区别：不经 gRPC 引擎执行（引擎按 kind 只做静态审核），
// 由主程序用 go-redis 执行；执行前抓前镜像，回滚语句落 core_rollbacks——
// 与 SQL / Mongo 共用同一张表，详情页的「回滚语句」面板无需区分数据源类型。
func executeRedisOrder(order *model.CoreSqlOrder, source *model.CoreDataSource, actor string) error {
	// 执行前先过硬保底（与提交时同一套）：提交到审批之间工单内容可能被改过
	if _, _, err := rediscmd.Validate(order.SQL); err != nil {
		recordExec(order.WorkId, order.SQL, "执行失败", 0, err.Error())
		return err
	}
	// 再用**当前**规则集复检一次：审批期间规则集可能被改严，
	// 这个数据源也可能被换成另一套规则集。引擎不可用时直接失败、不静默放行。
	rule, err := factory.CheckDataSourceRule(source.RuleId)
	if err != nil {
		recordExec(order.WorkId, order.SQL, "执行失败", 0, "规则集加载失败："+err.Error())
		return errors.New("规则集加载失败：" + err.Error())
	}
	if err := recheckRedisRules(order, source, rule); err != nil {
		recordExec(order.WorkId, order.SQL, "执行失败", 0, err.Error())
		return err
	}

	// 工单的 data_base 形如 db0，解析成库号后建连（go-redis 在建连时 SELECT）
	client, err := source.ConnectRedisDB(model.RedisParseDB(order.DataBase))
	if err != nil {
		recordExec(order.WorkId, order.SQL, "执行失败", 0, err.Error())
		return err
	}
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), redisExecTimeout)
	defer cancel()

	out, err := rediscmd.Execute(ctx, client, order.SQL)
	if err != nil {
		recordExec(order.WorkId, order.SQL, "执行失败", 0, err.Error())
		return err
	}
	recordExec(order.WorkId, order.SQL, "已执行", out.Affected, "")
	for _, s := range out.Rollback {
		model.DB().Create(&model.CoreRollback{WorkId: order.WorkId, SQL: s})
	}
	model.DB().Create(&model.CoreWorkflowDetail{
		WorkId:   order.WorkId,
		Username: actor,
		Time:     time.Now().Format("2006-01-02 15:04"),
		Action:   i18n.DefaultLang.Load(i18n.ORDER_EXECUTE_STATE),
	})
	return nil
}

// recheckRedisRules 执行前用**当前**规则集再审核一次。
//
// 与 SQL 侧的 client.Exec、Mongo 侧的 recheckMongoRules 同理：
// 提交时只做硬保底（不读规则集），申请页检测读的是**当时**的规则集，
// 执行前这一步才是最终裁决。引擎不可用时直接失败、不静默放行。
func recheckRedisRules(order *model.CoreSqlOrder, source *model.CoreDataSource, rule *engine.AuditRole) error {
	client, conn, err := calls.NewClient()
	if err != nil {
		return errors.New("规则复检失败（引擎不可用）：" + err.Error())
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), redisRecheckTimeout)
	defer cancel()
	// DataSource 只给 kind：Redis 审核是纯静态判定，不需要连目标库，
	// 也就没有理由把数据源口令在这里再传一遍。
	rep, err := client.Check(ctx, &enginev1.CheckRequest{
		Sql:    order.SQL,
		Schema: order.DataBase,
		Source: &enginev1.DataSource{Kind: calls.DataSourceKind(source.DBType)},
		Lang:   model.C.General.Lang,
		Rule:   engine.AuditRoleToProto(rule),
		Mode:   engine.CheckModeWrite,
	})
	if rep == nil || !rep.Ok {
		if e := calls.CombineReplyErr(rep, err); e != nil {
			return errors.New("规则复检失败：" + e.Error())
		}
		return errors.New("规则复检失败：引擎返回异常")
	}
	for _, r := range rep.Records {
		// Level 1 = error 级命中（拦截）；warn(2) / observe(3) 只提示不拦，
		// 与申请页检测、SQL 侧的判定口径一致。
		if r.GetLevel() == 1 {
			return errors.New("规则集拦截：" + r.GetError())
		}
	}
	return nil
}
