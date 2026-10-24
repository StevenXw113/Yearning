package server

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	enginev1 "engine/gen/engine/v1"
	"engine/internal/mysqlparse"

	_ "github.com/go-sql-driver/mysql"
)

// mysqlDSN 构造 MySQL DSN。首期支持明文 TCP；配了 CA/Cert/Key 则启用 TLS。
func mysqlDSN(s *enginev1.DataSource, db string) (string, error) {
	if s == nil || s.Ip == "" {
		return "", fmt.Errorf("数据源 IP 为空")
	}
	user := s.Username
	if user == "" {
		user = "root"
	}
	tlsParam := ""
	if s.Ca != "" || s.Cert != "" || s.Key != "" {
		tlsParam = "&tls=preferred"
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=Local%s",
		user, s.Password, s.Ip, s.Port, db, tlsParam), nil
}

// execSQL 执行 MySQL 工单：逐条拆分执行 + binlog / 前镜像回滚。
func (e *Engine) execSQL(ctx context.Context, req *enginev1.ExecRequest) (*enginev1.ExecReply, error) {
	stmts, err := mysqlparse.Split(req.Order.Sql)
	if err != nil || len(stmts) == 0 {
		return &enginev1.ExecReply{Ok: false, Error: "SQL 拆分失败"}, nil
	}

	dsn, err := mysqlDSN(req.Source, req.Order.DataBase)
	if err != nil {
		return &enginev1.ExecReply{Ok: false, Error: err.Error()}, nil
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return &enginev1.ExecReply{Ok: false, Error: "打开数据库失败: " + err.Error()}, nil
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return &enginev1.ExecReply{Ok: false, Error: "连接数据库失败: " + err.Error()}, nil
	}
	if req.Order.DataBase != "" {
		if _, err = db.ExecContext(ctx, "USE `"+escapeSQLIdent(req.Order.DataBase)+"`"); err != nil {
			return &enginev1.ExecReply{Ok: false, Error: "切换库失败: " + err.Error()}, nil
		}
	}

	tx, dmlTx := (*sql.Tx)(nil), false
	if req.Rules != nil && req.Rules.DmlTransaction && orderType(req.Order.Type) == "dml" {
		tx, err = db.BeginTx(ctx, nil)
		if err != nil {
			return &enginev1.ExecReply{Ok: false, Error: "开启事务失败: " + err.Error()}, nil
		}
		dmlTx = true
	}
	run := func(q string) (sql.Result, error) {
		if dmlTx {
			return tx.ExecContext(ctx, q)
		}
		return db.ExecContext(ctx, q)
	}

	// 回滚语句（binlog 优先，退回前镜像）
	var (
		rb  *rollbackBuilder
		blc *binlogCapture
	)
	if req.Order.Backup == 1 {
		meta := newTableMeta(db, req.Order.DataBase)
		if c, ok := newBinlogCapture(ctx, req.Source, db, meta, req.Order.DataBase, stmts); ok {
			blc = c
		} else {
			var q rowQueryer = db
			if dmlTx {
				q = tx
			}
			rb = newRollbackBuilder(q, req.Order.DataBase)
		}
	}

	recs := make([]*enginev1.Record, 0, len(stmts))
	for _, text := range stmts {
		rec := &enginev1.Record{Sql: text, Status: "执行中", Level: 0}
		if perr := mysqlparse.Check(text); perr != nil {
			rec.Error = "SQL 语法错误: " + perr.Error()
			rec.Status = "执行失败"
			rec.Level = 1
			recs = append(recs, rec)
			if dmlTx {
				_ = tx.Rollback()
				return &enginev1.ExecReply{Ok: false, Error: rec.Error, Records: recs}, nil
			}
			continue
		}
		if rb != nil {
			rec.Rollback = rb.build(ctx, text)
		}
		res, err := run(text)
		if err != nil {
			rec.Error = err.Error()
			rec.Status = "执行失败"
			rec.Level = 1
			recs = append(recs, rec)
			if dmlTx {
				_ = tx.Rollback()
			}
			return &enginev1.ExecReply{Ok: false, Error: rec.Error, Records: recs, Rollback: captureRollback(ctx, blc, db)}, nil
		}
		if n, err := res.RowsAffected(); err == nil {
			rec.AffectRows = uint32(n)
		}
		// 规则集影响行数上限：超限即中止（已执行的不可撤回，但不再放行后续语句）
		if over, limit := affectRowOverLimit(req.Rules, orderType(req.Order.Type), rec.AffectRows); over {
			rec.Error = fmt.Sprintf("影响行数 %d 超过规则集上限 %d，已中止执行", rec.AffectRows, limit)
			rec.Status = "执行失败"
			rec.Level = 1
			recs = append(recs, rec)
			if dmlTx {
				_ = tx.Rollback()
			}
			// 与执行失败路径一致：已生效的部分改动仍要抓回滚语句
			return &enginev1.ExecReply{Ok: false, Error: rec.Error, Records: recs, Rollback: captureRollback(ctx, blc, db)}, nil
		}
		rec.Status = "已执行"
		recs = append(recs, rec)
	}

	if dmlTx {
		if err := tx.Commit(); err != nil {
			return &enginev1.ExecReply{Ok: false, Error: "提交事务失败: " + err.Error(), Records: recs}, nil
		}
	}
	return &enginev1.ExecReply{Ok: true, Records: recs, Rollback: captureRollback(ctx, blc, db)}, nil
}

// captureRollback 执行完成（含提交）后读取 binlog 位点并抓取整单回滚语句。
func captureRollback(ctx context.Context, blc *binlogCapture, db *sql.DB) []string {
	if blc == nil {
		return nil
	}
	end, ok := masterStatus(ctx, db)
	if !ok {
		return nil
	}
	return blc.rollback(ctx, end)
}

// escapeSQLIdent 反引号内的反引号双写（MySQL 用反引号引用标识符）。
func escapeSQLIdent(s string) string {
	return strings.ReplaceAll(s, "`", "``")
}
