package server

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	enginev1 "engine/gen/engine/v1"

	_ "github.com/go-sql-driver/mysql" // Doris 兼容 MySQL 协议，用 mysql driver 直连
)

// dorisDSN 构造 Doris 连接串。Doris 兼容 MySQL 协议（FE 的 MySQL 协议端口），
// 用 go-sql-driver/mysql 直连即可。
func dorisDSN(s *enginev1.DataSource, db string) (string, error) {
	if s == nil || s.Ip == "" {
		return "", fmt.Errorf("数据源 IP 为空")
	}
	user := s.Username
	if user == "" {
		user = "root"
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4", user, s.Password, s.Ip, s.Port, db), nil
}

// splitDorisStatements 按分号切分 Doris 语句。
//
// Doris 的建表 DDL 是自有语法（DISTRIBUTED BY / PROPERTIES / KEY 模型），
// MySQL 解析器拆不了，所以这里不做语法解析、只按分号切分（字符串字面量里的
// 分号与反斜杠转义跳过）；每条语句的语法对错交给 Doris 在执行时报。
func splitDorisStatements(sql string) []string {
	var (
		stmts   []string
		buf     strings.Builder
		quote   rune
		escaped bool
	)
	for _, r := range sql {
		if escaped {
			buf.WriteRune(r)
			escaped = false
			continue
		}
		if quote == 0 {
			switch r {
			case '\'', '"', '`':
				quote = r
			case ';':
				if s := strings.TrimSpace(buf.String()); s != "" {
					stmts = append(stmts, s)
				}
				buf.Reset()
				continue
			}
		} else {
			if r == '\\' {
				escaped = true
			} else if r == quote {
				quote = 0
			}
		}
		buf.WriteRune(r)
	}
	if s := strings.TrimSpace(buf.String()); s != "" {
		stmts = append(stmts, s)
	}
	return stmts
}

// execDoris 执行 Doris 工单：按分号切分后逐条执行。
//
// Doris 是分析型 MPP：DML 不支持事务、也没有 binlog 前镜像，
// 与 pg 一样接受「不可回滚」，回滚语句留空，执行结果照常写 Records。
func (e *Engine) execDoris(ctx context.Context, req *enginev1.ExecRequest) (*enginev1.ExecReply, error) {
	stmts := splitDorisStatements(req.Order.Sql)
	if len(stmts) == 0 {
		return &enginev1.ExecReply{Ok: false, Error: "SQL 拆分失败"}, nil
	}

	dsn, err := dorisDSN(req.Source, req.Order.DataBase)
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

	recs := make([]*enginev1.Record, 0, len(stmts))
	for _, text := range stmts {
		rec := &enginev1.Record{Sql: text, Status: "执行中", Level: 0}
		res, err := db.ExecContext(ctx, text)
		if err != nil {
			rec.Error = err.Error()
			rec.Status = "执行失败"
			rec.Level = 1
			recs = append(recs, rec)
			return &enginev1.ExecReply{Ok: false, Error: rec.Error, Records: recs}, nil
		}
		if n, err := res.RowsAffected(); err == nil {
			rec.AffectRows = uint32(n)
		}
		rec.Status = "已执行"
		recs = append(recs, rec)
	}
	// Doris 无 binlog / 前镜像回滚，回滚语句留空
	return &enginev1.ExecReply{Ok: true, Records: recs}, nil
}
