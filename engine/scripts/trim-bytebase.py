#!/usr/bin/env python3
"""重放 engine/internal/bytebase 相对 Bytebase 上游的裁剪。

由 sync-bytebase.sh 调用，也可单独执行：trim-bytebase.py <internal/bytebase 目录>

设计原则：每一处裁剪都必须命中，否则报错退出——说明上游结构变了，需要人工调整本脚本，
而不是静默产出一份编译不过或有隐患的内化代码。裁剪清单与 internal/bytebase/README.md 一一对应。
"""

import pathlib
import re
import shutil
import sys

if len(sys.argv) < 2:
    print("用法: trim-bytebase.py <internal/bytebase 目录>", file=sys.stderr)
    sys.exit(2)
DST = pathlib.Path(sys.argv[1]).resolve()
if not DST.is_dir():
    print(f"目录不存在: {DST}", file=sys.stderr)
    sys.exit(2)

changed = []


def fail(msg):
    print(f"裁剪失败: {msg}", file=sys.stderr)
    print("上游结构可能已变化，请对照 engine/internal/bytebase/README.md 修正本脚本。", file=sys.stderr)
    sys.exit(1)


def read(rel):
    return (DST / rel).read_text()


def write(rel, s):
    (DST / rel).write_text(s)
    changed.append(rel)


def rm(rel):
    p = DST / rel
    if not p.exists():
        fail(f"待删路径不存在: {rel}")
    shutil.rmtree(p) if p.is_dir() else p.unlink()


def rm_optional(rel):
    """删除「我们不需要的上游文件」，上游已经删掉的同样视为达标（不报错）。

    与 rm() 的区别在语义：rm() 用于审核链路的**依赖源头**（例如 parser 里的补全/诊断实现，
    它们会拖入 lsp-protocol 与 antlr fork），这类文件一旦在上游改名/搬家，我们必须知道，
    所以要求严格命中；rm_optional() 用于我们自己不需要的上游应用文件（common 下的配置、
    密码策略、审计日志等），上游删掉它们只会让内化更干净，不该因此中断同步。
    3.23.0 就删掉了 common/audit.go 与 common/risk.go。
    """
    p = DST / rel
    if not p.exists():
        return
    shutil.rmtree(p) if p.is_dir() else p.unlink()
    changed.append(rel)


def drop_lines(rel, patterns):
    """按正则删除整行；每个 pattern 都必须命中。"""
    src = read(rel)
    compiled = [re.compile(p) for p in patterns]
    hit = [False] * len(patterns)
    out = []
    for line in src.splitlines(keepends=True):
        idx = next((i for i, c in enumerate(compiled) if c.search(line)), None)
        if idx is None:
            out.append(line)
        else:
            hit[idx] = True
    missing = [p for p, h in zip(patterns, hit) if not h]
    if missing:
        fail(f"{rel}: 未匹配到待删行 {missing}")
    write(rel, "".join(out))


def _drop_callable(rel, header_re, names):
    src = read(rel)
    for name in names:
        m = re.search(header_re.format(name=re.escape(name)), src, re.M)
        if not m:
            fail(f"{rel}: 未找到 {name}")
        start = m.start()
        i = src.index("{", m.end())
        depth = 0
        while i < len(src):
            if src[i] == "{":
                depth += 1
            elif src[i] == "}":
                depth -= 1
                if depth == 0:
                    break
            i += 1
        end = i + 1
        while end < len(src) and src[end] == "\n":
            end += 1
        src = src[:start] + src[end:]
    write(rel, src)


def drop_funcs(rel, names):
    """删除包级函数（含其前导注释）。"""
    _drop_callable(rel, r"(?:^//[^\n]*\n)*^func {name}\(", names)


def drop_methods(rel, names):
    """删除带接收者的方法（含其前导注释）。"""
    _drop_callable(rel, r"(?:^//[^\n]*\n)*^func \([^)\n]*\) {name}\(", names)


def drop_var_entry(rel, names):
    """删除 var (...) 块里的单个条目（含函数字面量等多行初始化，吞掉行尾的 }() 等）。"""
    src = read(rel)
    for name in names:
        m = re.search(rf"^\t{re.escape(name)}\s*[= ]", src, re.M)
        if not m:
            fail(f"{rel}: 未找到 var 条目 {name}")
        i, j, depth = m.start(), m.end(), 0
        while j < len(src):
            c = src[j]
            if c == "{":
                depth += 1
            elif c == "}":
                depth -= 1
            elif c == "\n" and depth <= 0:
                break
            j += 1
        end = j + 1 if j < len(src) else j
        src = src[:i] + src[end:]
    write(rel, src)


