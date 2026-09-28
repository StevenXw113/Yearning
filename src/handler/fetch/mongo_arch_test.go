package fetch

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// Mongo 的索引清单要摊成与 MySQL SHOW INDEX 一样的「一个索引键一行」，
// 且表结构/索引两处的键名必须落在 common.FieldInfo / common.IndexInfo 上。
func TestMongoIndexRows(t *testing.T) {
	// _id 索引：listIndexes 不返回 unique 字段，但它天然唯一
	got := mongoIndexRows("users", "_id_", false, bson.D{{Key: "_id", Value: int32(1)}})
	if len(got) != 1 {
		t.Fatalf("_id 索引应摊成 1 行，实际 %d", len(got))
	}
	r := got[0]
	if r.Table != "users" || r.IndexName != "_id_" || r.ColumnName != "_id" || r.Seq != 1 {
		t.Errorf("_id 索引字段不对: %+v", r)
	}
	if r.NonUnique != 0 {
		t.Errorf("_id 索引应唯一（NonUnique=0），实际 %d", r.NonUnique)
	}

	// 复合唯一索引：按 key 顺序摊成多行，Seq 从 1 递增
	got = mongoIndexRows("users", "email_name", true, bson.D{
		{Key: "email", Value: int32(1)},
		{Key: "name", Value: int32(-1)},
	})
	if len(got) != 2 {
		t.Fatalf("复合索引应摊成 2 行，实际 %d", len(got))
	}
	if got[0].Seq != 1 || got[0].ColumnName != "email" || got[1].Seq != 2 || got[1].ColumnName != "name" {
		t.Errorf("复合索引顺序不对: %+v", got)
	}
	if got[1].NonUnique != 0 {
		t.Errorf("unique 索引两行都该 NonUnique=0，实际 %d", got[1].NonUnique)
	}

	// 非唯一索引：多行编号靠 index 前缀与 MySQL 保持一致（无前缀 + Seq 区分）
	got = mongoIndexRows("users", "name_1", false, bson.D{{Key: "name", Value: int32(1)}})
	if got[0].NonUnique != 1 {
		t.Errorf("非唯一索引应 NonUnique=1，实际 %d", got[0].NonUnique)
	}
}

func TestMongoIndexType(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{int32(1), "BTREE"},
		{int32(-1), "BTREE"},
		{int64(-1), "BTREE"},
		{float64(1), "BTREE"},
		{"text", "TEXT"},
		{"2dsphere", "GEO"},
		{"2d", "GEO"},
		{"hashed", "HASHED"},
	}
	for _, c := range cases {
		if got := mongoIndexType(c.in); got != c.want {
			t.Errorf("mongoIndexType(%#v) = %q，期望 %q", c.in, got, c.want)
		}
	}
}
