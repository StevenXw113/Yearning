package server

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	enginev1 "engine/gen/engine/v1"
	storepb "engine/internal/bytebase/generated-go/store"
	"engine/internal/mysqlparse"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib" // 注册 database/sql 的 postgres 驱动
)

// Exec 按数据源方言分派执行：
//   - mongodb / redis → execCommand（命令式引擎，见 exec_command.go）
//   - postgres          → execPG
//   - 其余              → execSQL（MySQL）
//
// 三种路径各自独立，不再在 MySQL 实现里用 isPg 分支混入 pg 的逻辑。
func (e *Engine) Exec(ctx context.Context, req *enginev1.ExecRequest) (*enginev1.ExecReply, error) {
	if req == nil || req.Order == nil {
		return &enginev1.ExecReply{Ok: false, Error: "工单为空"}, nil
	}
	if strings.TrimSpace(req.Order.Sql) == "" {
		return &enginev1.ExecReply{Ok: false, Error: "工单 SQL 为空"}, nil
	}

	switch strings.ToLower(req.Source.GetKind()) {
	case "mongodb", "redis":
		return e.execCommand(ctx, req)
	case "postgres":
		return e.execPG(ctx, req)
	default:
		return e.execSQL(ctx, req)
	}
}

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

func orderType(t int32) string {
	if t == 1 {
		return "dml"
	}
	return "ddl"
}

func escapeSQLIdent(s string) string {
	return strings.ReplaceAll(s, "`", "``")
}

// escapePgIdent 在标识符进入双引号前把双引号双写（pg 用双引号引用标识符）。
func escapePgIdent(s string) string {
	return strings.ReplaceAll(s, `"`, `""`)
}