def drop_types(rel, names):
    """删除包级类型声明（含其前导注释；struct/接口体按花括号配平，单行别名声明的删到行尾）。"""
    src = read(rel)
    for name in names:
        m = re.search(rf"(?:^//[^\n]*\n)*^type {re.escape(name)} \w* ?[^=]*?(\n|\{{)", src, re.M)
        if not m:
            fail(f"{rel}: 未找到 type {name}")
        start = m.start()
        if m.group(1) == "{":
            i = src.index("{", m.end() - 1)
            depth = 0
            while i < len(src):
                if src[i] == "{":
                    depth += 1
                elif src[i] == "}":
                    depth -= 1
                    if depth == 0:
                        break
                i += 1
            end = i + 1
        else:
            end = m.end()
        while end < len(src) and src[end] == "\n":
            end += 1
        src = src[:start] + src[end:]
    write(rel, src)


def append_text(rel, text):
    """在文件末尾追加内容（幂等：已含 marker 则跳过）。

    用于内化「上游包里的少量代码」：与其为几个纯函数引入整个上游包
    （例如 backend/utils 会拖入 connectrpc / store / genproto），
    不如把用到的那几行抄进来。追加时保留来源说明。
    同步用 rsync 无 --delete，本地追加的内容会在下次同步后存活，故必须幂等。
    """
    src = read(rel)
    marker = text.strip().splitlines()[0]
    if marker in src:
        return
    write(rel, src.rstrip() + "\n\n" + text.strip() + "\n")


def create_file(rel, text):
    """以给定内容创建文件（已存在且内容相同则跳过）。

    用于「上游文件的活代码骨架 + 大量死代码」场景：比几十条 drop 更可靠的方式是
    冻结一份只含活函数的副本。上游若改名/删除该文件，前置的 rm() 会先报错拦下。
    """
    p = DST / rel
    if p.exists() and p.read_text() == text:
        return
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(text)
    changed.append(rel)


def replace_every(rel, old, new):
    """全部替换（至少 1 处，否则视为上游已变化）。"""
    src = read(rel)
    n = src.count(old)
    if n == 0:
        fail(f"{rel}: 未找到 {old!r}")
    write(rel, src.replace(old, new))


def truncate_at(rel, marker):
    src = read(rel)
    i = src.find(marker)
    if i < 0:
        fail(f"{rel}: 未找到截断标记 {marker!r}")
    write(rel, src[:i].rstrip() + "\n")


def replace_once(rel, old, new):
    src = read(rel)
    if src.count(old) != 1:
        fail(f"{rel}: 期望恰好 1 处 {old!r}，实际 {src.count(old)} 处")
    write(rel, src.replace(old, new))


def replace_optional(rel, old, new):
    """可选替换：上游已不存在该内容时跳过（存在时必须恰好 1 处）。

    用于「上游自己改掉了，但内化版本仍不该含该依赖」的改写。例：3.23.0 起
    common/engine.go 不再用 connectrpc 包装错误，对应的两处改写自然失效；
    保留为可选是为了将来上游若重新引入时能被处理，而不是静默漏改。
    """
    src = read(rel)
    n = src.count(old)
    if n == 0:
        return
    if n != 1:
        fail(f"{rel}: 期望最多 1 处 {old!r}，实际 {n} 处")
    write(rel, src.replace(old, new))


