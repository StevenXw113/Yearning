package main

import (
	"encoding/json"
	"testing"
)

// 权限是按 source_id 授权的：改写列表必须精确命中旧 ID，且不能碰别的数据源或别的键。
func TestRewritePermission(t *testing.T) {
	oldID := "a3c8542f-e229-4128-8b78-61fe6b349412"
	newID := "Rr6e"
	// 故意混入另一个数据源的 ID 与非 *_source 键，验证不会被误改
	perm := `{"ddl_source":["` + oldID + `","5gHt"],"dml_source":["` + oldID + `"],"query_source":[],"auditor":["admin"]}`

	out, changed := rewritePermission(perm, oldID, newID)
	if !changed {
		t.Fatal("应当发生改写")
	}
	var got map[string][]string
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("改写结果不是合法 JSON: %v (%s)", err, out)
	}
	if len(got["ddl_source"]) != 2 || got["ddl_source"][0] != newID || got["ddl_source"][1] != "5gHt" {
		t.Errorf("ddl_source 改写不正确: %v", got["ddl_source"])
	}
	if len(got["dml_source"]) != 1 || got["dml_source"][0] != newID {
		t.Errorf("dml_source 改写不正确: %v", got["dml_source"])
	}
	if len(got["query_source"]) != 0 {
		t.Errorf("空列表不该被改动: %v", got["query_source"])
	}

	// 不含旧 ID 时应判定为未改动（幂等：重复执行不会写库）
	if _, changed := rewritePermission(`{"dml_source":["5gHt"]}`, oldID, newID); changed {
		t.Error("不含旧 ID 时不应判定为改动")
	}
	// 非法 JSON 不能 panic，也不能误判为改动
	if _, changed := rewritePermission("not-json", oldID, newID); changed {
		t.Error("非法 JSON 不应判定为改动")
	}
}

func TestIsUUID(t *testing.T) {
	cases := map[string]bool{
		"a3c8542f-e229-4128-8b78-61fe6b349412": true,
		"Rr6e":                                 false,
		"":                                     false,
		"12345678-1234-1234-1234-123456789012": true,
		"rR6e-1234":                            false,
	}
	for in, want := range cases {
		if got := isUUID(in); got != want {
			t.Errorf("isUUID(%q) = %v，期望 %v", in, got, want)
		}
	}
}
