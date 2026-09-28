package archguard

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	enginev1 "engine/gen/engine/v1"
	"engine/internal/customrules"
	"engine/internal/mongocheck"
)

// selfRulesManifestName 是自研规则的清单文件（engine/SELF_RULES.md），
// 与上游规则清单 RULES.md 并列：那份管「上游有没有被漏掉」，这份管「自己写的有没有被漏掉」。
const selfRulesManifestName = "SELF_RULES.md"

// selfRuleGroups 是清单里小节的固定顺序。
var selfRuleGroups = []string{
	"SQL 侧 · internal/customrules",
	"MongoDB 侧 · internal/mongocheck",
	"执行期限制 · 非静态审核",
}

// mongoExecOnly 是 Mongo 侧「不参与静态审核、但同样是规则集里一项」的开关：
// 引擎只透传配置，判定发生在主程序（执行期限制，静态审核拿不到命中数）。
var mongoExecOnly = map[string]string{
	"MongoMaxAffectRows": "单次 update / delete 允许命中的文档数上限（0 = 不限），由主程序抓取前镜像时判定",
}

// selfRuleRe 匹配清单里 "- `NAME` — 开关：" 形式的条目。
var selfRuleRe = regexp.MustCompile("(?m)^- `([^`]+)` — 开关：")

// selfRule 一条自研规则在清单里的登记项。
type selfRule struct {
	Group    string   // 小节标题
	Name     string   // 规则名
	Switches []string // 对应 enginev1.AuditRole 里的开关字段名
	Desc     string   // 说明
}

// TestSelfRulesManifest 核对 engine/SELF_RULES.md 与代码里真实存在的自研规则是否一致。
//
// 目的是让「自研规则」也有清单可查：新增/删除/改名一条规则而不更新清单，测试会失败。
// 与上游清单的区别在于数据来源——这里直接读注册表（customrules.All / mongocheck.All），
// 不是扫源码正则，所以规则只要真的注册了就一定会出现在清单里。
//
// 更新：UPDATE_SELF_RULES_MANIFEST=1 go test ./internal/archguard -run TestSelfRule
func TestSelfRulesManifest(t *testing.T) {
	rules := scanSelfRules(t)
	want := renderSelfRulesManifest(rules)
	path := filepath.Join(moduleRoot(t), selfRulesManifestName)

	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		if os.Getenv("UPDATE_SELF_RULES_MANIFEST") == "1" {
			if werr := os.WriteFile(path, []byte(want), 0o644); werr != nil {
				t.Fatalf("写入 %s 失败: %v", selfRulesManifestName, werr)
			}
			t.Logf("已更新 %s：%d 条自研规则", selfRulesManifestName, len(rules))
			return
		}
		if err != nil {
			t.Fatalf("读取 %s 失败: %v（首次生成请跑 UPDATE_SELF_RULES_MANIFEST=1 go test ./internal/archguard -run TestSelfRule）", selfRulesManifestName, err)
		}
		onDisk := parseSelfRulesManifest(string(got))
		current := map[string]bool{}
		for _, r := range rules {
			current[r.Name] = true
		}
		added := missingFrom(onDisk, current)
		removed := missingFrom(current, onDisk)
		if len(added) > 0 {
			t.Errorf("代码里有但清单未登记的自研规则（新增规则后请更新清单）: %v", added)
		}
		if len(removed) > 0 {
			t.Errorf("清单里有但代码中已不存在的自研规则（删除/改名后请更新清单）: %v", removed)
		}
		if len(added) == 0 && len(removed) == 0 {
			t.Errorf("%s 内容与预期不同（可能是开关或说明变化），请审阅 diff", selfRulesManifestName)
		}
	}
}

// TestSelfRuleSwitchesExistInAuditRole 核对每条自研规则的开关都真实存在于 enginev1.AuditRole。
//
// 这条护栏挡的是一类静默失效：规则写好了、代码也跑了，但开关名与 proto 字段名不一致——
// 规则页里配不出来，rule_levels 里的级别也取不到（引擎按字段名查 map），
// 结果规则要么永远不生效、要么永远按默认 error 拦截，两种都很难从页面上看出来。
func TestSelfRuleSwitchesExistInAuditRole(t *testing.T) {
	fields := auditRoleFields(t)
	for _, r := range scanSelfRules(t) {
		if len(r.Switches) == 0 {
			t.Errorf("自研规则 %q 没有登记任何开关", r.Name)
			continue
		}
		for _, sw := range r.Switches {
			if !hasField(fields, sw) {
				t.Errorf("自研规则 %q 的开关 %q 不在 enginev1.AuditRole 里：规则页配不出来，rule_levels 的级别也无处安放", r.Name, sw)
			}
		}
	}
}