# --------------------------------------------------------------------------
# 1. 删除与审核链路无关、或会拖入 LSP / tidb 依赖的文件
# --------------------------------------------------------------------------
for rel in [
    "plugin/parser/base/complete.go",  # 补全（依赖 lsp-protocol 与 antlr fork）
    "plugin/parser/base/diagnose.go",  # 诊断（依赖 lsp-protocol）
    "plugin/parser/base/split_test_runner.go",  # 测试数据驱动辅助（依赖 testify）
    "plugin/parser/mysql/completion.go",  # MySQL 补全实现
    "plugin/parser/mysql/diagnose.go",  # MySQL 诊断实现

    # mariadb（3.24.0 起 mysql 解析器依赖它）：诊断实现会调用 base.RegisterDiagnoseFunc，
    # 而该注册已在 #3 里被裁掉；解析本体（omni.go）保留。
    "plugin/parser/mariadb/diagnose.go",

    # pg：补全/诊断/语句区间同样依赖 lsp-protocol 与 base 里被删掉的注册函数
    "plugin/parser/pg/completion.go",
    "plugin/parser/pg/diagnose.go",
    "plugin/parser/pg/statement_ranges.go",

    # pg advisor：SDL diff 检查，依赖 plugin/schema（未内化）。CheckSDLDropOperations
    # 无任何调用点、也无 init 注册，是本仓库用不到的 SDL 死代码。
    "plugin/advisor/pg/sdl_drop_check.go",

    # ---- query span / backup / restore / 编辑器校验整机：engine 审核链路只用
    # ParseStatements / SplitMultiSQL / GetStatementTypes / GetOmniNode /
    # EstimateAffectedRowsFromExplainJSON，下列机器零调用方 ----
    # base：span/limit/explain/backup/transform/词法行列/文档分析/json_path/拓扑排序/事务模式
    "plugin/parser/base/span.go",
    "plugin/parser/base/limit.go",
    "plugin/parser/base/explain.go",
    "plugin/parser/base/backup.go",
    "plugin/parser/base/transform.go",
    "plugin/parser/base/restore.go",
    "plugin/parser/base/lexer.go",
    "plugin/parser/base/document_analysis.go",
    "plugin/parser/base/json_path.go",
    "plugin/parser/base/topo_sort.go",
    "plugin/parser/base/transaction_mode.go",
    # mysql：query span 提取、备份、还原、explain 注册、limit 注册、资源变更、编辑器校验
    "plugin/parser/mysql/backup.go",
    "plugin/parser/mysql/explain.go",
    "plugin/parser/mysql/limit.go",
    "plugin/parser/mysql/query.go",
    "plugin/parser/mysql/query_span.go",
    "plugin/parser/mysql/query_span_extractor.go",
    "plugin/parser/mysql/resource_change.go",
    "plugin/parser/mysql/restore.go",
    # pg：同上 + antlr 校验器（拖 antlr fork）
    "plugin/parser/pg/access_tables.go",
    "plugin/parser/pg/backup.go",
    "plugin/parser/pg/explain.go",
    "plugin/parser/pg/limit.go",
    "plugin/parser/pg/query.go",
    "plugin/parser/pg/query_antlr.go",
    "plugin/parser/pg/query_span.go",
    "plugin/parser/pg/query_span_plpgsql.go",
    "plugin/parser/pg/resource_change.go",
    "plugin/parser/pg/restore.go",
]:
    rm(rel)

# common：Bytebase 应用级工具，审核链路完全用不到。
# 这一组用 rm_optional：上游新增/删除这些文件都不影响我们的裁剪目标。
for rel in [
    "common/audit.go",  # 审计日志（HTTP/元数据）—— 3.23.0 起上游已删除
    "common/config.go",  # release 模式
    "common/config_dev.go",  # release 模式（dev 构建）
    "common/config_release.go",  # release 模式（release 构建）
    "common/directory_sync.go",  # SCIM 目录同步令牌哈希
    "common/environment.go",  # 环境展示排序
    "common/password.go",  # 密码策略校验
    "common/risk.go",  # 语句风险分级 —— 3.23.0 起上游已删除
    # resource_name.go：38 个 Bytebase 资源名解析器，调用方（backup/restore 机器）已删，
    # 包外零引用。proto.go：SanitizeUTF8Message 零调用，删掉后 common 不再依赖
    # protobuf reflect。
    "common/resource_name.go",
    "common/proto.go",
]:
    rm_optional(rel)

