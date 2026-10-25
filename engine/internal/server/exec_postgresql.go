package server

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"engine/api"
	"engine/internal/mysqlparse"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	storepb "engine/internal/bytebase/generated-go/store"
)

// connectPostgreSQL 打开 PostgreSQL 连接：目标 schema（Yearning 里 PostgreSQL 的「库」）通过连接参数
// search_path 生效——它在每条连接建立时下发，不再依赖「池上执行 SET search_path」。
// 那种写法只影响当时拿到的那条连接，后续语句被 database/sql 分到另一条连接时
// 会话状态就静默丢失（断线重连、事务换连接都会触发）。
//
// 库名先试 postgres，连不上再试与用户同名的库：托管/最小权限实例常只授权业务库。
func connectPostgreSQL(ctx context.Context, s *api.DataSource, schema string) (*sql.DB, error) {
	if s == nil || s.Ip == "" {
		return nil, fmt.Errorf("数据源 IP 为空")
	}
	user := s.Username
	if user == "" {
		user = "postgres"
	}
	base := fmt.Sprintf("host=%s port=%d user=%s password=%s sslmode=disable",
		postgresqlDSNValue(s.Ip), s.Port, postgresqlDSNValue(user), postgresqlDSNValue(s.Password))

	var lastErr error
	for _, dbname := range postgresqlDatabaseCandidates(user) {
		// search_path 是连接级参数,必须随 schema 进缓存键
		poolKey := "pg|" + base + "|" + dbname + "|" + schema
		if v, ok := execPools.Load(poolKey); ok {
			return v.(*sql.DB), nil
		}
		cfg, err := pgx.ParseConfig(base + " dbname=" + postgresqlDSNValue(dbname))
		if err != nil {
			return nil, err
		}
		if schema != "" {
			// 值按双引号标识符转义，与旧实现 SET search_path TO "<schema>" 的语义一致
			cfg.RuntimeParams["search_path"] = `"` + strings.ReplaceAll(schema, `"`, `""`) + `"`
		}
		// pgx 连接器带 RuntimeParams(search_path),走通用 execPool 的 sql.Open 路径不可行
		pgdb := sql.OpenDB(stdlib.GetConnector(*cfg))
		pgdb.SetConnMaxIdleTime(time.Minute)
		pgdb.SetConnMaxLifetime(time.Minute * 3)
		pgdb.SetMaxOpenConns(20)
		pgdb.SetMaxIdleConns(5)
		if err := pgdb.PingContext(ctx); err != nil {
			_ = pgdb.Close()
			execPools.Delete(poolKey)
			lastErr = fmt.Errorf("连接库 %s 失败: %w", dbname, err)
			continue
		}
		actual, loaded := execPools.LoadOrStore(poolKey, pgdb)
		if loaded {
			pgdb.Close()
			return actual.(*sql.DB), nil
		}
		return pgdb, nil
	}
	return nil, lastErr
}

// postgresqlDatabaseCandidates 默认库候选：postgres 之外再试与用户同名的库
// （RDS 等托管实例上常见「只授权 $user 库」的最小权限账号）。
func postgresqlDatabaseCandidates(user string) []string {
	if user == "" || user == "postgres" {
		return []string{"postgres"}
	}
	return []string{"postgres", user}
}

// postgresqlDSNValue DSN 值只含安全字符就直接拼；含空白/引号等则单引号包裹（内部引号双写），
// 否则口令或库名里的空格会把 keyword=value 连接串拆坏。
func postgresqlDSNValue(s string) string {
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

// execPostgreSQL 执行 PostgreSQL 工单：PostgreSQL 方言拆分后走共享骨架 execSQLStatements
// （连接的 search_path 已在连接参数里）。与 MySQL 的关键差异：没有 binlog /
// 前镜像回滚（PostgreSQL 侧尚未实现，回滚语句留空）。
func (e *Engine) execPostgreSQL(ctx context.Context, req *api.ExecRequest) (*api.ExecReply, error) {
	stmts, err := mysqlparse.SplitEngine(storepb.Engine_POSTGRES, req.Order.Sql)
	if err != nil || len(stmts) == 0 {
		return &api.ExecReply{Ok: false, Error: "SQL 拆分失败"}, nil
	}

	db, err := connectPostgreSQL(ctx, req.Source, req.Order.DataBase)
	if err != nil {
		return &api.ExecReply{Ok: false, Error: "连接数据库失败: " + err.Error()}, nil
	}
	defer db.Close()

	return execSQLStatements(ctx, db, stmts, req, storepb.Engine_POSTGRES, nil, nil)
}
