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


def append_text(rel, text):
    """在文件末尾追加内容。

    用于内化「上游包里的少量代码」：与其为几个纯函数引入整个上游包
    （例如 backend/utils 会拖入 connectrpc / store / genproto），
    不如把用到的那几行抄进来。追加时保留来源说明。
    """
    src = read(rel)
    write(rel, src.rstrip() + "\n\n" + text.strip() + "\n")


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

    # pg：补全/诊断/语句区间同样依赖 lsp-protocol 与 base 里被删掉的注册函数
    "plugin/parser/pg/completion.go",
    "plugin/parser/pg/diagnose.go",
    "plugin/parser/pg/statement_ranges.go",

    # pg advisor：SDL diff 检查，依赖 plugin/schema（未内化）。CheckSDLDropOperations
    # 无任何调用点、也无 init 注册，是本仓库用不到的 SDL 死代码。
    "plugin/advisor/pg/sdl_drop_check.go",
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
# 3. plugin/parser/base/interface.go：去掉补全 / 诊断 / 语句区间注册（依赖 lsp-protocol）
# --------------------------------------------------------------------------
drop_lines(
    "plugin/parser/base/interface.go",
    [
        r'^\s*completers\s*=\s*make\(',
        r'^\s*diagnoseCollectors\s*=\s*make\(',
        r'^\s*statementRanges\s*=\s*make\(',
        r'^\s*type CompletionFunc func\(',
        r'^\s*type DiagnoseFunc func\(',
        r'^\s*type StatementRangeFunc func\(',
        r'^\s*type StatementRangeContext struct\{\}\s*$',
        r'^\s*type Range = lsp\.Range\s*$',
        r'^\s*lsp\s+"github\.com/bytebase/lsp-protocol"\s*$',
    ],
)
drop_funcs(
    "plugin/parser/base/interface.go",
    [
        "RegisterCompleteFunc",
        "Completion",
        "RegisterDiagnoseFunc",
        "Diagnose",
        "RegisterStatementRangesFunc",
        "GetStatementRanges",
    ],
)

# --------------------------------------------------------------------------
# 4. plugin/parser/tokenizer/tokenizer.go：去掉依赖 pingcap/tidb 的建表行号修正
# --------------------------------------------------------------------------
drop_methods("plugin/parser/tokenizer/tokenizer.go", ["SetLineForMySQLCreateTableStmt"])
drop_funcs("plugin/parser/tokenizer/tokenizer.go", ["matchMySQLTableConstraint"])
drop_lines(
    "plugin/parser/tokenizer/tokenizer.go",
    [
        r'^\s*tidbast\s+"github\.com/pingcap/tidb/pkg/parser/ast"\s*$',
        r'^\s*"strings"\s*$',
    ],
)

# --------------------------------------------------------------------------
# 5. plugin/parser/base/limit.go：内联 utils.IsSpaceOrSemicolon
#    （backend/utils 会拖入 connectrpc / store / genproto，只用到一个纯函数）
# --------------------------------------------------------------------------
drop_lines("plugin/parser/base/limit.go", [r'^\s*"engine/internal/bytebase/utils"\s*$'])
replace_once("plugin/parser/base/limit.go", "utils.IsSpaceOrSemicolon", "isSpaceOrSemicolon")
append_text(
    "plugin/parser/base/limit.go",
    """
// isSpaceOrSemicolon 内联自上游 backend/utils.IsSpaceOrSemicolon。
// 内化版本不引入 backend/utils：那个包会连带 connectrpc / store / genproto
// 等与审核链路无关的重依赖，而这里只需要这一个纯函数。
func isSpaceOrSemicolon(r rune) bool {
	return unicode.IsSpace(r) || r == ';'
}
""",
)

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

replace_once("common/resource_name.go", '\t"connectrpc.com/connect"\n', "")
replace_once(
    "common/resource_name.go",
    'connect.NewError(connect.CodeInvalidArgument, errors.New("policy parent resource name must not be empty"))',
    'errors.New("policy parent resource name must not be empty")',
)
replace_once(
    "common/resource_name.go",
    "connect.NewError(connect.CodeInvalidArgument, err)",
    "err",
)

print("裁剪完成，涉及文件:")
for rel in sorted(set(changed)):
    print(f"  - {rel}")