# --------------------------------------------------------------------------
# 2. plugin/advisor/sql_review.go：保留规则类型常量与 payload 解析，砍掉 SQLReviewCheck
#    （SQLReviewCheck 依赖 component/sheet 与 plugin/schema，会引入全部方言解析器）
# --------------------------------------------------------------------------
truncate_at("plugin/advisor/sql_review.go", "// SQLReviewCheck checks the statements with sql review rules.")
drop_lines(
    "plugin/advisor/sql_review.go",
    [
        r'^\s*"context"\s*$',
        r'^\s*"fmt"\s*$',
        r'^\s*"engine/internal/bytebase/common"\s*$',
        r'^\s*"engine/internal/bytebase/component/sheet"\s*$',
        r'^\s*"engine/internal/bytebase/plugin/advisor/code"\s*$',
        r'^\s*"engine/internal/bytebase/plugin/parser/base"\s*$',
        r'^\s*"engine/internal/bytebase/plugin/schema"\s*$',
        r'^\s*_\s*"engine/internal/bytebase/plugin/schema/mysql"\s*$',
        r'^\s*_\s*"engine/internal/bytebase/plugin/schema/pg"\s*$',
        r'^\s*_\s*"engine/internal/bytebase/plugin/schema/tidb"\s*$',
    ],
)

# --------------------------------------------------------------------------
# 3. plugin/parser/base/interface.go：只留 SplitMultiSQL / ParseStatements /
#    GetStatementTypes 三台注册机器。补全/诊断/语句区间（lsp-protocol）、
#    query span / DML→SELECT / 还原 / 资源变更 / 编辑器校验（IsAllDML 及其
#    golang-lru + xxh3 缓存）全部删除。
# --------------------------------------------------------------------------
drop_lines(
    "plugin/parser/base/interface.go",
    [
        # 补全/诊断/语句区间（上游本就有，先删）
        r'^\s*completers\s*=\s*make\(',
        r'^\s*diagnoseCollectors\s*=\s*make\(',
        r'^\s*statementRanges\s*=\s*make\(',
        r'^\s*type CompletionFunc func\(',
        r'^\s*type DiagnoseFunc func\(',
        r'^\s*type StatementRangeFunc func\(',
        r'^\s*type StatementRangeContext struct\{\}\s*$',
        r'^\s*type Range = lsp\.Range\s*$',
        r'^\s*lsp\s+"github\.com/bytebase/lsp-protocol"\s*$',
        # 其余注册表与 IsAllDML 缓存
        r'^\s*queryValidators\s*=\s*make\(',
        r'^\s*changedResourcesGetters\s*=\s*make\(',
        r'^\s*spans\s*=\s*make\(',
        r'^\s*transformDMLToSelect\s*=\s*make\(',
        r'^\s*generateRestoreSQL\s*=\s*make\(',
        r'^\s*// searchPathPrefix starts the statement',
        r'^\s*showplanReg\s*=\s*regexp\.MustCompile',
        r'^\s*// REFACTOR\(zp\): Put it here',
        # 随机器一起消失的 import
        r'^\s*"context"\s*$',
        r'^\s*"regexp"\s*$',
        r'^\s*"strings"\s*$',
        r'^\s*lru\s+"github\.com/hashicorp/golang-lru/v2"\s*$',
        r'^\s*"github\.com/pkg/errors"\s*$',
        r'^\s*"github\.com/zeebo/xxh3"\s*$',
        r'^\s*"engine/internal/bytebase/store/model"\s*$',
    ],
)
drop_types(
    "plugin/parser/base/interface.go",
    [
        "isAllDMLCacheKey",
        "isAllDMLResult",
        "ChangeSummary",
        "ValidateSQLForEditorFunc",
        "ExtractChangedResourcesFunc",
        "GetQuerySpanFunc",
        "TransformDMLToSelectFunc",
        "GenerateRestoreSQLFunc",
    ],
)
drop_var_entry("plugin/parser/base/interface.go", ["isAllDMLCacheMu", "isAllDMLCache"])
# 三台活机器的错误构造改用 stdlib（pkg/errors import 已删）
replace_every("plugin/parser/base/interface.go", "errors.Errorf(", "fmt.Errorf(")
drop_funcs(
    "plugin/parser/base/interface.go",
    [
        # 补全/诊断/语句区间注册（上游本就有）
        "RegisterCompleteFunc",
        "Completion",
        "RegisterDiagnoseFunc",
        "Diagnose",
        "RegisterStatementRangesFunc",
        "GetStatementRanges",
        # 编辑器校验 / 资源变更 / query span / DML→SELECT / 还原 / IsAllDML
        "RegisterQueryValidator",
        "ValidateSQLForEditor",
        "HasQueryValidator",
        "RegisterExtractChangedResourcesFunc",
        "ExtractChangedResources",
        "RegisterGetQuerySpan",
        "GetQuerySpan",
        "RegisterTransformDMLToSelect",
        "TransformDMLToSelect",
        "RegisterGenerateRestoreSQL",
        "GenerateRestoreSQL",
        "IsAllDML",
        "isAllDMLImpl",
        "WithSearchPath",
        "SplitSearchPath",
        "tsqlRecognizeExplainType",
    ],
)

