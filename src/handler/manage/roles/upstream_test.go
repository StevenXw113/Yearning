package roles

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestUpstreamTagParse(t *testing.T) {
	ok := map[string]string{
		"/bytebase/bytebase/releases/tag/3.22.0":  "3.22.0",
		"/bytebase/bytebase/releases/tag/v3.23.1": "v3.23.1",
	}
	for path, want := range ok {
		m := reReleaseTag.FindStringSubmatch(path)
		if m == nil || m[1] != want {
			t.Fatalf("%s 解析为 %v，期望 %s", path, m, want)
		}
	}
	if reReleaseTag.MatchString("/bytebase/bytebase/releases") {
		t.Fatal("非 tag 路径不应匹配")
	}
}

// bytebaseRef 与 engine/internal/bytebase/UPSTREAM 漂移会让页面结论失真，这里守住。
func TestBytebaseRefMatchesUpstream(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "engine", "internal", "bytebase", "UPSTREAM"))
	if err != nil {
		t.Fatalf("读取 UPSTREAM 失败: %v", err)
	}
	m := regexp.MustCompile(`(?m)^ref:\s*(\S+)\s*$`).FindSubmatch(b)
	if m == nil {
		t.Fatal("UPSTREAM 中未找到 ref")
	}
	if got := string(m[1]); got != bytebaseRef {
		t.Fatalf("upstream.go 的 bytebaseRef=%s 与 UPSTREAM 的 ref=%s 不一致", bytebaseRef, got)
	}
}
