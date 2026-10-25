package permission

import (
	"sync"
	"testing"

	"Yearning-go/src/model"
)

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


// TestIsSuperUser 超管判定：内置 admin 恒为超管；[General].SuperUsers 追加
// 名单（逗号分隔、容忍空白）；普通用户不是。lazy 解析依赖 model.C，
// 测试里直接写配置再触发解析。
func TestIsSuperUser(t *testing.T) {
	model.C.General.SuperUsers = " alice , bob ,"
	superUsersOnce = freshOnce() // 重置惰性解析（其他测试可能已触发）
	if !IsSuperUser("admin") {
		t.Error("内置 admin 必须恒为超管")
	}
	if !IsSuperUser("alice") || !IsSuperUser("bob") {
		t.Error("SuperUsers 配置名单应生效")
	}
	if IsSuperUser("eve") {
		t.Error("名单外用户不应是超管")
	}
}

// freshOnce 构造未触发的 sync.Once（测试隔离用）。
func freshOnce() (o sync.Once) { return }
