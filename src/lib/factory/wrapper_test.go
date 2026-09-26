package factory

import (
	"strings"
	"testing"
)

func TestArrayRemove(t *testing.T) {
	out, err := ArrayRemove([]byte(`["team-a","team-b","team-a"]`), "team-a")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `["team-b"]` {
		t.Errorf(`ArrayRemove = %s, want ["team-b"]`, out)
	}

	// 全部命中时要给空数组而不是 null：调用方（权限/用户组）按数组继续解析
	out, err = ArrayRemove([]byte(`["team-a"]`), "team-a")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `[]` {
		t.Errorf("ArrayRemove 全命中 = %s, want []", out)
	}
}

func TestMultiArrayRemove(t *testing.T) {
	out, err := MultiArrayRemove(
		[]byte(`{"ddl_source":["a","b"],"dml_source":["a"],"query_source":["c"]}`),
		[]string{"ddl_source", "dml_source", "query_source"}, "a")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `"ddl_source":["b"]`) ||
		!strings.Contains(s, `"dml_source":[]`) ||
		!strings.Contains(s, `"query_source":["c"]`) {
		t.Errorf("MultiArrayRemove = %s", s)
	}
}
