package dbengine

import (
	"context"
	"errors"

	"Yearning-go/src/model"
)

// rejected 未注册数据源类型的占位实现：所有能力都明确报错，
// 而不是静默返回空——让「漏注册」在调用处立刻暴露。
type rejected struct{}

func (rejected) Kind() string { return "" }

func (rejected) ListDatabases(context.Context, *model.CoreDataSource) ([]string, error) {
	return nil, errors.New("dbengine: 未注册的数据源类型")
}
func (rejected) ListCollections(context.Context, *model.CoreDataSource, string) ([]string, error) {
	return nil, errors.New("dbengine: 未注册的数据源类型")
}
func (rejected) DescribeTable(context.Context, *model.CoreDataSource, string, string) ([]Field, error) {
	return nil, errors.New("dbengine: 未注册的数据源类型")
}
func (rejected) Highlight(*model.CoreDataSource, bool, string) []map[string]string { return nil }
func (rejected) ValidateChange(*model.CoreDataSource, string) error {
	return errors.New("dbengine: 未注册的数据源类型")
}
