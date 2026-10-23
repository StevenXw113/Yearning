package dbengine

import (
	"context"
	"strings"

	"Yearning-go/src/model"
)

func init() { register(model.DBTypeDoris, dorisEngine{}) }

type dorisEngine struct{}

func (dorisEngine) Kind() string { return "doris" }

func (dorisEngine) ListDatabases(_ context.Context, src *model.CoreDataSource) ([]string, error) {
	db, err := sqlConnect(src)
	if err != nil {
		return nil, err
	}
	defer model.Close(db)
	// Doris 的系统库与 MySQL 不同：只有 information_schema / __internal_schema
	return queryStrings(db, `SELECT SCHEMA_NAME FROM information_schema.schemata
		WHERE SCHEMA_NAME NOT IN ('information_schema','__internal_schema')
		ORDER BY SCHEMA_NAME`)
}

func (dorisEngine) ListCollections(_ context.Context, src *model.CoreDataSource, dbName string) ([]string, error) {
	db, err := sqlConnect(src)
	if err != nil {
		return nil, err
	}
	defer model.Close(db)
	return queryStrings(db, `SELECT table_name FROM information_schema.tables
		WHERE table_schema = ? AND table_type = 'BASE TABLE' ORDER BY table_name`, dbName)
}

func (dorisEngine) DescribeTable(_ context.Context, src *model.CoreDataSource, dbName, table string) ([]Field, error) {
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

// Highlight Doris 补全：Doris 兼容 information_schema，逻辑同 MySQL，仅系统库不同。
func (dorisEngine) Highlight(src *model.CoreDataSource, isField bool, dbName string) []map[string]string {
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
	q, args := dorisSchemataQuery(src)
	if ss, err := queryStrings(db, q, args...); err == nil {
		for _, s := range ss {
			list = append(list, map[string]string{"vl": s, "meta": "Schema"})
		}
	}
	return list
}

// ValidateChange SQL 不做提交硬保底：解析与审核都由引擎完成。
func (dorisEngine) ValidateChange(_ *model.CoreDataSource, _ string) error { return nil }

// dorisSchemataQuery 构造 Doris 库列表查询：系统库只有 information_schema / __internal_schema。
func dorisSchemataQuery(src *model.CoreDataSource) (string, []interface{}) {
	excl := []string{"information_schema", "__internal_schema"}
	for _, v := range strings.Split(src.ExcludeDbList, ",") {
		if v = strings.TrimSpace(v); v != "" {
			excl = append(excl, v)
		}
	}
	q := `SELECT SCHEMA_NAME FROM information_schema.schemata WHERE SCHEMA_NAME NOT IN (` + placeholders(len(excl)) + `) ORDER BY SCHEMA_NAME`
	args := make([]interface{}, len(excl))
	for i, v := range excl {
		args[i] = v
	}
	return q, args
}
