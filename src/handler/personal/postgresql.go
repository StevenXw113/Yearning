package personal

import "Yearning-go/src/model"

// FetchPostgreSQLSchemaInfo 列出 PostgreSQL 的 schema（树的「库」层）。
// PostgreSQL 没有 SHOW DATABASES，「库」映射为 schema，节点结构沿用 SQL 树。
func FetchPostgreSQLSchemaInfo(u *model.CoreDataSource) ([]map[string]interface{}, error) {
	names, err := u.PostgreSQLSchemas()
	if err != nil {
		return nil, err
	}
	list := make([]map[string]interface{}, 0, len(names))
	for _, n := range names {
		list = append(list, map[string]interface{}{
			"title": n, "key": n, "meta": "Schema", "isLeaf": false, "db_type": model.DBTypePostgreSQL,
		})
	}
	return list, nil
}

// FetchPostgreSQLTableInfo 列出指定 schema 下的表（树的「表」层）。
func FetchPostgreSQLTableInfo(u *model.CoreDataSource, schema string) ([]map[string]interface{}, error) {
	names, err := u.PostgreSQLTables(schema)
	if err != nil {
		return nil, err
	}
	list := make([]map[string]interface{}, 0, len(names))
	for _, n := range names {
		list = append(list, map[string]interface{}{
			"title": n, "key": n, "meta": "Table", "isLeaf": true, "db_type": model.DBTypePostgreSQL,
		})
	}
	return list, nil
}
