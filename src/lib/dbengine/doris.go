package dbengine

import (
	"Yearning-go/src/model"
)

// Doris 兼容 information_schema,与 MySQL 共用 sqlEngine,仅系统库不同：
// information_schema / __internal_schema。
func init() {
	register(model.DBTypeDoris, sqlEngine{
		kind:       "doris",
		sysSchemas: []string{"information_schema", "__internal_schema"},
	})
}
