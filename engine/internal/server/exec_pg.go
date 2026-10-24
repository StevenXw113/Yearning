package server

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	enginev1 "engine/gen/engine/v1"
	storepb "engine/internal/bytebase/generated-go/store"
	"engine/internal/mysqlparse"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// pgConnect 打开 pg 连接：目标 schema（Yearning 里 pg 的「库」）通过连接参数
// search_path 生效——它在每条连接建立时下发，不再依赖「池上执行 SET search_path」。
// 那种写法只影响当时拿到的那条连接，后续语句被 database/sql 分到另一条连接时
// 会话状态就静默丢失（断线重连、事务换连接都会触发）。
//
// 库名先试 postgres，连不上再试与用户同名的库：托管/最小权限实例常只授权业务库。
func pgConnect(ctx context.Context, s *enginev1.DataSource, schema string) (*sql.DB, error) {
	if s == nil || s.Ip == "" {
		return nil, fmt.Errorf("数据源 IP 为空")
	}
	user := s.Username
	if user == "" {
		user = "postgres"
	}
	base := fmt.Sprintf("host=%s port=%d user=%s password=%s sslmode=disable",
		pgDSNValue(s.Ip), s.Port, pgDSNValue(user), pgDSNValue(s.Password))

	var lastErr error
	for _, dbname := range pgDatabaseCandidates(user) {
		cfg, err := pgx.ParseConfig(base + " dbname=" + pgDSNValue(dbname))
		if err != nil {
			return nil, err
		}
		if schema != "" {
			// 值按双引号标识符转义，与旧实现 SET search_path TO "<schema>" 的语义一致
			cfg.RuntimeParams["search_path"] = `"` + strings.ReplaceAll(schema, `"`, `""`) + `"`
		}
		db := sql.OpenDB(stdlib.GetConnector(*cfg))
		if err := db.PingContext(ctx); err != nil {
			_ = db.Close()
			lastErr = fmt.Errorf("连接库 %s 失败: %w", dbname, err)
			continue
		}
		return db, nil
	}
	return nil, lastErr
}

// pgDatabaseCandidates 默认库候选：postgres 之外再试与用户同名的库
// （RDS 等托管实例上常见「只授权 $user 库」的最小权限账号）。
func pgDatabaseCandidates(user string) []string {
	if user == "" || user == "postgres" {
		return []string{"postgres"}
	}
	return []string{"postgres", user}
}

// pgDSNValue DSN 值只含安全字符就直接拼；含空白/引号等则单引号包裹（内部引号双写），
// 否则口令或库名里的空格会把 keyword=value 连接串拆坏。
func pgDSNValue(s string) string {
	if s == "" {
		return "''"
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_' || r == '.' || r == '-' || r == '/' || r == ':':
		default:
			return "'" + strings.ReplaceAll(s, "'", "''") + "'"
		}
	}
	return s
}

// execPG 执行 PostgreSQL 工单：用 pg 方言拆分/语法检查，pgx 连接（search_path 已在连接参数里）。
// 与 MySQL 的关键差异：没有 binlog / 前镜像回滚（pg 侧尚未实现，回滚语句留空）。
func (e *Engine) execPG(ctx context.Context, req *enginev1.ExecRequest) (*enginev1.ExecReply, error) {
	stmts, err := mysqlparse.SplitEngine(storepb.Engine_POSTGRES, req.Order.Sql)
	if err != nil || len(stmts) == 0 {
		return &enginev1.ExecReply{Ok: false, Error: "SQL 拆分失败"}, nil
	}

	db, err := pgConnect(ctx, req.Source, req.Order.DataBase)
	if err != nil {
		return &enginev1.ExecReply{Ok: false, Error: "连接数据库失败: " + err.Error()}, nil
	}
	defer db.Close()

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
		// 规则集影响行数上限：超限即中止（已执行的不可撤回，但不再放行后续语句）
		if over, limit := affectRowOverLimit(req.Rules, orderType(req.Order.Type), rec.AffectRows); over {
			rec.Error = fmt.Sprintf("影响行数 %d 超过规则集上限 %d，已中止执行", rec.AffectRows, limit)
			rec.Status = "执行失败"
			rec.Level = 1
			recs = append(recs, rec)
			if dmlTx {
				_ = tx.Rollback()
			}
			return &enginev1.ExecReply{Ok: false, Error: rec.Error, Records: recs}, nil
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