# base.go：补一个 ListDatabaseNamesFunc 类型声明（原在已删的 span.go 里，
# advisor.Context 的字段仍引用它）。引擎侧从未注入实现，advisor.DatabaseExists
# 恒走 nil 分支；接入真实实现时再评估去留。
replace_once(
    "plugin/parser/base/base.go",
    'import storepb "engine/internal/bytebase/generated-go/store"\n',
    'import (\n\t"context"\n\n\tstorepb "engine/internal/bytebase/generated-go/store"\n)\n',
)
append_text(
    "plugin/parser/base/base.go",
    """
// ListDatabaseNamesFunc 原在 query-span 机器（已删）里声明，advisor 的 Context 字段仍在用。
// ponytail: 引擎侧从未注入该实现，advisor.DatabaseExists 恒走 nil 分支；接入真实实现时再评估去留。
type ListDatabaseNamesFunc func(context.Context, string) ([]string, error)
""",
)

# mysql/explain_plan.go：dmlKeywordStart 原属已删的 backup.go，explain 链路仍在用。
append_text(
    "plugin/parser/mysql/explain_plan.go",
    """
// dmlKeywordStart 原属 parser 的 backup 机器（已删），explain 链路仍在用，随迁至此：
// WITH 前缀不算 DML 关键字本身，定位到 UPDATE/DELETE 真正开始处。
func dmlKeywordStart(sql string, loc ast.Loc) int {
	if loc.Start < 0 || loc.End > len(sql) || loc.Start >= loc.End {
		return loc.Start
	}
	tokens := mysqlparser.Tokenize(sql[loc.Start:loc.End])
	if len(tokens) == 0 || mysqlparser.TokenName(tokens[0].Type) != "WITH" {
		return loc.Start
	}
	depth := 0
	for _, token := range tokens[1:] {
		switch token.Type {
		case '(':
			depth++
		case ')':
			depth--
		default:
			if depth != 0 {
				continue
			}
			if name := mysqlparser.TokenName(token.Type); name == "UPDATE" || name == "DELETE" {
				return loc.Start + token.Loc
			}
		}
	}
	return loc.Start
}
""",
)
# explain_plan.go 引入 mysqlparser import（幂等：已有则跳过）
replace_optional(
    "plugin/parser/mysql/explain_plan.go",
    '\t"github.com/bytebase/omni/mysql/ast"\n\t"github.com/pkg/errors"\n',
    '\t"github.com/bytebase/omni/mysql/ast"\n\tmysqlparser "github.com/bytebase/omni/mysql/parser"\n\t"github.com/pkg/errors"\n',
)

