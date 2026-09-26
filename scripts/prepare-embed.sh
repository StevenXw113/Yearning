#!/usr/bin/env bash
#
# 为后端的 go:embed 生成最小占位页。
#
# src/service/dist/ 被 .gitignore 忽略（属构建产物），因此干净 clone 后直接
# `go build ./...` 会失败于：pattern dist/*: no matching files found
# 先跑本脚本即可编译；已有的文件不会被覆盖（前端完整产物按 docs/DEVELOPMENT.md 第 3.1 节覆盖该目录）。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST="$ROOT/src/service/dist/index.html"
HTML='<!DOCTYPE html><html><head><meta charset="utf-8"><title>Yearning</title></head><body><div id="app"></div></body></html>'

mkdir -p "$(dirname "$DIST")"
if [ -f "$DIST" ]; then
	echo "保留已有文件: ${DIST#"$ROOT"/}"
else
	printf '%s\n' "$HTML" >"$DIST"
	echo "已生成占位页: ${DIST#"$ROOT"/}"
fi
