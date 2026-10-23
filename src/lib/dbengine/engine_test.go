package dbengine

import (
	"testing"

	"Yearning-go/src/model"
)

// TestFor 核对 db_type → 引擎方言的注册关系（与 model.DBType* 常量一一对应）。
func TestFor(t *testing.T) {
	cases := map[int]string{
		0:                     "mysql",
		model.DBTypePostgreSQL: "postgres",
		model.DBTypeMongoDB:    "mongodb",
		model.DBTypeRedis:      "redis",
	}
	for dbType, want := range cases {
		if got := For(dbType).Kind(); got != want {
			t.Errorf("For(%d).Kind() = %q, want %q", dbType, got, want)
		}
	}
	// 未注册的类型走「拒绝一切」占位，不 panic
	if For(9).Kind() != "" {
		t.Errorf("未注册类型应返回空 Kind, got %q", For(9).Kind())
	}
	if err := For(9).ValidateChange(nil, "x"); err == nil {
		t.Error("未注册类型的 ValidateChange 应报错")
	}
}

// TestMongoValidateChange 硬保底不依赖引擎在线，可直接离线验证。
func TestMongoValidateChange(t *testing.T) {
	e := For(model.DBTypeMongoDB)
	if err := e.ValidateChange(nil, `{"update":"users","updates":[{"q":{},"u":{"$set":{"a":1}}}]}`); err == nil {
		t.Error("空 filter 的 update 应被硬保底拦下")
	}
	if err := e.ValidateChange(nil, `{"update":"users","updates":[{"q":{"_id":1},"u":{"$set":{"a":2}}}]}`); err != nil {
		t.Errorf("合法 update 不应报错: %v", err)
	}
	if err := e.ValidateChange(nil, `not-json`); err == nil {
		t.Error("非法 JSON 应报错")
	}
}

// TestRedisValidateChange 硬保底同样离线可验证。
func TestRedisValidateChange(t *testing.T) {
	e := For(model.DBTypeRedis)
	if err := e.ValidateChange(nil, "SET k v"); err != nil {
		t.Errorf("SET 应通过: %v", err)
	}
	if err := e.ValidateChange(nil, "FLUSHALL"); err == nil {
		t.Error("FLUSHALL 应被硬保底拦下")
	}
}
