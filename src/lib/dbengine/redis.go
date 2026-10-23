package dbengine

import (
	"context"

	"Yearning-go/src/lib/rediscmd"
	"Yearning-go/src/model"
)

func init() { register(model.DBTypeRedis, redisEngine{}) }

type redisEngine struct{}

func (redisEngine) Kind() string { return "redis" }

func (redisEngine) ListDatabases(_ context.Context, src *model.CoreDataSource) ([]string, error) {
	return src.RedisDatabases()
}

func (redisEngine) ListCollections(_ context.Context, src *model.CoreDataSource, db string) ([]string, error) {
	// Redis 的「表」就是键；按默认 pattern 抽样（上限由 model.RedisScanLimit 兜底）。
	return src.RedisScanKeys(model.RedisParseDB(db), "", 0)
}

func (redisEngine) DescribeTable(_ context.Context, _ *model.CoreDataSource, _, _ string) ([]Field, error) {
	// Redis 键没有 schema，返回空。
	return nil, nil
}

func (redisEngine) Highlight(_ *model.CoreDataSource, _ bool, _ string) []map[string]string {
	// Redis 没有库/表/字段语义，暂无补全。
	return nil
}

func (redisEngine) ValidateChange(_ *model.CoreDataSource, sql string) error {
	_, _, err := rediscmd.Validate(sql)
	return err
}
