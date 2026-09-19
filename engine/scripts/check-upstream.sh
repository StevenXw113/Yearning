#!/usr/bin/env bash
#
# 检查 Bytebase 是否有比当前内化版本更新的 release。
#
# 用法:
#   ./scripts/check-upstream.sh          # 打印当前/最新并比较，返回退出码
#   ./scripts/check-upstream.sh --latest # 只打印上游最新版本（供 CI 解析 ref）
#   ./scripts/check-upstream.sh --quiet  # 不打印，只看退出码
#
# 退出码: 0=已是最新, 10=有新版本, 其他=查询失败。
#
# 用 git ls-remote 取 tag 而非 GitHub API：无频率限制，且与同步脚本同源。
set -euo pipefail

REPO="${BYTEBASE_REPO:-https://github.com/bytebase/bytebase}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
UPSTREAM="$ROOT/internal/bytebase/UPSTREAM"

MODE="compare"
case "${1:-}" in
	--latest) MODE="latest" ;;
	--quiet) MODE="quiet" ;;
	"") ;;
	*)
		echo "未知参数: $1" >&2
		exit 2
		;;
esac

# 取上游最新的 x.y.z 形式的 release tag（忽略非版本号 tag）。
latest="$(git ls-remote --tags --refs "$REPO" 2>/dev/null \
	| sed 's#.*refs/tags/##' \
	| grep -E '^[0-9]+\.[0-9]+\.[0-9]+$' \
	| sort -t. -k1,1n -k2,2n -k3,3n \
	| tail -1 || true)"

if [ -z "$latest" ]; then
	echo "无法获取上游标签，仓库: $REPO" >&2
	exit 1
fi

if [ "$MODE" = "latest" ]; then
	echo "$latest"
	exit 0
fi

current="$(awk '/^ref:/{print $2}' "$UPSTREAM" 2>/dev/null || true)"

if [ "$MODE" != "quiet" ]; then
	echo "当前内化版本: ${current:-未知}"
	echo "上游最新版本: $latest"
fi

if [ "$current" = "$latest" ]; then
	if [ "$MODE" != "quiet" ]; then echo "已是最新。"; fi
	exit 0
fi

if [ "$MODE" != "quiet" ]; then
	echo "有新版本可用，执行: cd engine && ./scripts/sync-bytebase.sh $latest"
fi
exit 10