// TestMongoSwitchesAreImplemented 反向核对：AuditRole 里每个 Mongo* 开关都必须有实现。
//
// 加了开关却忘了写规则（或写了规则却登记错名字），页面会多出一个勾了也不起作用的开关，
// 这里让它变成测试失败。
func TestMongoSwitchesAreImplemented(t *testing.T) {
	covered := map[string]bool{}
	for _, r := range mongocheck.All() {
		covered[normSwitch(r.Name())] = true
	}
	for name := range mongoExecOnly {
		covered[normSwitch(name)] = true
	}
	found := 0
	for _, name := range auditRoleFields(t) {
		if !strings.HasPrefix(strings.ToLower(name), "mongo") {
			continue
		}
		found++
		if !covered[normSwitch(name)] {
			t.Errorf("enginev1.AuditRole 有 %q，但引擎侧没有实现：请写一条 mongocheck 规则（静态审核）或登记进 mongoExecOnly（执行期限制）", name)
		}
	}
	if found == 0 {
		t.Error("未在 AuditRole 里找到任何 Mongo* 开关，proto 生成物可能有问题")
	}
}

// TestMongoConfigCoversAllSwitches 核对 proto 里每个 Mongo* 开关都能被 mongocheck.Config 取到。
//
// 挡的是「加了 proto 字段 + 前端开关，忘了在 Config 里加字段」这一类静默失效：
// 页面勾得动、规则集存得下，但引擎读不到，规则永远不生效。
// 执行期限制（mongoExecOnly）不参与静态审核，Config 里不需要它。
func TestMongoConfigCoversAllSwitches(t *testing.T) {
	cfg := mongocheck.Config{}
	checked := 0
	for _, name := range auditRoleFields(t) {
		if !strings.HasPrefix(strings.ToLower(name), "mongo") {
			continue
		}
		if _, execOnly := mongoExecOnlyByName(name); execOnly {
			continue
		}
		checked++
		if !cfg.Has(name) {
			t.Errorf("enginev1.AuditRole 的 %q 在 mongocheck.Config 里没有对应字段：规则读不到它，开关会变成「勾了不生效」", name)
		}
	}
	if checked == 0 {
		t.Error("未在 AuditRole 里找到任何参与静态审核的 Mongo* 开关")
	}
}

// TestMongoRuleDefaultLevels 核对规则自带的默认级别：
// 取值必须合法，且安全类规则必须默认拦截——安全类降成 warn/observe 意味着
// 「默认不拦」，那种降级只能由使用者在规则集里显式配置，不能写死在代码里。
func TestMongoRuleDefaultLevels(t *testing.T) {
	for _, r := range mongocheck.All() {
		lv := r.DefaultLevel()
		if !mongocheck.ValidLevel(lv) {
			t.Errorf("规则 %q 的默认级别非法: %q", r.Name(), lv)
			continue
		}
		if r.Category() == mongocheck.CategorySecurity && lv != mongocheck.LevelError {
			t.Errorf("安全类规则 %q 的默认级别是 %q，必须是 error", r.Name(), lv)
		}
	}
}

// TestMongoRulesHaveFrontendEntries 核对每条引擎规则都能在规则页配出来。
//
// 规则页的开关列表（rules.ts）与文案（两个语言的 i18n）是前端**静态**维护的，
// 引擎加了规则而前端没加，页面上就找不到这个开关——用户以为没这条规则。
// 这是跨语言的核对，只能扫文本，但足够挡住「加了引擎规则忘了前端」。
func TestMongoRulesHaveFrontendEntries(t *testing.T) {
	root := filepath.Join(moduleRoot(t), "..")
	rulesTS := readFile(t, filepath.Join(root, "front/src/views/manager/rules/rules.ts"))
	langs := map[string]string{
		"zh-cn": readFile(t, filepath.Join(root, "front/src/lang/zh-cn/rule/index.ts")),
		"en-us": readFile(t, filepath.Join(root, "front/src/lang/en-us/rule/index.ts")),
	}
	for _, r := range mongocheck.All() {
		if !strings.Contains(rulesTS, "'"+r.Name()+"'") {
			t.Errorf("规则 %q 不在 front/src/views/manager/rules/rules.ts 里：规则页配不出这个开关", r.Name())
		}
		for lang, content := range langs {
			if !strings.Contains(content, r.Name()+":") {
				t.Errorf("规则 %q 缺少 %s 文案（front/src/lang/%s/rule/index.ts）", r.Name(), lang, lang)
			}
		}
	}
}

// mongoExecOnlyByName 归一化匹配 mongoExecOnly 里的开关名
func mongoExecOnlyByName(name string) (string, bool) {
	for k := range mongoExecOnly {
		if normSwitch(k) == normSwitch(name) {
			return k, true
		}
	}
	return "", false
}

// readFile 读文件，失败即测试失败（护栏读不到文件等于没护栏）
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", path, err)
	}
	return string(b)
}

// auditRoleFields 返回 enginev1.AuditRole 的全部字段名（proto 名，snake_case）。
func auditRoleFields(t *testing.T) []string {
	t.Helper()
	md := (&enginev1.AuditRole{}).ProtoReflect().Descriptor()
	fds := md.Fields()
	out := make([]string, 0, fds.Len())
	for i := 0; i < fds.Len(); i++ {
		out = append(out, string(fds.Get(i).Name()))
	}
	if len(out) == 0 {
		t.Fatal("AuditRole 描述符为空，proto 生成物可能有问题")
	}
	return out
}

