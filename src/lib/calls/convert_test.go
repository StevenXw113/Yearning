package calls

import (
	"testing"

	"Yearning-go/src/model"
)

// TestDataSourceKind 四值映射决定引擎按哪种方言审核：映射错了不会报错，
// 只会让 PG 被当 MySQL 审、Mongo 被当 SQL 拆——只能靠测试拦住。
//
// 编号同时是前端「数据源类型」选项的取值（front/src/views/manager/db/dbForm.vue），
// 两边漂移会静默地把命令发到错误的审核链路。
func TestDataSourceKind(t *testing.T) {
	cases := map[int]string{
		0: "mysql",
		1: "postgres",
		2: "mongodb",
		3: "redis",
		9: "", // 未知类型由引擎拒绝
	}
	for in, want := range cases {
		if got := DataSourceKind(in); got != want {
			t.Errorf("DataSourceKind(%d) = %q, want %q", in, got, want)
		}
	}

	// 常量编号是跨层契约（前端 db_type、数据库列、引擎 kind 都由它派生）
	if model.DBTypePostgreSQL != 1 || model.DBTypeMongoDB != 2 || model.DBTypeRedis != 3 {
		t.Fatalf("数据源类型常量漂移: pg=%d mongo=%d redis=%d",
			model.DBTypePostgreSQL, model.DBTypeMongoDB, model.DBTypeRedis)
	}
}
