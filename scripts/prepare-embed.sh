#!/usr/bin/env bash
#
# 为后端的 go:embed 生成最小占位页。
#
# src/service/dist/ 与 src/service/chat/server/app/ 都被 .gitignore 忽略（属构建产物），
# 因此干净 clone 后直接 `go build ./...` 会失败于：
#   pattern dist/*: no matching files found / pattern chat/*: no matching files found
# 先跑本脚本即可编译；已有的文件不会被覆盖（前端完整产物按 docs/DEVELOPMENT.md 第 3.1 节覆盖 src/service/dist）。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST="$ROOT/src/service/dist/index.html"
CHAT="$ROOT/src/service/chat/server/app/index.html"
HTML='<!DOCTYPE html><html><head><meta charset="utf-8"><title>Yearning</title></head><body><div id="app"></div></body></html>'

mkdir -p "$(dirname "$DIST")" "$(dirname "$CHAT")"
for f in "$DIST" "$CHAT"; do
	if [ -f "$f" ]; then
		echo "保留已有文件: ${f#"$ROOT"/}"
	else
		printf '%s\n' "$HTML" >"$f"
		echo "已生成占位页: ${f#"$ROOT"/}"
	fi
done
