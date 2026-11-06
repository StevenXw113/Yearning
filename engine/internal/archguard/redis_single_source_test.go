package archguard

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestRedisCommandTablesSingleSource 只读/写命令名单只能在 engine/api 维护一份。
//
// 这三个包曾经各有一份：主程序查询页、引擎分类、审计规则。漂移过一次
// （SINTERCARD 在查询页放行、在引擎被判成变更命令），所以用源码级护栏钉死。
func TestRedisCommandTablesSingleSource(t *testing.T) {
	root := moduleRoot(t)
	allowed := filepath.Join(root, "api")

	var files []string
	files = append(files, goFiles(t, root, "internal/bytebase")...)
	files = append(files, goFiles(t, filepath.Join(root, "..", "src", "lib", "rediscmd"), "")...)

	// 只认「变量声明」形态的 map 字面量，避免匹配到 overwriteCommands 这类同后缀名字
	banned := regexp.MustCompile(`(?m)^\s*(read|write)Commands\s*=\s*map\[string\]struct\{\}\{`)
	hits := 0
	for _, f := range files {
		if strings.HasPrefix(f, allowed) || strings.HasSuffix(f, "redis_single_source_test.go") {
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("读 %s 失败: %v", f, err)
		}
		if banned.MatchString(string(raw)) {
			rel, _ := filepath.Rel(root, f)
			t.Errorf("%s 里又出现命令表：名单应只在 engine/api 维护，其余包通过 RedisSemantics 取值", rel)
			hits++
		}
	}
	if hits == 0 && len(files) < 10 {
		t.Fatalf("只扫到 %d 个文件，护栏可能没生效", len(files))
	}
}

// TestRedisSplitSingleSource 切词与抓镜像辅助实现也只留 api 一份
// （此前三个包各抄一遍，改一边就会漂移）。
func TestRedisSplitSingleSource(t *testing.T) {
	root := moduleRoot(t)
	files := append(
		goFiles(t, root, "internal/bytebase"),
		goFiles(t, filepath.Join(root, "..", "src", "lib", "rediscmd"), "")...,
	)
	cases := []struct{ sig, call string }{
		{"func Split(line string) ([]string, error) {", "api.SplitRedisCommand"},
		{"func AffectedKeys(args []string) []string {", "api.RedisAffectedKeys"},
		{"func Quote(s string) string {", "api.QuoteRedisArg"},
	}
	for _, f := range files {
		if strings.HasPrefix(f, filepath.Join(root, "api")) {
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("读 %s 失败: %v", f, err)
		}
		// 统一成 LF 再截取函数体：仓库检出是 CRLF，"\n}" 在 CRLF 下匹配不到
		src := strings.ReplaceAll(string(raw), "\r\n", "\n")
		for _, c := range cases {
			i := strings.Index(src, c.sig)
			if i < 0 {
				continue
			}
			end := strings.Index(src[i:], "\n}")
			if end < 0 {
				continue
			}
			body := src[i : i+end]
			if !strings.Contains(body, c.call) {
				rel, _ := filepath.Rel(root, f)
				t.Errorf("%s 里又写了一份 %q 的实现：应只调用 %s", rel, c.sig, c.call)
			}
		}
	}
}
