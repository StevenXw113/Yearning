package permission

import "testing"

func TestUnique(t *testing.T) {
	got := unique([]string{"a", "b", "a", "c", "b"})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("unique = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unique = %v, want %v（需保序）", got, want)
		}
	}
}

func TestContains(t *testing.T) {
	list := []string{"s1", "s2"}
	if !contains(list, "s2") {
		t.Error("contains 应命中 s2")
	}
	if contains(list, "s3") {
		t.Error("contains 不该命中 s3")
	}
	if contains(nil, "s1") {
		t.Error("空列表不该命中")
	}
}
