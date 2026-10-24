package server

import (
	"context"
	"database/sql"
	"fmt"

	enginev1 "engine/gen/engine/v1"
	"engine/internal/mysqlparse"

	storepb "engine/internal/bytebase/generated-go/store"
)

// execSQLStatements mysql / postgres 共用的逐条执行骨架：
// 可选事务 → 逐条（语法预检 → 前镜像回滚构建 → 执行 → 影响行数上限复检）→ 提交。
// Doris 不走这里（自有 DDL 无法做语法预检、无事务），见 exec_doris.go。
//
// buildRollback 为前镜像路径的每条语句回滚构建，入参 q 是执行用的连接/事务
// （保证前镜像读到同一事务内的状态）；无该能力的引擎传 nil。
// capture 在工单中止或完成后抓取整单回滚（binlog 路径）；传 nil 跳过。
//
// 语法预检失败即中止整单：语句本就由同一解析器切分，预检再失败说明内容有问题，
// 静默跳过并返回 Ok=true 会让工单带着缺口标记成功。
func execSQLStatements(
	ctx context.Context,
	db *sql.DB,
	stmts []string,
	req *enginev1.ExecRequest,
	dialect storepb.Engine,
	buildRollback func(ctx context.Context, q rowQueryer, text string) (string, string),
	capture func(ctx context.Context) ([]string, string),
) (*enginev1.ExecReply, error) {
	tx, dmlTx := (*sql.Tx)(nil), false
	if req.Rules != nil && req.Rules.DmlTransaction && orderType(req.Order.Type) == "dml" {
		txNew, err := db.BeginTx(ctx, nil)
		if err != nil {
			return &enginev1.ExecReply{Ok: false, Error: "开启事务失败: " + err.Error()}, nil
		}
		tx, dmlTx = txNew, true
	}
	run := func(q string) (sql.Result, error) {
		if dmlTx {
			return tx.ExecContext(ctx, q)
		}
		return db.ExecContext(ctx, q)
	}

	// finish 收尾：失败回滚事务（binlog 抓取照做——事务已回滚时抓不到本工单事件，
	// 空结果无害）；成功先提交再抓取（提交后的位点才能覆盖全部改动）。
	finish := func(ok bool, msg string, recs []*enginev1.Record) (*enginev1.ExecReply, error) {
		if !ok {
			if dmlTx {
				_ = tx.Rollback()
			}
			var rb []string
			if capture != nil {
				var warn string
				rb, warn = capture(ctx)
				attachRollbackWarning(recs, warn)
			}
			return &enginev1.ExecReply{Ok: false, Error: msg, Records: recs, Rollback: rb}, nil
		}
		if dmlTx {
			if err := tx.Commit(); err != nil {
				return &enginev1.ExecReply{Ok: false, Error: "提交事务失败: " + err.Error(), Records: recs}, nil
			}
		}
		var rb []string
		if capture != nil {
			var warn string
			rb, warn = capture(ctx)
			attachRollbackWarning(recs, warn)
		}
		return &enginev1.ExecReply{Ok: true, Records: recs, Rollback: rb}, nil
	}

	recs := make([]*enginev1.Record, 0, len(stmts))
	for _, text := range stmts {
		rec := &enginev1.Record{Sql: text, Status: "执行中", Level: 0}
		if perr := mysqlparse.CheckEngine(dialect, text); perr != nil {
			rec.Error = "SQL 语法错误: " + perr.Error()
			rec.Status, rec.Level = "执行失败", 1
			recs = append(recs, rec)
			return finish(false, rec.Error, recs)
		}
		var rwarn string
		if buildRollback != nil {
			var q rowQueryer = db
			if dmlTx {
				q = tx
			}
			rec.Rollback, rwarn = buildRollback(ctx, q, text)
		}
		res, err := run(text)
		if err != nil {
			rec.Error = err.Error()
			rec.Status, rec.Level = "执行失败", 1
			if rwarn != "" {
				rec.Error += "\n" + rwarn
			}
			recs = append(recs, rec)
			return finish(false, rec.Error, recs)
		}
		if n, err := res.RowsAffected(); err == nil {
			rec.AffectRows = uint32(n)
		}
		// 规则集影响行数上限：超限即中止（已执行的不可撤回，但不再放行后续语句）
		if over, limit := affectRowOverLimit(req.Rules, orderType(req.Order.Type), rec.AffectRows); over {
			rec.Error = fmt.Sprintf("影响行数 %d 超过规则集上限 %d，已中止执行", rec.AffectRows, limit)
			rec.Status, rec.Level = "执行失败", 1
			recs = append(recs, rec)
			return finish(false, rec.Error, recs)
		}
		if rwarn != "" { // 前镜像回滚的不完整提示（状态仍为已执行）
			rec.Error = rwarn
		}
		rec.Status = "已执行"
		recs = append(recs, rec)
	}
	return finish(true, "", recs)
}
