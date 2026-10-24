// Package dbengine 统一主程序侧的「数据引擎」适配器。
//
// 每个数据源类型（mysql / postgres / mongodb / redis）实现一个 Engine，把散落在
// 各 handler 里的 `switch s.DBType` 分支收敛成接口调用。规则审核**不在这里**：
// 它已经由引擎 gRPC 按 kind 统一分派，本包只管元数据 / 补全 / 提交硬保底 / 执行。
//
// 迁移分批进行，见 design.md。本包不得 import handler 包（common/fetch/audit/personal），
// 否则会让 handler 反过来 import 本包时成环。
package dbengine

import (
	"context"

	"Yearning-go/src/model"
)

// Field 一张表/集合的字段概要。命令式引擎（Mongo/Redis）没有固定的列类型，
// Type 可为空；SQL 引擎填 information_schema 里的列类型。
type Field struct {
	Name string
	Type string
}

// Index 一个索引概要。命令式引擎可能没有「唯一」概念，Unique 用零值。
type Index struct {
	Name   string
	Unique bool
}

// Engine 是一个数据引擎在主程序侧的能力集合。
type Engine interface {
	// Kind 返回引擎方言标识（与 lib/calls.DataSourceKind 一致）
	Kind() string

	// ---- 元数据 ----
	// ListDatabases 列出库（PostgreSQL 映射为 schema，redis 映射为 db0..dbN）。
	ListDatabases(ctx context.Context, src *model.CoreDataSource) ([]string, error)
	// ListCollections 列出某库下的表/集合/键。
	ListCollections(ctx context.Context, src *model.CoreDataSource, db string) ([]string, error)
	// DescribeTable 列出某表/集合的字段概要。无 schema 概念的引擎返回空。
	DescribeTable(ctx context.Context, src *model.CoreDataSource, db, name string) ([]Field, error)

	// ---- 补全 ----
	// Highlight 返回给前端 monaco 的建议：vl=插入文本，meta 仅作分组说明（Schema/Table/Fields）。
	Highlight(src *model.CoreDataSource, isField bool, db string) []map[string]string

	// ---- 变更 ----
	// ValidateChange 提交时的硬保底：不读规则集、不依赖引擎在线。
	// SQL 引擎返回 nil（静态解析由引擎完成）。
	ValidateChange(src *model.CoreDataSource, sql string) error
}
