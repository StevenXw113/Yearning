package personal

import "Yearning-go/src/model"

// treeNodes 生成前端左树节点列表：meta/isLeaf/db_type 决定层级语义
// （Schema=库层可展开，Table=叶子；db_type 供前端决定右键生成什么语句）。
func treeNodes(names []string, meta string, isLeaf bool, dbType int) []map[string]interface{} {
	list := make([]map[string]interface{}, 0, len(names))
	for _, n := range names {
		list = append(list, map[string]interface{}{
			"title": n, "key": n, "meta": meta, "isLeaf": isLeaf, "db_type": dbType,
		})
	}
	return list
}

// FetchPostgreSQLSchemaInfo 列出 PostgreSQL 的 schema（树的「库」层）。
// PostgreSQL 没有 SHOW DATABASES，「库」映射为 schema，节点结构沿用 SQL 树。
func FetchPostgreSQLSchemaInfo(u *model.CoreDataSource) ([]map[string]interface{}, error) {
	names, err := u.PostgreSQLSchemas()
	if err != nil {
		return nil, err
	}
	return treeNodes(names, "Schema", false, model.DBTypePostgreSQL), nil
}

// FetchPostgreSQLTableInfo 列出指定 schema 下的表（树的「表」层）。
func FetchPostgreSQLTableInfo(u *model.CoreDataSource, schema string) ([]map[string]interface{}, error) {
	names, err := u.PostgreSQLTables(schema)
	if err != nil {
		return nil, err
	}
	return treeNodes(names, "Table", true, model.DBTypePostgreSQL), nil
}
