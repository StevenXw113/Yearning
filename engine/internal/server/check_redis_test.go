package server

import (
	"testing"

	"engine/api"
)

// 检测阶段没有执行，影响行数按命令字面量里的键数给（前端「影响行数」列不再恒为 0）。
func TestCheckRedisAffectRows(t *testing.T) {
	cases := map[string]uint32{
		"SET app:k v":       1,
		"DEL app:k1 app:k2": 2,
		"MSET k1 v1 k2 v2":  2,
		"FLUSHALL":          0,
	}
	for sql, want := range cases {
		rep := checkRedis(&api.CheckRequest{Sql: sql, Schema: "db0", Source: &api.DataSource{Kind: "redis"}})
		if len(rep.Records) != 1 {
			t.Fatalf("%q: 期望 1 条记录, got %d", sql, len(rep.Records))
		}
		if got := rep.Records[0].AffectRows; got != want {
			t.Errorf("%q 的 affect_rows = %d, want %d", sql, got, want)
		}
	}
}
