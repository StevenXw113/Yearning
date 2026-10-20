package server

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	enginev1 "engine/gen/engine/v1"
	"engine/internal/mysqlparse"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib" // 注册 database/sql 的 postgres 驱动
)

// dsnOf 依据 DataSource 构造 SQL 方言的 DSN（MySQL 或 PostgreSQL）。
// 首期支持明文 TCP 连接；若配置了 CA/Cert/Key 则启用 TLS（校验服务端证书链）。
// pg 的「库」是 schema：连接固定落在 postgres 库，schema 由 SET search_path 切换。
func dsnOf(s *enginev1.DataSource, db string) (string, error) {
	if s == nil || s.Ip == "" {
		return "", fmt.Errorf("数据源 IP 为空")
	}
	user := s.Username
	if user == "" {
		user = "root"
	}
	if strings.EqualFold(s.GetKind(), "postgres") {
		return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=postgres sslmode=disable",
			s.Ip, s.Port, user, s.Password), nil
	}
	tlsParam := ""
	if s.Ca != "" || s.Cert != "" || s.Key != "" {
		tlsParam = "&tls=preferred"
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=Local%s",
		user, s.Password, s.Ip, s.Port, db, tlsParam), nil
}

// Exec 在目标数据源执行工单 SQL。
// 逐条拆分执行，返回影响行与错误明细；DML 超限或事务开启时按规则处理。
func (e *Engine) Exec(ctx context.Context, req *enginev1.ExecRequest) (*enginev1.ExecReply, error) {
	if req == nil || req.Order == nil {
		return &enginev1.ExecReply{Ok: false, Error: "工单为空"}, nil
	}
	if strings.TrimSpace(req.Order.Sql) == "" {
		return &enginev1.ExecReply{Ok: false, Error: "工单 SQL 为空"}, nil
	}

	stmts, err := mysqlparse.Split(req.Order.Sql)
	if err != nil || len(stmts) == 0 {
		return &enginev1.ExecReply{Ok: false, Error: "SQL 拆分失败"}, nil
	}

	dsn, err := dsnOf(req.Source, req.Order.DataBase)
	if err != nil {
		return &enginev1.ExecReply{Ok: false, Error: err.Error()}, nil
	}
	isPg := strings.EqualFold(req.Source.GetKind(), "postgres")
	driverName := "mysql"
	if isPg {
		driverName = "pgx"
	}
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return &enginev1.ExecReply{Ok: false, Error: "打开数据库失败: " + err.Error()}, nil
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return &enginev1.ExecReply{Ok: false, Error: "连接数据库失败: " + err.Error()}, nil
	}
	if req.Order.DataBase != "" {
		// 切库：MySQL 用 USE，pg 用 SET search_path（pg 的「库」是 schema）
		if isPg {
			_, err = db.ExecContext(ctx, `SET search_path TO "`+escapePgIdent(req.Order.DataBase)+`"`)
		} else {
			_, err = db.ExecContext(ctx, "USE `"+escapeSQLIdent(req.Order.DataBase)+"`")
		}
		if err != nil {
			return &enginev1.ExecReply{Ok: false, Error: "切换 schema 失败: " + err.Error()}, nil
		}
	}

	// DML 且开启了事务则整体包裹。
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

	// 工单要求回滚语句（backup=1）时优先用 binlog 抓真实行镜像，覆盖多表/无主键/全表/INSERT...SELECT
	// 等全部 DML 场景；binlog 不可用（未开启或无 REPLICATION 权限）再退回执行前 SELECT 前镜像。
	var (
		rb  *rollbackBuilder
		blc *binlogCapture
	)
	// 回滚只对 MySQL 实现（binlog 与前镜像都基于 MySQL 的 information_schema/binlog）；
	// pg 工单暂不生成回滚语句，后续需要时再按 pg 的 pg_catalog 重写前镜像。
	if req.Order.Backup == 1 && !isPg {
		meta := newTableMeta(db, req.Order.DataBase)
		if c, ok := newBinlogCapture(ctx, req.Source, db, meta, req.Order.DataBase, stmts); ok {
			blc = c
		} else {
			var q rowQueryer = db
			if dmlTx {
				q = tx // 事务内取数，保证能看到同事务前面语句的改动
			}
			rb = newRollbackBuilder(q, req.Order.DataBase)
		}
	}

	recs := make([]*enginev1.Record, 0, len(stmts))
	for _, text := range stmts {
		rec := &enginev1.Record{Sql: text, Status: "执行中", Level: 0}
		// 语法预检
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
		// 回滚语句必须在语句执行前生成（依赖改动前的数据）
		if rb != nil {
			rec.Rollback = rb.build(ctx, text)
		}
		// DML 影响行上限（先探测影响行很复杂，故先执行后校验）
		res, err := run(text)
		if err != nil {
			rec.Error = err.Error()
			rec.Status = "执行失败"
			rec.Level = 1
			recs = append(recs, rec)
			if dmlTx {
				_ = tx.Rollback()
			}
			// 非事务模式遇错即停止整单；前面已生效的语句仍要给出回滚语句
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

// captureRollback 执行完成（含提交）后读取 binlog 位点并抓取整单回滚语句。
// blc 为空表示走的是前镜像方案（回滚语句已按条挂在 records 上）。
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
