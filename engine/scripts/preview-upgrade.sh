#!/usr/bin/env bash
#
# 预演一次上游升级：在临时 git worktree 里跑一遍真实同步，报告影响面（文件增删、
# 规则增删、依赖变化、能否构建通过），**完全不动当前工作区**。
#
# 典型用法（页面「检测上游更新」提示有新版本后）：
#   cd engine && ./scripts/preview-upgrade.sh 3.23.0
#
# 看完成功的预演再决定是否正式执行：
#   ./scripts/sync-bytebase.sh 3.23.0
#
# 注意：预演用的是 **HEAD（已提交）** 的脚本与代码，不是工作区里未提交的改动；
#       本脚本自身有改动时请先提交再预演。
#
# 退出码：0=预演成功（可升级） 1=预演失败（需人工处理，工作区未受影响） 2=用法/环境错误。
set -euo pipefail

REF="${1:-}"
if [ -z "$REF" ]; then
	echo "用法: $0 <bytebase-ref>（如 3.23.0）" >&2
	exit 2
fi

SCRIPTS="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPTS/.." && pwd)"        # engine/
REPO_ROOT="$(cd "$ROOT/.." && pwd)"      # 仓库根
WORK="$(mktemp -d)"
WT="$WORK/wt"
LOG="$WORK/preview.log"

cleanup() {
	git -C "$REPO_ROOT" worktree remove --force "$WT" >/dev/null 2>&1 || true
	rm -rf "$WORK"
}
trap cleanup EXIT

echo "==> 创建临时工作区（HEAD 的副本，不碰当前目录）"
git -C "$REPO_ROOT" worktree add --detach --quiet "$WT" HEAD

BEFORE_REF="$(awk '/^ref:/{print $2}' "$WT/engine/internal/bytebase/UPSTREAM" 2>/dev/null || echo 未知)"

echo "==> 在临时工作区里跑真实同步: $REF"
CODE=0
( cd "$WT/engine" && bash scripts/sync-bytebase.sh "$REF" ) >"$LOG" 2>&1 || CODE=$?

AFTER_REF="$(awk '/^ref:/{print $2}' "$WT/engine/internal/bytebase/UPSTREAM" 2>/dev/null || echo "$BEFORE_REF")"

echo
if [ "$CODE" != "0" ]; then
	echo "结论：预演失败（退出码 ${CODE}）——按当前状态无法升级，需要人工处理。"
	echo "      当前工作区未受影响（改动都在临时工作区里，已丢弃）。"
	echo
	echo "失败原因（日志尾部）："
	tail -20 "$LOG" | sed 's/^/  /'
	echo
	echo "常见三类及处理："
	echo "  - 裁剪锚点未命中：上游挪了代码结构，按提示修正 engine/scripts/trim-bytebase.py"
	echo "  - 适配层编译错误：上游 API 漂移，按报错改 engine/internal/server 或 internal/mysqlparse"
	echo "  - 依赖解析失败：先确认能访问模块代理（go env GOPROXY），再重试"
	exit 1
fi

echo "结论：预演成功，可以正式升级。"
echo
echo "影响面："
echo "  - 版本        : $BEFORE_REF -> $AFTER_REF"
# UPSTREAM 里的 synced 时间戳每次都变，不算变化，故排除
printf '  - 文件变化    : '
git -C "$WT" status --porcelain -- engine/internal/bytebase ':(exclude)engine/internal/bytebase/UPSTREAM' | wc -l | tr -d ' '
printf '  - 代码 diff   : '
git -C "$WT" diff --shortstat -- engine/internal/bytebase engine/go.mod engine/go.sum engine/RULES.md \
	':(exclude)engine/internal/bytebase/UPSTREAM' | tr -d '\n'
echo
printf '  - 依赖变化    : '
DEP="$(git -C "$WT" diff --stat -- engine/go.mod engine/go.sum | tail -1 | sed 's/^ *//')"
echo "${DEP:-无}"
printf '  - 规则清单    : '
if [ -f "$WT/engine/RULES.md" ]; then
	EN="$(grep -m1 '^## 已启用' "$WT/engine/RULES.md" | sed 's/^## //')"
	DIS="$(grep -m1 '^## 未启用' "$WT/engine/RULES.md" | sed 's/^## //')"
	echo "$EN / $DIS"
else
	echo "（本次未生成）"
fi

# 规则增删只有在 HEAD 里已跟踪 RULES.md 时才有基线（未跟踪时只是首次生成）
if git -C "$WT" cat-file -e "HEAD:engine/RULES.md" 2>/dev/null; then
	ADDED="$(git -C "$WT" diff -- engine/RULES.md | grep '^+- `' | sed 's/^+- `\([A-Z0-9_]*\)`.*/\1/' | tr '\n' ' ' || true)"
	GONE="$(git -C "$WT" diff -- engine/RULES.md | grep '^-- `' | sed 's/^-- `\([A-Z0-9_]*\)`.*/\1/' | tr '\n' ' ' || true)"
	if [ -n "$ADDED" ]; then
		echo "  - 新增未启用规则: $ADDED"
	fi
	if [ -n "$GONE" ]; then
		echo "  - 上游已移除规则: $GONE"
	fi
else
	echo "  - 规则增删    : RULES.md 尚未入库，本次是首次生成，无法给出增删基线"
fi

cat <<NEXT

下一步：
  1) 正式同步：cd engine && ./scripts/sync-bytebase.sh $REF
  2) 审阅 git diff（engine/RULES.md 的规则增删最关键）
  3) 对拍线上引擎：go run ./tools/checkdiff -old <线上引擎> -new <候选引擎> -meta-dsn '...'
  4) 构建镜像、同端口替换容器；旧 tag 留着回滚
NEXT
