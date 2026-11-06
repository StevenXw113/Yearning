package archguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRedisAuditFactsAtEveryCheckEntry 每个审核入口都必须通过 calls.RedisAudit 带上
// Redis 运行时事实（部署形态/生产标记/版本/键状态快照）。
//
// 这条是踩过的坑：入口漏传时规则不报错、只是静默失效——拓扑 13 条、键状态 4 条、
// 生产写与版本兼容全都变成摆设，页面上看不出来。
func TestRedisAuditFactsAtEveryCheckEntry(t *testing.T) {
	root := srcRoot(t)

	var entries int
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		src := string(raw)
		if !strings.Contains(src, "&api.CheckRequest{") {
			return nil
		}
		entries++
		if !strings.Contains(src, "calls.RedisAudit(") {
			rel, _ := filepath.Rel(root, path)
			t.Errorf("%s 构造了 CheckRequest 却没走 calls.RedisAudit：Redis 规则会静默失效", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("遍历失败: %v", err)
	}
	if entries == 0 {
		t.Fatal("没扫到任何审核入口，护栏可能失效")
	}
}

// srcRoot 向上找到 src 目录（本测试就在 src/archguard 下）
func srcRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if filepath.Base(dir) == "src" {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("向上未找到 src 目录")
		}
		dir = parent
	}
}
