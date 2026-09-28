package audit

import (
	"context"
	"errors"
	"strings"
	"time"

	"Yearning-go/src/engine"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/calls"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/lib/mongodb"
	"Yearning-go/src/model"

	enginev1 "engine/gen/engine/v1"

	"go.mongodb.org/mongo-driver/bson"
)

// mongoExecTimeout 单条变更工单的执行超时（聚合/大批量更新可能较慢）
const mongoExecTimeout = 10 * time.Minute

// mongoRecheckTimeout 执行前规则复检的超时（与申请页检测一致）
const mongoRecheckTimeout = 30 * time.Second

// executeMongoOrder 执行一条 MongoDB 变更工单。
// 与 SQL 侧的区别：不经 gRPC 引擎（引擎只会用 MySQL DSN 连目标库），
// 由主程序直接用 mongo 驱动执行；执行前抓前镜像，回滚语句落 core_rollbacks——
// 与 SQL 共用同一张表，所以详情页的「回滚语句」面板不需要区分数据源类型。
func executeMongoOrder(order *model.CoreSqlOrder, source *model.CoreDataSource, actor string) error {
	var cmd bson.D
	if err := bson.UnmarshalExtJSON([]byte(order.SQL), false, &cmd); err != nil {
		recordMongo(order.WorkId, order.SQL, "执行失败", 0, "工单内容不是合法的 MongoDB 命令 JSON："+err.Error())
		return errors.New("工单内容不是合法的 MongoDB 命令 JSON：" + err.Error())
	}
	// 执行前再校验一次：提交到审批这段时间里命令可能被改过，不能只信提交时的结论。
	// 两步都要过：先是不受配置影响的硬保底，再是规则集复检（与检测/审批同一份规则集）。
	if _, err := mongodb.Validate(cmd); err != nil {
		recordMongo(order.WorkId, order.SQL, "执行失败", 0, err.Error())
		return err
	}
	// 命中上限来自规则集：它是执行期限制（静态审核拿不到命中数），
	// 与引擎侧的审核规则共用同一份 AuditRole。
	rule, err := factory.CheckDataSourceRule(source.RuleId)
	if err != nil {
		recordMongo(order.WorkId, order.SQL, "执行失败", 0, "规则集加载失败："+err.Error())
		return errors.New("规则集加载失败：" + err.Error())
	}
	if err := recheckMongoRules(order, source, rule); err != nil {
		recordMongo(order.WorkId, order.SQL, "执行失败", 0, err.Error())
		return err
	}
	maxAffectRows := 0
	if rule != nil {
		maxAffectRows = rule.MongoMaxAffectRows
	}

	client, err := source.ConnectMongo()
	if err != nil {
		recordMongo(order.WorkId, order.SQL, "执行失败", 0, err.Error())
		return err
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), mongoExecTimeout)
	defer cancel()

	// createIndexes 是长时间的后台操作（大集合可达数分钟到数小时）：把构建进度
	// 写进 osc_info，复用详情页的「在线变更」面板。看不到进度不影响变更本身。
	if mongodb.CommandName(cmd) == "createindexes" {
		stopWatch := mongodb.WatchIndexBuild(ctx, client, order.DataBase, mongodb.CommandCollection(cmd), func(line string) {
			appendOSCInfo(order.WorkId, line)
		})
		defer stopWatch()
	}

	out, err := mongodb.Execute(ctx, client, order.DataBase, cmd, maxAffectRows)
	if err != nil {
		recordMongo(order.WorkId, order.SQL, "执行失败", 0, err.Error())
		return err
	}

	note := ""
	if out.Warning != "" {
		note = "提示：" + out.Warning
	}
	recordMongo(order.WorkId, order.SQL, "已执行", out.Affected, note)
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

// appendOSCInfo 追加一行在线变更进度。详情页面板整段渲染 osc_info，
// 所以是「读—追加—写」；只保留最近 200 行，长构建不会把这个字段撑爆。
func appendOSCInfo(workId, line string) {
	var o model.CoreSqlOrder
	model.DB().Select("osc_info").Where("work_id = ?", workId).First(&o)
	text := o.OSCInfo
	if text != "" {
		text += "\n"
	}
	text += line
	if lines := strings.Split(text, "\n"); len(lines) > 200 {
		text = strings.Join(lines[len(lines)-200:], "\n")
	}
	oscUpdate(workId, text)
}

// recheckMongoRules 执行前用**当前**规则集再审核一次。
//
// 为什么要再来一次：提交时只做硬保底（不读规则集），申请页检测与审批读的是**当时**的规则集——
// 审批期间规则集可能被改严，这个数据源也可能被换成另一套规则集。与 SQL 侧 client.Exec 同理，
// 执行前这一步才是最终裁决。引擎不可用时直接失败、不静默放行（同样与 SQL 侧一致）。
func recheckMongoRules(order *model.CoreSqlOrder, source *model.CoreDataSource, rule *engine.AuditRole) error {
	client, conn, err := calls.NewClient()
	if err != nil {
		return errors.New("规则复检失败（引擎不可用）：" + err.Error())
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), mongoRecheckTimeout)
	defer cancel()
	// DataSource 只给 kind：Mongo 审核是纯静态判定，不需要连目标库，
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

// recordMongo 写一条执行明细（与 SQL 侧共用 core_sql_records，详情页无需区分）
func recordMongo(workId, sql, state string, affected int64, note string) {
	rows := affected
	if rows < 0 {
		rows = 0
	}
	model.DB().Create(&model.CoreSqlRecord{
		WorkId:    workId,
		SQL:       sql,
		State:     state,
		Affectrow: uint(rows),
		Time:      time.Now().Format("2006-01-02 15:04"),
		Error:     note,
	})
}
