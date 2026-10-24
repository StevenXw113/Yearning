package model

// DBTypePostgreSQL 数据源类型：0 mysql（默认）、1 PostgreSQL、2 mongodb、3 redis。
// PostgreSQL 与 mysql 同是 SQL 方言：查询、工单、元数据都复用 MySQL 链路，
// 只在驱动 / DSN / 方言专属 SQL 处按 DBType 分叉。
const DBTypePostgreSQL = 1

// PostgreSQL 的「库」映射为 schema（namespace），而非 database：
// database 固定落在 postgres（PostgreSQL 默认存在的库），schema 用 SET search_path 轻量切换，
// 这与 MySQL 的「一次连接、use 切库」同构，复用同一条查询链路。
const PostgreSQLDefaultDatabase = "postgres"

// PostgreSQLSchemas 列出 PostgreSQL 的 schema（作为 Yearning 树里的「库」）。
// 排除系统 schema（pg_*）与 information_schema。
func (s *CoreDataSource) PostgreSQLSchemas() ([]string, error) {
	db, err := s.ConnectDB(PostgreSQLDefaultDatabase)
	if err != nil {
		return nil, err
	}
	defer func() { _ = Close(db) }()
	rows, err := db.Raw("SELECT schema_name FROM information_schema.schemata WHERE schema_name NOT LIKE 'pg_%' AND schema_name <> 'information_schema' ORDER BY schema_name").Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		out = append(out, name)
	}
	return out, nil
}

// PostgreSQLTables 列出指定 schema 下的表。
func (s *CoreDataSource) PostgreSQLTables(schema string) ([]string, error) {
	db, err := s.ConnectDB(PostgreSQLDefaultDatabase)
	if err != nil {
		return nil, err
	}
	defer func() { _ = Close(db) }()
	rows, err := db.Raw("SELECT table_name FROM information_schema.tables WHERE table_schema = ? AND table_type = 'BASE TABLE' ORDER BY table_name", schema).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		out = append(out, name)
	}
	return out, nil
}

// PostgreSQLTableColumns 列出指定 schema 下某张表的字段名（按 ordinal_position），
// 供 AI 建议感知数据结构（只取字段名、不取值）。
func (s *CoreDataSource) PostgreSQLTableColumns(schema, table string) ([]string, error) {
	db, err := s.ConnectDB(PostgreSQLDefaultDatabase)
	if err != nil {
		return nil, err
	}
	defer func() { _ = Close(db) }()
	rows, err := db.Raw("SELECT column_name FROM information_schema.columns WHERE table_schema = ? AND table_name = ? ORDER BY ordinal_position", schema, table).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		out = append(out, name)
	}
	return out, nil
}
