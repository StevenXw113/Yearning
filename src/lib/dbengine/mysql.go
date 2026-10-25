package dbengine

import (
	"context"
	"strings"

	"Yearning-go/src/lib/enc"
	"Yearning-go/src/model"

	"gorm.io/gorm"
)

func init() {
	register(0, sqlEngine{
		kind:       "mysql",
		sysSchemas: []string{"information_schema", "mysql", "performance_schema", "sys"},
	})
}

// sqlEngine 是走 information_schema 的 MySQL 系方言引擎（MySQL / Doris），
// 差异只有 kind 与系统库列表。
type sqlEngine struct {
	kind       string
	sysSchemas []string
}

func (e sqlEngine) Kind() string { return e.kind }

func (e sqlEngine) ListDatabases(_ context.Context, src *model.CoreDataSource) ([]string, error) {
	// TODO(batch3): 迁移 FetchBase 时带上 src.ExcludeDbList 的过滤（现有行为）
	db, err := sqlConnect(src)
	if err != nil {
		return nil, err
	}
	defer model.Close(db)
	return queryStrings(db, `SELECT SCHEMA_NAME FROM information_schema.schemata
		WHERE SCHEMA_NAME NOT IN (`+placeholders(len(e.sysSchemas))+`)
		ORDER BY SCHEMA_NAME`, toAny(e.sysSchemas)...)
}

func (e sqlEngine) ListCollections(_ context.Context, src *model.CoreDataSource, dbName string) ([]string, error) {
	db, err := sqlConnect(src)
	if err != nil {
		return nil, err
	}
	defer model.Close(db)
	return queryStrings(db, `SELECT table_name FROM information_schema.tables
		WHERE table_schema = ? AND table_type = 'BASE TABLE' ORDER BY table_name`, dbName)
}

func (e sqlEngine) DescribeTable(_ context.Context, src *model.CoreDataSource, dbName, table string) ([]Field, error) {
	db, err := sqlConnect(src)
	if err != nil {
		return nil, err
	}
	defer model.Close(db)
	rows, err := db.Raw(`SELECT column_name, column_type FROM information_schema.columns
		WHERE table_schema = ? AND table_name = ? ORDER BY ordinal_position`, dbName, table).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Field
	for rows.Next() {
		var f Field
		if err := rows.Scan(&f.Name, &f.Type); err == nil {
			out = append(out, f)
		}
	}
	return out, nil
}

// Highlight SQL 补全：isField 时给当前库的表 + 字段，否则给库列表（排除 ExcludeDbList）。
func (e sqlEngine) Highlight(src *model.CoreDataSource, isField bool, dbName string) []map[string]string {
	db, err := sqlConnect(src)
	if err != nil {
		return nil
	}
	defer model.Close(db)
	var list []map[string]string
	if isField {
		if ts, err := queryStrings(db, `SELECT table_name FROM information_schema.tables WHERE table_schema = ? ORDER BY table_name`, dbName); err == nil {
			for _, t := range ts {
				list = append(list, map[string]string{"vl": t, "meta": "Table"})
			}
		}
		if fs, err := queryStrings(db, `SELECT column_name FROM information_schema.columns WHERE table_schema = ? GROUP BY column_name ORDER BY column_name`, dbName); err == nil {
			for _, f := range fs {
				list = append(list, map[string]string{"vl": f, "meta": "Fields"})
			}
		}
		return list
	}
	q, args := schemataQuery(src, e.sysSchemas)
	if ss, err := queryStrings(db, q, args...); err == nil {
		for _, s := range ss {
			list = append(list, map[string]string{"vl": s, "meta": "Schema"})
		}
	}
	return list
}

// ValidateChange SQL 不做提交硬保底：解析与审核都由引擎完成。
func (e sqlEngine) ValidateChange(_ *model.CoreDataSource, _ string) error { return nil }

// sqlConnect 建一条元数据查询用的连接（mysql 驱动）。
func sqlConnect(src *model.CoreDataSource) (*gorm.DB, error) {
	return model.NewDBSub(model.DSN{
		Username: src.Username,
		Password: enc.Decrypt(model.C.General.SecretKey, src.Password),
		Host:     src.IP,
		Port:     src.Port,
		CA:       src.CAFile,
		Cert:     src.Cert,
		Key:      src.KeyFile,
	})
}

// queryStrings 执行一条只读查询，把首列扫进 []string。
func queryStrings(db *gorm.DB, sql string, args ...interface{}) ([]string, error) {
	rows, err := db.Raw(sql, args...).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err == nil {
			out = append(out, s)
		}
	}
	return out, nil
}

// schemataQuery 构造库列表查询，拼进 ExcludeDbList 与系统库的 NOT IN。
func schemataQuery(src *model.CoreDataSource, sys []string) (string, []interface{}) {
	excl := append([]string{}, sys...)
	for _, v := range strings.Split(src.ExcludeDbList, ",") {
		if v = strings.TrimSpace(v); v != "" {
			excl = append(excl, v)
		}
	}
	q := `SELECT SCHEMA_NAME FROM information_schema.schemata WHERE SCHEMA_NAME NOT IN (` + placeholders(len(excl)) + `) ORDER BY SCHEMA_NAME`
	return q, toAny(excl)
}

// placeholders 生成 n 个 `?`，用逗号连接（给 NOT IN 用）。
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func toAny(ss []string) []interface{} {
	out := make([]interface{}, len(ss))
	for i, v := range ss {
		out[i] = v
	}
	return out
}
