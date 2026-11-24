package model

import "testing"

func TestProductionAuditRequired(t *testing.T) {
	old := GloOther.Swap(&Other{ProductionAuditTags: []string{"prod", "staging"}})
	defer GloOther.Store(old)

	cases := []struct {
		name   string
		source *CoreDataSource
		want   bool
	}{
		{"nil 不触发", nil, false},
		{"空标签兼容 IsProduction=true", &CoreDataSource{IsProduction: true}, true},
		{"空标签兼容 IsProduction=false", &CoreDataSource{IsProduction: false}, false},
		{"命中自定义标签（大小写与空白容错）", &CoreDataSource{EnvironmentTag: " Prod "}, true},
		{"未命中自定义标签", &CoreDataSource{EnvironmentTag: "test"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ProductionAuditRequired(c.source); got != c.want {
				t.Fatalf("ProductionAuditRequired() = %v, want %v", got, c.want)
			}
		})
	}
}
