package server

import (
	"context"
	"fmt"
	"strings"

	enginev1 "engine/gen/engine/v1"
	"engine/internal/customrules"
	"engine/internal/mysqlparse"

	storepb "engine/internal/bytebase/generated-go/store"
	bbadvisor "engine/internal/bytebase/plugin/advisor"
	_ "engine/internal/bytebase/plugin/advisor/mysql" // 注册 MySQL 审核规则实现（80+ 条）
	_ "engine/internal/bytebase/plugin/advisor/pg"    // 注册 PostgreSQL 审核规则实现（54 条）
	"engine/internal/bytebase/plugin/parser/base"
)

// Check 按数据源方言分派审核。五个引擎各自独立实现、互不混用：
//   - mongodb  → checkMongo（check_mongo.go）
//   - redis    → checkRedis（check_redis.go）
//   - postgres → checkPostgreSQL（check_pg.go）
//   - doris    → checkDoris（check_doris.go，建表走 dorischeck，DML/查询走 MySQL 解析器）
//   - mysql / 空 kind → checkSQL（check_sql.go；不填 kind 的历史调用方按 MySQL）
//
// 其余未知 kind 显式拒绝（fail-closed）：新增数据源若引擎侧未实现，静默按
// MySQL 审会产出误报/漏报都难察觉的假结果，宁可报「不支持」。
func (e *Engine) Check(ctx context.Context, req *enginev1.CheckRequest) (*enginev1.CheckReply, error) {
	if req == nil || strings.TrimSpace(req.Sql) == "" {
		return &enginev1.CheckReply{Ok: false, Error: "SQL 不能为空"}, nil
	}
	switch {
	case isMongoSource(req.Source):
		return checkMongo(req), nil
	case isRedisSource(req.Source):
		return checkRedis(req), nil
	case isPostgreSQLSource(req.Source):
		return checkPostgreSQL(ctx, req), nil
	case isDorisSource(req.Source):
		return checkDoris(ctx, req), nil
	}
	if k := strings.TrimSpace(req.Source.GetKind()); k != "" && !strings.EqualFold(k, "mysql") {
		return &enginev1.CheckReply{Ok: false, Error: "引擎暂不支持该数据源类型: " + k + "，请升级引擎或检查数据源配置"}, nil
	}
	return checkSQL(ctx, req), nil
}

// ---- 以下为 SQL 审核（MySQL / PostgreSQL 共用）的辅助 ----

// errText 把解析错误安全转成文案。
func errText(err error) string {
	if err == nil {
		return "语句为空"
	}
	return err.Error()
}

// review 对已解析的单条 SQL 依次执行规则，返回最严重的一条建议（error 优先于 warning）。
func review(ctx context.Context, engine storepb.Engine, stmts []base.ParsedStatement, schema string, rules []*storepb.SQLReviewRule) (*storepb.Advice, error) {
	size := 0
	for _, s := range stmts {
		size += len(s.Text)
	}
	checkCtx := bbadvisor.Context{
		DBType:              engine,
		CurrentDatabase:     schema,
		ParsedStatements:    stmts,
		StatementsTotalSize: size,
	}
	var warn *storepb.Advice
	for _, rule := range rules {
		checkCtx.Rule = rule
		advices, err := bbadvisor.Check(ctx, engine, rule.GetType(), checkCtx)
		if err != nil {
			if mysqlparse.IsUnknownAdvisorError(err) {
				// 该方言没有实现这条规则（例如 PostgreSQL 没有 MySQL 的自增/无符号/LIMIT 规则）。
				// 规则集是 MySQL 语义的、两方言共用，这里的缺失不是错误，跳过即可。
				continue
			}
			return nil, err
		}
		for _, a := range advices {
			if a.GetStatus() == storepb.Advice_ERROR {
				return a, nil
			}
			if warn == nil && a.GetStatus() == storepb.Advice_WARNING {
				warn = a
			}
		}
	}
	return warn, nil
}

// customStatus 把自研规则的级别渲染成与上游规则一致的状态文案。
func customStatus(l customrules.Level) string {
	switch l {
	case customrules.LevelWarning:
		return "警告"
	case customrules.LevelInfo:
		return "观察"
	default:
		return "审核不通过"
	}
}

