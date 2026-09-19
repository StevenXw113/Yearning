package archguard

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// manifestName 是随代码入库的规则清单（engine/RULES.md）。
const manifestName = "RULES.md"

// manifestRuleRe 匹配清单里 "- `RULE_TYPE`" 形式的条目。
var manifestRuleRe = regexp.MustCompile("(?m)^- `([A-Z][A-Z0-9_]*)`")

// metadataDependent 是「需要库内元数据或执行计划才有意义」的规则：
// 引擎只做静态审核、审核阶段不连业务库，所以这些规则当前无法启用。
// 见 internal/server/rules.go 里关于 FinalMetadata 的说明。
var metadataDependent = map[string]bool{
	"STATEMENT_DML_DRY_RUN":        true, // 需要真实执行计划
	"STATEMENT_AFFECTED_ROW_LIMIT": true, // 需要按执行计划估算影响行数
	"INDEX_TOTAL_NUMBER_LIMIT":     true, // 依赖 FinalMetadata（库内索引快照）
	"COLUMN_NO_NULL":               true, // 依赖 FinalMetadata（库内列定义）
}

// scanEnumNames 返回 SQLReviewRule_Type 枚举的全部名字。
func scanEnumNames(t *testing.T, root string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(root, "internal/bytebase/generated-go/store/review_config.pb.go"))
	if err != nil {
		t.Fatalf("读取枚举定义失败: %v", err)
	}
	m := enumBlockRe.FindSubmatch(src)
	if m == nil {
		t.Fatal("未在 review_config.pb.go 中找到 SQLReviewRule_Type_name 枚举")
	}
	out := map[string]bool{}
	for _, n := range enumNameRe.FindAllStringSubmatch(string(m[1]), -1) {
		out[n[1]] = true
	}
	if len(out) == 0 {
		t.Fatal("枚举解析结果为空，review_config.pb.go 结构可能已变化")
	}
	return out
}

// scanMySQLRegistered 返回 MySQL 侧已注册实现的规则类型。
func scanMySQLRegistered(t *testing.T, root string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, path := range goFiles(t, filepath.Join(root, "internal/bytebase/plugin/advisor/mysql"), "") {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", path, err)
		}
		for _, m := range registerRe.FindAllStringSubmatch(string(src), -1) {
			out[m[1]] = true
		}
	}
	if len(out) == 0 {
		t.Fatal("未扫描到任何 MySQL 规则注册，注册写法可能已变化")
	}
	return out
}

// scanEnabledRules 返回 rules.go 里实际映射（启用）的规则类型。
func scanEnabledRules(t *testing.T, root string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(root, "internal/server/rules.go"))
	if err != nil {
		t.Fatalf("读取 rules.go 失败: %v", err)
	}
	out := map[string]bool{}
	for _, m := range usedTypeRe.FindAllStringSubmatch(string(src), -1) {
		if levelAndSentinelNames[m[1]] {
			continue
		}
		out[m[1]] = true
	}
	if len(out) == 0 {
		t.Fatal("未在 rules.go 中解析到任何规则类型引用")
	}
	return out
}

