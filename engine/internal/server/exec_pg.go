package server

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	enginev1 "engine/gen/engine/v1"
	storepb "engine/internal/bytebase/generated-go/store"
	"engine/internal/mysqlparse"

	_ "github.com/jackc/pgx/v5/stdlib" // 注册 database/sql 的 postgres 驱动
)

// pgDSN 构造 PostgreSQL DSN。pg 的「库」是 schema：连接固定落在 postgres 库，
// 目标 schema 由 SET search_path 切换。
func pgDSN(s *enginev1.DataSource) string {
	user := s.Username
	if user == "" {
		user = "root"
	}
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=postgres sslmode=disable",
		s.Ip, s.Port, user, s.Password)
}

// execPG 执行 PostgreSQL 工单：用 pg 方言拆分/语法检查，pgx 连接，SET search_path 切 schema。
// 与 MySQL 的关键差异：没有 binlog / 前镜像回滚（pg 侧尚未实现，回滚语句留空）。
func (e *Engine) execPG(ctx context.Context, req *enginev1.ExecRequest) (*enginev1.ExecReply, error) {
	stmts, err := mysqlparse.SplitEngine(storepb.Engine_POSTGRES, req.Order.Sql)
	if err != nil || len(stmts) == 0 {
		return &enginev1.ExecReply{Ok: false, Error: "SQL 拆分失败"}, nil
	}

	db, err := sql.Open("pgx", pgDSN(req.Source))
	if err != nil {
		return &enginev1.ExecReply{Ok: false, Error: "打开数据库失败: " + err.Error()}, nil
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return &enginev1.ExecReply{Ok: false, Error: "连接数据库失败: " + err.Error()}, nil
	}
	if req.Order.DataBase != "" {
		if _, err = db.ExecContext(ctx, `SET search_path TO "`+escapePgIdent(req.Order.DataBase)+`"`); err != nil {
			return &enginev1.ExecReply{Ok: false, Error: "切换 schema 失败: " + err.Error()}, nil
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

	recs := make([]*enginev1.Record, 0, len(stmts))
	for _, text := range stmts {
		rec := &enginev1.Record{Sql: text, Status: "执行中", Level: 0}
		if perr := mysqlparse.CheckEngine(storepb.Engine_POSTGRES, text); perr != nil {
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
		res, err := run(text)
		if err != nil {
			rec.Error = err.Error()
			rec.Status = "执行失败"
			rec.Level = 1
			recs = append(recs, rec)
			if dmlTx {
				_ = tx.Rollback()
			}
			return &enginev1.ExecReply{Ok: false, Error: rec.Error, Records: recs}, nil
		}
		if n, err := res.RowsAffected(); err == nil {
			rec.AffectRows = uint32(n)
		}
		rec.Status = "已执行"
		recs = append(recs, rec)
	}

	if dmlTx {
		if err := tx.Commit(); err != nil {
			return &enginev1.ExecReply{Ok: false, Error: "提交事务失败: " + err.Error(), Records: recs}, nil
		}
	}
	// pg 侧暂无 binlog / 前镜像回滚，回滚语句留空
	return &enginev1.ExecReply{Ok: true, Records: recs}, nil
}

// escapePgIdent 在标识符进入双引号前把双引号双写（pg 用双引号引用标识符）。
func escapePgIdent(s string) string {
	return strings.ReplaceAll(s, `"`, `""`)
}