// auditSQLStatement 单条语句的 SQL 方言审核：自研规则 → 语法解析 → advisor
// 错误组 → 降级组（警告/观察）。mysql / postgres / doris(DML) 三条路径共用，
// 方言差异只在 engine 参数；影响行数估算等方言专属增强由调用方在返回的
// Record 上追加。返回的 parsed 供调用方复用（避免重复解析）。
func auditSQLStatement(ctx context.Context, engine storepb.Engine, text, schema string, rule *enginev1.AuditRole, plan reviewPlan) (*enginev1.Record, []base.ParsedStatement) {
	rec := &enginev1.Record{Sql: text, Schema: schema, Status: "审核通过", Level: 0}
	if f, ok := runCustomRules(rule, text, schema); ok {
		rec.Error = fmt.Sprintf("[%s] %s", f.Title, f.Content)
		rec.Status, rec.Level = customStatus(f.Level), uint32(f.Level)
		return rec, nil
	}
	parsed, perr := base.ParseStatements(engine, text)
	switch {
	case perr != nil || len(parsed) == 0:
		rec.Error, rec.Status, rec.Level = "SQL 语法错误: "+errText(perr), "语法错误", 1
	default:
		advice, rerr := review(ctx, engine, parsed, schema, plan.error)
		switch {
		case rerr != nil:
			rec.Error, rec.Status, rec.Level = "SQL 语法错误: "+rerr.Error(), "语法错误", 1
		case advice != nil:
			rec.Error, rec.Status, rec.Level = adviceText(advice), "审核不通过", 1
		default:
			rec.Error, rec.Status, rec.Level = downgraded(ctx, engine, parsed, schema, plan)
		}
	}
	return rec, parsed
}

// findingsRecord 把自研规则包（mongocheck / redischeck / dorischeck，三者助手
// 同构）的命中映射成统一 Record：msgs 为空表示未命中，保持「审核通过」；
// 命中时按 blocked 区分拦截与仅提示（警告/观察不拦提交，与 SQL 侧降级语义一致）。
func findingsRecord(sql, schema, msgs string, severity int, blocked bool) *enginev1.Record {
	rec := &enginev1.Record{Sql: sql, Schema: schema, Status: "审核通过", Level: 0}
	if msgs == "" {
		return rec
	}
	rec.Error = msgs
	rec.Level = uint32(severity)
	rec.Status = "审核不通过"
	if !blocked {
		if rec.Level == 2 {
			rec.Status = "警告"
		} else {
			rec.Status = "观察"
		}
	}
	return rec
}

// adviceText 把 Bytebase 建议渲染成检测结果里的错误文案。
func adviceText(a *storepb.Advice) string {
	return fmt.Sprintf("[%s] %s", a.GetTitle(), a.GetContent())
}

// downgraded 在错误级规则未命中时，按「警告 → 观察」继续判定，返回记录需要的
// 文案/状态/级别（Yearning 约定：0 通过 1 错误 2 警告 3 信息，前端只拦 level===1）。
// 两组都不命中时返回空文案，记录保持「审核通过」。
func downgraded(ctx context.Context, engine storepb.Engine, stmts []base.ParsedStatement, schema string, plan reviewPlan) (string, string, uint32) {
	for _, g := range []struct {
		rules []*storepb.SQLReviewRule
		text  string
		level uint32
	}{
		{plan.warning, "警告", 2},
		{plan.observe, "观察", 3},
	} {
		advice, err := review(ctx, engine, stmts, schema, g.rules)
		if err != nil || advice == nil {
			continue
		}
		return adviceText(advice), g.text, g.level
	}
	return "", "", 0
}

// runCustomRules 执行自研规则层（internal/customrules），返回最严重的一条命中。
// role 为 nil（未配置审核规则）时不启用自研规则，与历史语义保持一致。
//
// 自研规则与上游规则分属两层：前者在 internal/customrules（同步不覆盖），
// 后者在 internal/bytebase（可被 sync-bytebase.sh 整体重建）。
func runCustomRules(role *enginev1.AuditRole, sql, schema string) (customrules.Finding, bool) {
	if role == nil {
		return customrules.Finding{}, false
	}
	cfg := customrules.Config{
		AllowDropDatabase: role.GetDdlEnableDropDatabase(),
		AllowDropTable:    role.GetDdlEnableDropTable(),
		ForbidTruncate:    role.GetDdlForbidTruncate(),
	}
	ctx := customrules.Context{SQL: sql, Schema: schema}

	var hit []customrules.Finding
	for _, r := range customrules.All() {
		hit = append(hit, r.Check(cfg, ctx)...)
	}
	if len(hit) == 0 {
		return customrules.Finding{}, false
	}
	best := hit[0]
	for _, f := range hit[1:] {
		if f.Level < best.Level { // 数值越小越严重
			best = f
		}
	}
	return best, true
}
