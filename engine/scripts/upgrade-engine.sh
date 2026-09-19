#!/usr/bin/env bash
#
# 一键升级引擎：把上游 Bytebase 的最新规则同步进 engine/internal/bytebase，
# 生成新的规则清单，并给出重新部署引擎的命令。
#
# 用法（在 engine/ 目录下执行）：
#   ./scripts/upgrade-engine.sh                # 自动检测上游最新版本并升级
#   ./scripts/upgrade-engine.sh 3.23.0         # 升到指定版本 / 分支 / commit
#   ./scripts/upgrade-engine.sh 3.23.0 --preview   # 只预演，不改动工作区
#   SKIP_DOCKER_BUILD=1 ./scripts/upgrade-engine.sh   # 不构建镜像，只改源码
#
# 执行完后需要你做的：审阅 git diff → 提交 → 重新部署引擎（脚本会打印命令）。
#
# 退出码：0=已是最新或升级成功；1=升级失败（已自动回滚，工作区保持原样）；2=用法/环境错误。
set -euo pipefail

SCRIPTS="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPTS/.." && pwd)"   # engine/
REPO_ROOT="$(cd "$ROOT/.." && pwd)"

REF=""
PREVIEW="${PREVIEW:-0}"
for arg in "$@"; do
	case "$arg" in
	--preview) PREVIEW=1 ;;
	-h | --help)
		sed -n '2,14p' "${BASH_SOURCE[0]}"
		exit 0
		;;
	-*)
		echo "未知参数: $arg" >&2
		exit 2
		;;
	*) REF="$arg" ;;
	esac
done
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# ---- 1. 决定目标版本：没给参数就查上游最新 release ----
if [ -z "$REF" ]; then
	echo "==> 未指定版本，查询上游最新 release"
	REF="$(bash "$SCRIPTS/check-upstream.sh" --latest)"
	echo "    上游最新: $REF"
fi

CURRENT="$(awk '/^ref:/{print $2}' "$ROOT/internal/bytebase/UPSTREAM" 2>/dev/null || echo 未知)"
if [ "$REF" = "$CURRENT" ]; then
	echo
	echo "当前内化版本已是 ${CURRENT}，无需升级。"
	echo "（若你确定要重放同步，直接执行: ./scripts/sync-bytebase.sh ${REF}）"
	exit 0
fi
echo "==> 准备升级: $CURRENT -> $REF"

# ---- 2. 记录同步前的规则清单，便于事后对比增删 ----
RULES_BEFORE="$WORK/rules_before.txt"
grep -o '^- `[A-Z0-9_]*`' "$ROOT/RULES.md" 2>/dev/null | tr -d '`-' | tr -d ' ' | sort >"$RULES_BEFORE" || true

# ---- 3. 预演模式：不动工作区，只报告 ----
if [ "${PREVIEW:-0}" = "1" ]; then
	exec bash "$SCRIPTS/preview-upgrade.sh" "$REF"
fi

# ---- 4. 正式同步（复制子集 → 改写 import → 重放裁剪 → go mod tidy → 构建/测试 → 登记新规则）----
if ! bash "$SCRIPTS/sync-bytebase.sh" "$REF"; then
	echo
	echo "升级失败：internal/bytebase 已回滚到同步前内容，工作区保持原样。" >&2
	echo "处理办法见上方报错；再跑一次本脚本即可重试。" >&2
	exit 1
fi

# ---- 5. 报告规则增删 ----
RULES_AFTER="$WORK/rules_after.txt"
grep -o '^- `[A-Z0-9_]*`' "$ROOT/RULES.md" 2>/dev/null | tr -d '`-' | tr -d ' ' | sort >"$RULES_AFTER" || true
ADDED="$(comm -13 "$RULES_BEFORE" "$RULES_AFTER" | tr '\n' ' ' || true)"
GONE="$(comm -23 "$RULES_BEFORE" "$RULES_AFTER" | tr '\n' ' ' || true)"

echo
echo "==== 升级结果 ===="
echo "版本      : $CURRENT -> $REF"
echo "规则清单  : $(grep -m1 '^## 已启用' "$ROOT/RULES.md" | sed 's/^## //') / $(grep -m1 '^## 未启用' "$ROOT/RULES.md" | sed 's/^## //')"
if [ -n "$ADDED" ]; then
	echo "新增规则  : $ADDED"
	echo "            （默认「未启用」；要启用见 engine/README.md「把上游规则接到开关上」）"
else
	echo "新增规则  : 无"
fi
if [ -n "$GONE" ]; then
	echo "上游移除  : $GONE"
	echo "            检查 rules.go 是否还引用它们（archguard 测试会报）"
fi
echo "源码改动  : 见 git status / git diff（engine/internal/bytebase、engine/RULES.md、engine/go.mod）"

# ---- 6. 构建镜像（可选）----
IMAGE="yearning-engine:${REF}"
BUILT=0
if [ "${SKIP_DOCKER_BUILD:-0}" != "1" ] && command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
	echo
	echo "==> 构建镜像 $IMAGE"
	if docker build -t "$IMAGE" "$ROOT"; then
		BUILT=1
	else
		echo "    镜像构建失败，可稍后手动重试（不影响源码已经同步好的状态）" >&2
	fi
else
	echo
	echo "==> 跳过镜像构建（未检测到可用的 docker，或 SKIP_DOCKER_BUILD=1）"
fi

cat <<NEXT

==== 接下来要做的 ====
  1) 审阅改动并提交
       git -C $REPO_ROOT status --short
       git -C $REPO_ROOT diff
       git -C $REPO_ROOT add -A && git -C $REPO_ROOT commit -m "chore(engine): 内化 Bytebase $REF"

  2) 上线前对拍线上引擎与候选引擎（审核行为是否变化）
       cd $REPO_ROOT && go run ./tools/checkdiff \\
         -old <线上引擎 host:13307> -new <候选引擎 host:port> \\
         -meta-dsn '<元数据库 DSN>' -limit 200

  3) 重新部署引擎（同端口替换容器；旧 tag 留着回滚）
NEXT

if [ "$BUILT" = "1" ]; then
	echo "       docker rm -f yearning-engine"
	echo "       docker run -d --name yearning-engine --restart always -p 13307:13307 $IMAGE"
else
	echo "       docker build -t $IMAGE $ROOT"
	echo "       docker rm -f yearning-engine"
	echo "       docker run -d --name yearning-engine --restart always -p 13307:13307 $IMAGE"
fi
cat <<'NEXT'
       # 回滚：把镜像 tag 换回上一版即可（引擎无状态，秒级）

  4) 页面「检测上游更新」核对版本号已变为新版本
NEXT