// TestRulesManifest 核对 engine/RULES.md 与「上游 MySQL 规则 × 本引擎映射状态」是否一致。
//
// 上游同步后若出现一条既不映射、也没登记的规则，本测试会失败，提示同步的人做一次选择：
//   - 映射到 internal/server/rules.go（并按需在前端规则表加开关）
//   - 或登记进清单（清单是按状态自动生成的，跑一次 UPDATE_RULES_MANIFEST=1 即可）
//
// 目的是消除「上游新增规则静默不生效」：审核页看不出任何异常，也没人会发现。
func TestRulesManifest(t *testing.T) {
	root := moduleRoot(t)
	enumNames := scanEnumNames(t, root)
	registered := scanMySQLRegistered(t, root)
	enabled := scanEnabledRules(t, root)

	var en, dis []string
	for n := range registered {
		if enabled[n] {
			en = append(en, n)
			continue
		}
		dis = append(dis, n)
	}
	sort.Strings(en)
	sort.Strings(dis)

	// 引用了枚举里不存在的类型：上游改名/删除，需同步调整（另一条测试也会报，这里给出清单视角）
	var missing []string
	for n := range enabled {
		if !enumNames[n] {
			missing = append(missing, n)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("rules.go 引用了枚举中不存在的规则（上游可能已改名/删除）: %v", missing)
	}

	want := renderManifest(en, dis)
	path := filepath.Join(root, manifestName)
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		if os.Getenv("UPDATE_RULES_MANIFEST") == "1" {
			if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
				t.Fatalf("写入 %s 失败: %v", manifestName, err)
			}
			t.Logf("已更新 %s：启用 %d 条、未启用 %d 条", manifestName, len(en), len(dis))
			return
		}
		if err != nil {
			t.Fatalf("读取 %s 失败: %v（首次生成请跑 UPDATE_RULES_MANIFEST=1 go test ./internal/archguard -run TestRulesManifest）", manifestName, err)
		}
		onDisk := parseManifestRules(string(got))
		current := manifestRuleSet(en, dis)
		added := missingFrom(onDisk, current)
		removed := missingFrom(current, onDisk)
		if len(added) > 0 {
			t.Errorf("上游新增了未登记的规则（要么在 internal/server/rules.go 里映射，要么更新清单）: %v", added)
		}
		if len(removed) > 0 {
			t.Errorf("清单里有但上游已不存在/未注册的规则（上游可能改名或删除）: %v", removed)
		}
		if len(added) == 0 && len(removed) == 0 {
			t.Errorf("%s 内容与预期不同（可能是启用/未启用状态或标注变化），请审阅 diff", manifestName)
		}
		t.Logf("当前应为：启用 %d 条、未启用 %d 条", len(en), len(dis))
	}
}

// parseManifestRules 提取清单文件里登记的全部规则名。
func parseManifestRules(content string) map[string]bool {
	out := map[string]bool{}
	for _, m := range manifestRuleRe.FindAllStringSubmatch(content, -1) {
		out[m[1]] = true
	}
	return out
}

// manifestRuleSet 把启用/未启用两组规则合成一个集合。
func manifestRuleSet(groups ...[]string) map[string]bool {
	out := map[string]bool{}
	for _, g := range groups {
		for _, n := range g {
			out[n] = true
		}
	}
	return out
}

// missingFrom 返回在 want 里但不在 have 里的元素（用于给出「新增/消失」清单）。
func missingFrom(have, want map[string]bool) []string {
	var out []string
	for n := range want {
		if !have[n] {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// renderManifest 生成清单内容：已启用 / 未启用两组，未启用标注原因分类。
func renderManifest(enabled, disabled []string) string {
	var b strings.Builder
	b.WriteString("# 上游 MySQL 审核规则清单（自动生成，勿手改）\n\n")
	b.WriteString("本文件由 `internal/archguard/rules_manifest_test.go` 生成并核对：\n\n")
	b.WriteString("- 更新：`UPDATE_RULES_MANIFEST=1 go test ./internal/archguard -run TestRulesManifest`\n")
	b.WriteString("- 核对：`go test ./internal/archguard -run TestRulesManifest`（CI 每次都会跑）\n\n")
	b.WriteString("「已启用」= 在 `internal/server/rules.go` 里映射到了 Yearning 的审核开关；\n")
	b.WriteString("「未启用」= 上游有实现、本引擎暂时没有对应开关或缺少必要上下文。\n")
	b.WriteString("上游同步后若这里出现新增条目，说明上游加了规则——按需映射或保持未启用即可，\n")
	b.WriteString("但**不能让它悄悄消失**，这正是本清单存在的意义。\n\n")
	b.WriteString("## 已启用（" + strconv.Itoa(len(enabled)) + " 条）\n\n")
	for _, n := range enabled {
		b.WriteString("- `" + n + "`\n")
	}
	b.WriteString("\n## 未启用（" + strconv.Itoa(len(disabled)) + " 条）\n\n")
	for _, n := range disabled {
		if metadataDependent[n] {
			b.WriteString("- `" + n + "` — 需要库内元数据/执行计划，引擎当前只做静态审核\n")
			continue
		}
		b.WriteString("- `" + n + "`\n")
	}
	return b.String()
}
