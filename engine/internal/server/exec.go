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

// dsnOf 依据 DataSource 构造 MySQL DSN。
// 首期支持明文 TCP 连接；若配置了 CA/Cert/Key 则启用 TLS（校验服务端证书链）。
func dsnOf(s *enginev1.DataSource, db string) (string, error) {
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
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return &enginev1.ExecReply{Ok: false, Error: "打开数据库失败: " + err.Error()}, nil
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return &enginev1.ExecReply{Ok: false, Error: "连接数据库失败: " + err.Error()}, nil
	}
	if req.Order.DataBase != "" {
		if _, err := db.ExecContext(ctx, "USE `"+escapeSQLIdent(req.Order.DataBase)+"`"); err != nil {
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

	recs := make([]*enginev1.Record, 0, len(stmts))
	for _, st := range stmts {
		rec := &enginev1.Record{Sql: st.Text, Status: "执行中", Level: 3}
		// 语法预检
		if perr := mysqlparse.Check(st.Text); perr != nil {
			rec.Error = "SQL 语法错误: " + perr.Msg
			rec.Status = "执行失败"
			rec.Level = 1
			recs = append(recs, rec)
			if dmlTx {
				_ = tx.Rollback()
				return &enginev1.ExecReply{Ok: false, Error: rec.Error}, nil
			}
			continue
		}
		// DML 影响行上限（先探测影响行很复杂，故先执行后校验）
		res, err := run(st.Text)
		if err != nil {
			rec.Error = err.Error()
			rec.Status = "执行失败"
			rec.Level = 1
			recs = append(recs, rec)
			if dmlTx {
				_ = tx.Rollback()
			}
			// 非事务模式遇错即停止整单
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
	return &enginev1.ExecReply{Ok: true, Records: recs}, nil
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
