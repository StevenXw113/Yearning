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


# --------------------------------------------------------------------------
# 1. 删除与审核链路无关、或会拖入 LSP / tidb 依赖的文件
# --------------------------------------------------------------------------
for rel in [
    "plugin/parser/base/complete.go",  # 补全（依赖 lsp-protocol 与 antlr fork）
    "plugin/parser/base/diagnose.go",  # 诊断（依赖 lsp-protocol）
    "plugin/parser/base/split_test_runner.go",  # 测试数据驱动辅助（依赖 testify）
    "plugin/parser/mysql/completion.go",  # MySQL 补全实现
    "plugin/parser/mysql/diagnose.go",  # MySQL 诊断实现

    # common：Bytebase 应用级工具，审核链路完全用不到
    "common/audit.go",  # 审计日志（HTTP/元数据）
    "common/config.go",  # release 模式
    "common/config_dev.go",  # release 模式（dev 构建）
    "common/config_release.go",  # release 模式（release 构建）
    "common/directory_sync.go",  # SCIM 目录同步令牌哈希
    "common/environment.go",  # 环境展示排序
    "common/password.go",  # 密码策略校验
    "common/risk.go",  # 语句风险分级
]:
    rm(rel)

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
# 5. common：去掉手机号校验（phonenumbers）与 connectrpc 依赖
# --------------------------------------------------------------------------
drop_funcs("common/util.go", ["ValidatePhone"])
drop_lines("common/util.go", [r'^\s*"github\.com/nyaruka/phonenumbers/v2"\s*$'])

replace_once("common/engine.go", '\t"connectrpc.com/connect"\n', "")
replace_once(
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