# pg/query_type.go：上游版本是 QueryType 分类器（query span/编辑器校验链路），
# 死代码为主；statement-type 仍需其中 6 个纯 AST 辅助 + 已删 explain.go 里的
# explainedText。先 rm 保证上游改名时 loudly fail，再冻结一份只含活函数的副本。
rm("plugin/parser/pg/query_type.go")
create_file(
    "plugin/parser/pg/query_type.go",
    '''package pg

import (
	"strings"

	"github.com/bytebase/omni/pg/ast"
)

// 原文件还含 QueryType 分类器（query span/编辑器校验链路），机器已删，只留 statement-type
// 仍在用的这几个纯 AST 辅助。

// getWithClause returns the WITH clause of a query-carrying statement.
func getWithClause(node ast.Node) *ast.WithClause {
	switch n := node.(type) {
	case *ast.SelectStmt:
		return n.WithClause
	case *ast.InsertStmt:
		return n.WithClause
	case *ast.UpdateStmt:
		return n.WithClause
	case *ast.DeleteStmt:
		return n.WithClause
	case *ast.MergeStmt:
		return n.WithClause
	case *ast.CreateTableAsStmt:
		return getWithClause(n.Query)
	default:
		return nil
	}
}

// hasOmniIntoClause checks if a SelectStmt has an INTO clause (SELECT INTO).
func hasOmniIntoClause(n *ast.SelectStmt) bool {
	return omniIntoClause(n) != nil
}

// omniIntoClause returns the INTO clause of n, searching set-operation arms
// (UNION/INTERSECT/EXCEPT) — the parser attaches INTO to the first arm, not the root.
func omniIntoClause(n *ast.SelectStmt) *ast.IntoClause {
	if n == nil {
		return nil
	}
	if n.IntoClause != nil {
		return n.IntoClause
	}
	if c := omniIntoClause(n.Larg); c != nil {
		return c
	}
	return omniIntoClause(n.Rarg)
}

// isExplainAnalyzeOmni reports whether an ExplainStmt executes its query: its last ANALYZE option
// is not FALSE, OFF, or 0, the values PostgreSQL reads as false.
func isExplainAnalyzeOmni(n *ast.ExplainStmt) bool {
	if n.Options == nil {
		return false
	}
	analyze := false
	for _, item := range n.Options.Items {
		de, ok := item.(*ast.DefElem)
		if !ok || !strings.EqualFold(de.Defname, "analyze") {
			continue
		}
		switch arg := de.Arg.(type) {
		case *ast.String:
			analyze = !strings.EqualFold(arg.Str, "false") && !strings.EqualFold(arg.Str, "off")
		case *ast.Integer:
			analyze = arg.Ival != 0
		default:
			analyze = true
		}
	}
	return analyze
}

// UnwrapExplainAnalyze returns the statement that an EXPLAIN ANALYZE executes and that statement's text
// within text, the text of the EXPLAIN. It returns any other node and text unchanged.
func UnwrapExplainAnalyze(node ast.Node, text string) (ast.Node, string) {
	explain, ok := node.(*ast.ExplainStmt)
	if !ok || !isExplainAnalyzeOmni(explain) {
		return node, text
	}
	inner, ok := explainedText(explain, text)
	if !ok {
		return explain.Query, text
	}
	return explain.Query, inner
}

// explainedText returns the text of the statement an EXPLAIN runs, within the EXPLAIN's own text.
func explainedText(explain *ast.ExplainStmt, text string) (string, bool) {
	// The statement runs from its WITH clause, which the location of a SELECT leaves out, to the end
	// of the EXPLAIN, which its location can also leave out, as for ORDER BY.
	start := ast.NodeLoc(explain.Query).Start
	if with := getWithClause(explain.Query); with != nil && with.Loc.Start >= 0 && with.Loc.Start < start {
		start = with.Loc.Start
	}
	if start < 0 || start >= explain.Loc.End || explain.Loc.End > len(text) {
		return "", false
	}
	return text[start:explain.Loc.End], true
}
''',
)

# --------------------------------------------------------------------------
# 4. prior-backup / builtin 规则：EnablePriorBackup / EnableGhost 全仓库从未置位，
#    这两条规则的第一道门恒为假（Check 永远直接返回 nil）；prior-backup 还引用
#    已删的 mysqlparser 提取机器。builtin_rules.go 的 GetBuiltinRules 零调用。
#    Yearning 的规则清单（RULES.md / archguard）同步把它们记为不存在。
# --------------------------------------------------------------------------
for rel in [
    "plugin/advisor/mysql/rule_builtin_prior_backup_check.go",
    "plugin/advisor/pg/advisor_builtin_prior_backup_check.go",
    "plugin/advisor/builtin_rules.go",
]:
    rm(rel)
# defaultSchema 原在已删的 pg prior-backup 规则里，object-owner 检查仍在用。
append_text(
    "plugin/advisor/pg/advisor_statement_object_owner_check.go",
    """
// defaultSchema 原在已删的 prior-backup 规则文件里，object-owner 检查仍在用。
const defaultSchema = "public"
""",
)

# --------------------------------------------------------------------------
# 5. generated-go/store：protobuf 只留审核链路用到的 5 个文件（advice / common /
#    database / plan_check_run[ChangedResource 家族] / review_config[SQLReviewRule]）。
#    *_equal.pb.go（Equal 方法零调用）与其余 30+ 个应用级 proto（issue/plan/task/
#    project/user/setting/…）全部删除。rm_optional 语义：上游新增 proto 不应中断同步。
# --------------------------------------------------------------------------
PROTO_KEEP = {
    "advice.pb.go",
    "common.pb.go",
    "database.pb.go",
    "plan_check_run.pb.go",
    "review_config.pb.go",
}
for p in sorted((DST / "generated-go/store").glob("*.pb.go")):
    if p.name not in PROTO_KEEP:
        rm_optional(f"generated-go/store/{p.name}")

