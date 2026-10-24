package dbengine

import (
	"context"

	"Yearning-go/src/model"
)

func init() { register(model.DBTypePostgreSQL, postgresqlEngine{}) }

type postgresqlEngine struct{}

func (postgresqlEngine) Kind() string { return "postgres" }

func (postgresqlEngine) ListDatabases(_ context.Context, src *model.CoreDataSource) ([]string, error) {
	// PostgreSQL 的「库」是 schema，直接列出来（PostgreSQLSchemas 已排除 pg_*/information_schema）
	return src.PostgreSQLSchemas()
}

func (postgresqlEngine) ListCollections(_ context.Context, src *model.CoreDataSource, db string) ([]string, error) {
	return src.PostgreSQLTables(db)
}

func (postgresqlEngine) DescribeTable(_ context.Context, src *model.CoreDataSource, db, table string) ([]Field, error) {
	names, err := src.PostgreSQLTableColumns(db, table)
	if err != nil {
		return nil, err
	}
	out := make([]Field, 0, len(names))
	for _, n := range names {
		out = append(out, Field{Name: n})
	}
	return out, nil
}

// Highlight PostgreSQL 暂不补全（与拆分前一致：mysql 的 information_schema 语句不适用于 PostgreSQL）。
func (postgresqlEngine) Highlight(_ *model.CoreDataSource, _ bool, _ string) []map[string]string {
	return nil
}

func (postgresqlEngine) ValidateChange(_ *model.CoreDataSource, _ string) error { return nil }
