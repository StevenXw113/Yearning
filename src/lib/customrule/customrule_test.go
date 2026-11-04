package customrule

import (
	"testing"

	"Yearning-go/src/model"
)

func TestHits(t *testing.T) {
	rules := []model.CoreCustomRule{
		{Name: "禁 DROP", Pattern: `(?i)\bdrop\s+table\b`, Level: "error"},
		{Name: "提醒 SELECT *", Pattern: `(?i)select\s+\*`, Level: "warning"},
		{Name: "非法正则", Pattern: `(`, Level: "error"},
	}

	if hits := Hits("DROP TABLE t", rules); len(hits) != 1 || hits[0].Level != levelError {
		t.Fatalf("error 级规则应命中 1 条: %+v", hits)
	}
	if hits := Hits("select * from t", rules); len(hits) != 1 || hits[0].Level != levelWarning {
		t.Fatalf("warning 级规则应命中 1 条: %+v", hits)
	}
	if hits := Hits("insert into t values(1)", rules); len(hits) != 0 {
		t.Fatalf("不该命中: %+v", hits)
	}
	if hits := Hits("", rules); hits != nil {
		t.Fatalf("空 SQL 应返回 nil: %+v", hits)
	}
}
