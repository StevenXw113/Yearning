package server

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	enginev1 "engine/gen/engine/v1"
	"engine/internal/mysqlparse"

	_ "github.com/go-sql-driver/mysql"

	storepb "engine/internal/bytebase/generated-go/store"
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

// execSQL 执行 MySQL 工单：连接与切库后走共享骨架 execSQLStatements
// （事务、语法预检、影响行数上限），回滚两级方案的挂载在这里：
// binlog 抓取优先（capture 钩子），不可用时退回前镜像（buildRollback 钩子）。
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

	var buildRollback func(ctx context.Context, q rowQueryer, text string) (string, string)
	var capture func(ctx context.Context) ([]string, string)
	if req.Order.Backup == 1 {
		meta := newTableMeta(db, req.Order.DataBase)
		if c, ok := newBinlogCapture(ctx, req.Source, db, meta, req.Order.DataBase, stmts); ok {
			capture = func(ctx context.Context) ([]string, string) {
				return captureRollback(ctx, c, db)
			}
		} else {
			// 前镜像构建器按查询连接缓存（db 与事务各一份，主键/列信息不重复查询）
			var rbDB, rbTx *rollbackBuilder
			buildRollback = func(ctx context.Context, q rowQueryer, text string) (string, string) {
				b := rbDB
				if _, isTx := q.(*sql.Tx); isTx {
					if rbTx == nil {
						rbTx = newRollbackBuilder(q, req.Order.DataBase)
					}
					b = rbTx
				} else if b == nil {
					rbDB = newRollbackBuilder(q, req.Order.DataBase)
					b = rbDB
				}
				return b.build(ctx, text)
			}
		}
	}

	return execSQLStatements(ctx, db, stmts, req, storepb.Engine_MYSQL, buildRollback, capture)
}

// captureRollback 执行完成（含提交）后读取 binlog 位点并抓取整单回滚语句。
// 返回的 warning 非空表示回滚缺失/不完整，调用方必须呈现给审批人。
func captureRollback(ctx context.Context, blc *binlogCapture, db *sql.DB) ([]string, string) {
	if blc == nil {
		return nil, ""
	}
	end, ok := masterStatus(ctx, db)
	if !ok {
		return nil, "回滚抓取失败：执行后无法读取 binlog 位点，本工单没有自动回滚语句"
	}
	return blc.rollback(ctx, end)
}

// escapeSQLIdent 反引号内的反引号双写（MySQL 用反引号引用标识符）。
func escapeSQLIdent(s string) string {
	return strings.ReplaceAll(s, "`", "``")
}