// normSwitch 规范化开关名后比较：Go/前端侧写 PascalCase（MongoForbidEmptyFilter），
// proto 字段名是 snake_case（mongo_forbid_empty_filter），两者只是命名风格不同。
func normSwitch(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, "_", ""))
}

// hasField 判断开关是否存在于字段列表，命名风格不敏感。
func hasField(fields []string, name string) bool {
	want := normSwitch(name)
	for _, f := range fields {
		if normSwitch(f) == want {
			return true
		}
	}
	return false
}

// scanSelfRules 收集全部自研规则：customrules 读注册表，mongocheck 读规则表，
// 再加上不参与静态审核的执行期限制开关。
func scanSelfRules(t *testing.T) []selfRule {
	t.Helper()
	var out []selfRule

	all := customrules.All()
	if len(all) == 0 {
		t.Fatal("未扫描到任何 customrules 规则：init 注册是否被移除了？")
	}
	for _, r := range all {
		d, ok := r.(customrules.Describable)
		if !ok {
			t.Errorf("自研规则 %q 未实现 customrules.Describable：清单登记不了它的开关", r.Name())
			continue
		}
		out = append(out, selfRule{
			Group:    selfRuleGroups[0],
			Name:     r.Name(),
			Switches: d.Switches(),
			Desc:     d.Desc(),
		})
	}

	mongoRules := mongocheck.All()
	if len(mongoRules) == 0 {
		t.Fatal("未扫描到任何 mongocheck 规则：注册表是不是空了（每条规则都应在 init 里 Register）？")
	}
	for _, r := range mongoRules {
		// Mongo 规则的规则名就是开关字段名（也是 rule_levels 的 key）
		out = append(out, selfRule{
			Group:    selfRuleGroups[1],
			Name:     r.Name(),
			Switches: []string{r.Name()},
			Desc:     r.Desc(),
		})
	}

	for name, desc := range mongoExecOnly {
		out = append(out, selfRule{
			Group:    selfRuleGroups[2],
			Name:     name,
			Switches: []string{name},
			Desc:     desc,
		})
	}

	groupIdx := func(g string) int {
		for i, n := range selfRuleGroups {
			if n == g {
				return i
			}
		}
		return len(selfRuleGroups)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return groupIdx(out[i].Group) < groupIdx(out[j].Group)
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// parseSelfRulesManifest 提取清单里登记的全部规则名。
func parseSelfRulesManifest(content string) map[string]bool {
	out := map[string]bool{}
	for _, m := range selfRuleRe.FindAllStringSubmatch(content, -1) {
		out[m[1]] = true
	}
	return out
}

// renderSelfRulesManifest 生成清单内容。
func renderSelfRulesManifest(rules []selfRule) string {
	var b strings.Builder
	b.WriteString("# 自研审核规则清单（自动生成，勿手改）\n\n")
	b.WriteString("本文件由 `internal/archguard/self_rules_manifest_test.go` 生成并核对：\n\n")
	b.WriteString("- 更新：`UPDATE_SELF_RULES_MANIFEST=1 go test ./internal/archguard -run TestSelfRule`\n")
	b.WriteString("- 核对：`go test ./internal/archguard -run TestSelfRule`（CI 每次都会跑）\n\n")
	b.WriteString("自研规则 = 上游 bytebase 没有、本仓库自己实现的规则，分两处存放：\n\n")
	b.WriteString("- `internal/customrules`：SQL 侧（上游虽有 SQL 规则，但这几条静态规则上游没有实现）\n")
	b.WriteString("- `internal/mongocheck`：MongoDB 侧（上游完全没有 Mongo 审核能力，\n")
	b.WriteString("  `common.EngineSupportSQLReview(MONGODB)` 为 false，advisor 目录里也没有 mongodb 方言）\n\n")
	b.WriteString("每条规则的开关都必须在 `enginev1.AuditRole` 里存在：否则规则页配不出来、\n")
	b.WriteString("`rule_levels` 里的级别也取不到（引擎按字段名查 map）。这条由\n")
	b.WriteString("`TestSelfRuleSwitchesExistInAuditRole` 核对，反方向由 `TestMongoSwitchesAreImplemented` 兜住。\n\n")
	b.WriteString("上游规则清单见同目录的 `RULES.md`（那份管「上游新增的有没有被漏掉」，与本文件互补）。\n")

	for _, g := range selfRuleGroups {
		var items []selfRule
		for _, r := range rules {
			if r.Group == g {
				items = append(items, r)
			}
		}
		b.WriteString("\n## " + g + "（" + strconv.Itoa(len(items)) + " 条）\n\n")
		if len(items) == 0 {
			b.WriteString("（暂无）\n")
			continue
		}
		for _, r := range items {
			quoted := make([]string, 0, len(r.Switches))
			for _, sw := range r.Switches {
				quoted = append(quoted, "`"+sw+"`")
			}
			b.WriteString("- `" + r.Name + "` — 开关：" + strings.Join(quoted, "、") + "；" + r.Desc + "\n")
		}
	}
	return b.String()
}