# --------------------------------------------------------------------------
# 6. plugin/db/mysql/stats.go：只保留 ExplainJSON / EstimateAffectedRows 两个包级函数
#    内化只拷了 stats.go 这一个文件（见 sync-bytebase.sh），Driver 类型在 mysql.go，
#    那里是完整驱动实现（会拖 AWS SDK 等），所以绑定在 Driver 上的方法必须删掉。
# --------------------------------------------------------------------------
drop_methods("plugin/db/mysql/stats.go", ["CountAffectedRows"])
# 该方法原本是 storepb 的唯一使用者，删掉后 import 会悬空
drop_lines("plugin/db/mysql/stats.go", [r'^\s*storepb\s+"engine/internal/bytebase/generated-go/store"\s*$'])

# --------------------------------------------------------------------------
# 7. plugin/advisor/pg/advisor_statement_non_transactional.go：内联 IsNonTransactionStatement
#    （backend/plugin/db/pg 是完整 pg 驱动、未内化；该函数只是 4 个正则判断、不连库）
# --------------------------------------------------------------------------
drop_lines("plugin/advisor/pg/advisor_statement_non_transactional.go", [r'^\s*"engine/internal/bytebase/plugin/db/pg"\s*$'])
replace_once(
    "plugin/advisor/pg/advisor_statement_non_transactional.go",
    '\t"context"\n',
    '\t"context"\n\t"regexp"\n',
)
replace_once(
    "plugin/advisor/pg/advisor_statement_non_transactional.go",
    "pg.IsNonTransactionStatement(",
    "isNonTransactionStatement(",
)
append_text(
    "plugin/advisor/pg/advisor_statement_non_transactional.go",
    """
// isNonTransactionStatement 内联自上游 backend/plugin/db/pg.IsNonTransactionStatement。
// 内化版本不引入 backend/plugin/db/pg（那是完整 pg 驱动、会拖重依赖），
// 该函数只做 4 个正则判断、不依赖连接。
var (
	isNonTransactionalDropDatabaseReg = regexp.MustCompile(`(?i)DROP DATABASE`)
	isNonTransactionalCreateIndexReg  = regexp.MustCompile(`(?i)CREATE(\s+(UNIQUE\s+)?)INDEX(\s+)CONCURRENTLY`)
	isNonTransactionalDropIndexReg    = regexp.MustCompile(`(?i)DROP(\s+)INDEX(\s+)CONCURRENTLY`)
	isNonTransactionalVacuumReg       = regexp.MustCompile(`(?i)^\s*VACUUM`)
)

func isNonTransactionStatement(stmt string) bool {
	if len(isNonTransactionalDropDatabaseReg.FindString(stmt)) > 0 {
		return true
	}
	if len(isNonTransactionalCreateIndexReg.FindString(stmt)) > 0 {
		return true
	}
	if len(isNonTransactionalDropIndexReg.FindString(stmt)) > 0 {
		return true
	}
	return len(isNonTransactionalVacuumReg.FindString(stmt)) > 0
}
""",
)

# --------------------------------------------------------------------------
# 8. common：去掉手机号校验（phonenumbers）与 connectrpc 依赖
# --------------------------------------------------------------------------
drop_funcs("common/util.go", ["ValidatePhone"])
drop_lines("common/util.go", [r'^\s*"github\.com/nyaruka/phonenumbers/v2"\s*$'])

# 3.23.0 起上游已不再用 connectrpc 包装这个错误，故用 replace_optional：
# 内化版本的目标始终是「不含 connectrpc 依赖」，上游自己做到也算达标。
replace_optional("common/engine.go", '\t"connectrpc.com/connect"\n', "")
replace_optional(
    "common/engine.go",
    'connect.NewError(connect.CodeInvalidArgument, errors.Errorf("invalid engine type %v", e))',
    'errors.Errorf("invalid engine type %v", e)',
)

print("裁剪完成，涉及文件:")
for rel in sorted(set(changed)):
    print(f"  - {rel}")
