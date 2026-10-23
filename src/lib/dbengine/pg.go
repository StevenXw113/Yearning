package dbengine

import (
	"context"

	"Yearning-go/src/model"
)

func init() { register(model.DBTypePostgreSQL, pgEngine{}) }

type pgEngine struct{}

func (pgEngine) Kind() string { return "postgres" }

func (pgEngine) ListDatabases(_ context.Context, src *model.CoreDataSource) ([]string, error) {
	// pg 的「库」是 schema，直接列出来（PgSchemas 已排除 pg_*/information_schema）
	return src.PgSchemas()
}

func (pgEngine) ListCollections(_ context.Context, src *model.CoreDataSource, db string) ([]string, error) {
	return src.PgTables(db)
}

func (pgEngine) DescribeTable(_ context.Context, src *model.CoreDataSource, db, table string) ([]Field, error) {
	names, err := src.PgTableColumns(db, table)
	if err != nil {
		return nil, err
	}
	out := make([]Field, 0, len(names))
	for _, n := range names {
		out = append(out, Field{Name: n})
	}
	return out, nil
}

// Highlight pg 暂不补全（与拆分前一致：mysql 的 information_schema 语句不适用于 pg）。
func (pgEngine) Highlight(_ *model.CoreDataSource, _ bool, _ string) []map[string]string {
	return nil
}

func (pgEngine) ValidateChange(_ *model.CoreDataSource, _ string) error { return nil }
