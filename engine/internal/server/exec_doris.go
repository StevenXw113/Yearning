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
// MySQL 解析器拆不了，所以这里不做语法解析、只按分号切分；每条语句的语法对错
// 交给 Doris 在执行时报。引号内的分号跳过，注释里的分号也跳过——
// `-- xx;` / `# xx;` / `/* xx; */` 里的分号属于注释文本，切开会拼出残缺语句。
func splitDorisStatements(sql string) []string {
	var (
		stmts []string
		buf   strings.Builder
		quote byte
	)
	for i := 0; i < len(sql); i++ {
		c := sql[i]
		if quote != 0 {
			// 反斜杠转义只在字符串内有效（反引号标识符内 '\' 是普通字符）
			if c == '\\' && quote != '`' && i+1 < len(sql) {
				i++
				buf.WriteByte(c)
				buf.WriteByte(sql[i])
				continue
			}
			if c == quote {
				// 引号双写（'' / "" / ``）是转义，不结束引号
				if i+1 < len(sql) && sql[i+1] == quote {
					i++
					buf.WriteByte(c)
					buf.WriteByte(sql[i])
					continue
				}
				quote = 0
			}
			buf.WriteByte(c)
			continue
		}
		// 行注释：-- 后须跟空白/行尾（与 MySQL 词法一致，不误伤 --x 这类负号表达式）
		if c == '-' && i+1 < len(sql) && sql[i+1] == '-' &&
			(i+2 >= len(sql) || sql[i+2] == ' ' || sql[i+2] == '\t' || sql[i+2] == '\r' || sql[i+2] == '\n') {
			for ; i < len(sql) && sql[i] != '\n'; i++ {
			}
			if i < len(sql) {
				buf.WriteByte('\n')
			}
			continue
		}
		if c == '#' {
			for ; i < len(sql) && sql[i] != '\n'; i++ {
			}
			if i < len(sql) {
				buf.WriteByte('\n')
			}
			continue
		}
		// 块注释 /* ... */（含 /*! ... */ 版本注释，切分层面一律按注释对待）
		if c == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			end := strings.Index(sql[i+2:], "*/")
			buf.WriteByte(' ')
			if end < 0 { // 未闭合：吞掉剩余全部
				break
			}
			i = i + 2 + end + 1 // 指向 "*/" 的 '/'，随 for 的 i++ 落到注释后
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
			buf.WriteByte(c)
		case ';':
			if s := strings.TrimSpace(buf.String()); s != "" {
				stmts = append(stmts, s)
			}
			buf.Reset()
		default:
			buf.WriteByte(c)
		}
	}
	if s := strings.TrimSpace(buf.String()); s != "" {
		stmts = append(stmts, s)
	}
	return stmts
}

// execDoris 执行 Doris 工单：按分号切分后逐条执行。
//
// Doris 是分析型 MPP：DML 不支持事务、也没有 binlog 前镜像，
// 与 PostgreSQL 一样接受「不可回滚」，回滚语句留空，执行结果照常写 Records。
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
		// 规则集影响行数上限：超限即中止（已执行的不可撤回，但不再放行后续语句）
		if over, limit := affectRowOverLimit(req.Rules, orderType(req.Order.Type), rec.AffectRows); over {
			rec.Error = fmt.Sprintf("影响行数 %d 超过规则集上限 %d，已中止执行", rec.AffectRows, limit)
			rec.Status = "执行失败"
			rec.Level = 1
			recs = append(recs, rec)
			return &enginev1.ExecReply{Ok: false, Error: rec.Error, Records: recs}, nil
		}
		rec.Status = "已执行"
		recs = append(recs, rec)
	}
	// Doris 无 binlog / 前镜像回滚，回滚语句留空
	return &enginev1.ExecReply{Ok: true, Records: recs}, nil
}
