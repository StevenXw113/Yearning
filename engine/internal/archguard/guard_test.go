package archguard

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// bytebasePkgPrefix 是内化代码的 import 前缀。
const bytebasePkgPrefix = "engine/internal/bytebase"

// allowedImporters 是允许直接依赖内化 Bytebase 代码的包（适配层）。
// 其余任何包都不得直接 import，以把上游 API 漂移的影响面钉死在这里。
var allowedImporters = []string{
	"internal/mysqlparse/",
	"internal/server/",
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("向上未找到 go.mod")
		}
		dir = parent
	}
}

func goFiles(t *testing.T, root, skipPrefix string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		base := filepath.Base(path)
		if strings.HasPrefix(base, "._") {
			// macOS 在非原生文件系统上产生的 AppleDouble 元数据。
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			if base == ".git" || base == "vendor" || base == "node_modules" {
				return filepath.SkipDir
			}
			if skipPrefix != "" && strings.HasPrefix(filepath.ToSlash(path), filepath.ToSlash(skipPrefix)) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("遍历 %s 失败: %v", root, err)
	}
	sort.Strings(out)
	return out
}

func importsOf(t *testing.T, path string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("解析 %s 失败: %v", path, err)
	}
	var out []string
	for _, im := range f.Imports {
		p, err := strconv.Unquote(im.Path.Value)
		if err != nil {
			continue
		}
		out = append(out, p)
	}
	return out
}

// TestBytebaseCouplingBoundary 保证只有适配层能 import 内化代码。
// 违反时说明有新的调用点绕过了适配层，上游一变就会引发更大范围的重构。
func TestBytebaseCouplingBoundary(t *testing.T) {
	root := moduleRoot(t)
	var violations []string
	for _, path := range goFiles(t, root, "") {
		rel := filepath.ToSlash(mustRel(t, root, path))
		if strings.HasPrefix(rel, "internal/bytebase/") {
			continue // 内化代码自身
		}
		for _, imp := range importsOf(t, path) {
			if !strings.HasPrefix(imp, bytebasePkgPrefix) {
				continue
			}
			allowed := false
			for _, p := range allowedImporters {
				if strings.HasPrefix(rel, p) {
					allowed = true
					break
				}
			}
			if !allowed {
				violations = append(violations, rel+" -> "+imp)
			}
		}
	}
	if len(violations) > 0 {
		t.Errorf("以下文件直接依赖内化 Bytebase 代码，请改为通过适配层（%v）调用：\n  %s",
			allowedImporters, strings.Join(violations, "\n  "))
	}
}

func mustRel(t *testing.T, base, target string) string {
	t.Helper()
	rel, err := filepath.Rel(base, target)
	if err != nil {
		t.Fatalf("计算相对路径失败: %v", err)
	}
	return rel
}

var (
	enumBlockRe = regexp.MustCompile(`(?s)SQLReviewRule_Type_name = map\[int32\]string\{(.*?)\n\}`)
	enumNameRe  = regexp.MustCompile(`"([A-Z][A-Z0-9_]*)"`)
	registerRe  = regexp.MustCompile(`advisor\.Register\(\s*storepb\.Engine_MYSQL\s*,\s*storepb\.SQLReviewRule_([A-Z][A-Z0-9_]*)\b`)
	// \b 用于排除 SQLReviewRule_Type / SQLReviewRule_NumberPayload 这类
	// 非规则类型标识（否则会误捕获首字母 T / N）。
	usedTypeRe = regexp.MustCompile(`storepb\.SQLReviewRule_([A-Z][A-Z0-9_]*)\b`)
)

// levelAndSentinelNames 是 SQLReviewRule_ 前缀下但**不是**规则类型的标识
// （Level 取值与哨兵值），静态扫描时需排除。
var levelAndSentinelNames = map[string]bool{
	"ERROR":             true,
	"WARNING":           true,
	"TYPE_UNSPECIFIED":  true,
	"LEVEL_UNSPECIFIED": true,
}

// TestBytebaseRuleMappingConsistent 静态校验 rules.go 用到的规则类型
// 都能在枚举与 MySQL 注册表中找到，避免运行期才报 "unknown advisor"。
func TestBytebaseRuleMappingConsistent(t *testing.T) {
	root := moduleRoot(t)

	// 1. 枚举全集
	enumSrc, err := os.ReadFile(filepath.Join(root, "internal/bytebase/generated-go/store/review_config.pb.go"))
	if err != nil {
		t.Fatalf("读取枚举定义失败: %v", err)
	}
	m := enumBlockRe.FindSubmatch(enumSrc)
	if m == nil {
		t.Fatal("未在 review_config.pb.go 中找到 SQLReviewRule_Type_name 枚举")
	}
	enumNames := map[string]bool{}
	for _, n := range enumNameRe.FindAllStringSubmatch(string(m[1]), -1) {
		enumNames[n[1]] = true
	}
	if len(enumNames) == 0 {
		t.Fatal("枚举解析结果为空，review_config.pb.go 结构可能已变化")
	}

	// 2. MySQL 侧已注册的规则类型
	registered := map[string]bool{}
	mysqlDir := filepath.Join(root, "internal/bytebase/plugin/advisor/mysql")
	for _, path := range goFiles(t, mysqlDir, "") {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", path, err)
		}
		for _, mm := range registerRe.FindAllStringSubmatch(string(src), -1) {
			registered[mm[1]] = true
		}
	}
	if len(registered) == 0 {
		t.Fatal("未扫描到任何 MySQL 规则注册，注册写法可能已变化")
	}

	// 3. rules.go 实际使用的规则类型
	rulesSrc, err := os.ReadFile(filepath.Join(root, "internal/server/rules.go"))
	if err != nil {
		t.Fatalf("读取 rules.go 失败: %v", err)
	}
	used := map[string]bool{}
	for _, mm := range usedTypeRe.FindAllStringSubmatch(string(rulesSrc), -1) {
		if levelAndSentinelNames[mm[1]] {
			continue
		}
		used[mm[1]] = true
	}
	if len(used) == 0 {
		t.Fatal("未在 rules.go 中解析到任何规则类型引用")
	}

	var notInEnum, notRegistered []string
	for n := range used {
		if !enumNames[n] {
			notInEnum = append(notInEnum, n)
		}
		if !registered[n] {
			notRegistered = append(notRegistered, n)
		}
	}
	sort.Strings(notInEnum)
	sort.Strings(notRegistered)
	if len(notInEnum) > 0 {
		t.Errorf("rules.go 引用了枚举中不存在的规则类型（上游可能已改名/删除）: %v", notInEnum)
	}
	if len(notRegistered) > 0 {
		t.Errorf("rules.go 引用了 MySQL 未注册的规则类型（运行期会报 unknown advisor）: %v", notRegistered)
	}
	t.Logf("枚举 %d 条，MySQL 已注册 %d 条，rules.go 使用 %d 条，校验通过",
		len(enumNames), len(registered), len(used))
}

// TestSyncDoesNotTouchCustomRules 保证同步只重建 internal/bytebase，
// 不会触碰自研规则目录 internal/customrules——否则同步会冲掉自定义规则。
func TestSyncDoesNotTouchCustomRules(t *testing.T) {
	root := moduleRoot(t)
	for _, rel := range []string{
		"scripts/sync-bytebase.sh",
		"scripts/trim-bytebase.py",
	} {
		src, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", rel, err)
		}
		if strings.Contains(string(src), "customrules") {
			t.Errorf("%s 引用了 customrules：同步可能覆盖自研规则目录", rel)
		}
	}
}
