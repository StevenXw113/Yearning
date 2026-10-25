#!/usr/bin/env bash
#
# 从 Bytebase 官方仓库同步 SQL 解析器与 MySQL 审核规则到 engine/internal/bytebase。
#
# 用法:
#   ./scripts/sync-bytebase.sh              # 重放「当前内化版本」（从 UPSTREAM 读取）
#   ./scripts/sync-bytebase.sh 3.23.0       # 指定 tag / 分支 / commit
#   ./scripts/sync-bytebase.sh 3.23.0 <repo-url>
#
# 流程：稀疏拉取上游 → 复制子集 → 改写 import 前缀 → 重放裁剪
#       → go build / go test → 同步主程序里的内化版本号。
# 同步后务必先审阅 git diff，再提交。
set -euo pipefail

REPO="${2:-https://github.com/bytebase/bytebase}"
SRC_IMPORT_PREFIX="github.com/bytebase/bytebase/backend/"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)" # = engine/
DST="$ROOT/internal/bytebase"

# 无参数时重放当前内化版本：直接从 UPSTREAM 读，避免脚本里再养一个版本号
# （曾经脚本默认写死旧版本，不带参数一跑就把内化代码降级了）。
DEFAULT_REF="$(awk '/^ref:/{print $2}' "$DST/UPSTREAM" 2>/dev/null || true)"
REF="${1:-${DEFAULT_REF:-}}"
if [ -z "$REF" ]; then
	echo "用法: $0 <bytebase-ref>（首次同步必须显式指定版本）" >&2
	exit 2
fi
WORK="$(mktemp -d)"
DONE=0

# 中途失败时把 internal/bytebase 还原成同步前的内容：
# 否则旧目录已被 mv 走、又随 $WORK 一起清掉，仓库只剩一份半成品。
cleanup() {
	if [ "$DONE" != "1" ] && [ -d "$WORK/old" ]; then
		echo "同步失败：回滚 internal/bytebase 到同步前内容" >&2
		[ -d "$DST" ] && mv "$DST" "$WORK/failed"
		mv "$WORK/old" "$DST"
	fi
	rm -rf "$WORK"
}
trap cleanup EXIT

echo "==> 拉取上游 $REPO@$REF"
if ! git clone --quiet --filter=blob:none --no-checkout --depth 1 --branch "$REF" "$REPO" "$WORK/src" 2>/dev/null; then
	echo "    $REF 不是分支/标签，退回全量 ref 拉取"
	rm -rf "$WORK/src"
	git clone --quiet --filter=blob:none --no-checkout "$REPO" "$WORK/src"
fi
git -C "$WORK/src" sparse-checkout init --cone
git -C "$WORK/src" sparse-checkout set \
	backend/plugin/advisor \
	backend/plugin/parser \
	backend/common \
	backend/store/model \
	backend/generated-go/store \
	backend/plugin/db/transaction \
	backend/plugin/db/mysql
git -C "$WORK/src" checkout --quiet "$REF"
SRC="$WORK/src/backend"
COMMIT="$(git -C "$WORK/src" rev-parse --short HEAD)"

# README.md / LICENSE 是内化时补的来源与许可说明，上游没有，重建前先留存
KEEP="$WORK/keep"
mkdir -p "$KEEP"
for f in README.md LICENSE; do
	if [ -f "$DST/$f" ]; then
		cp "$DST/$f" "$KEEP/$f"
	fi
done

# 旧内容整体移出后再重建：避免误删，且移动不受批量删除限制。
# 同步成功后 $WORK 会被清理，无需手工处理。
if [ -d "$DST" ]; then
	mv "$DST" "$WORK/old"
fi
mkdir -p "$DST/plugin/parser" "$DST/store" "$DST/generated-go" "$DST/plugin/db/transaction"

echo "==> 复制子集（排除测试、测试数据与非 MySQL/PG 方言）"
# bigquery 是 3.24.0 新增的 advisor（依赖 parser/bigquery + parser/googlesql，
# 两者都不在子集里），且没有任何保留代码 import 它，整目录排除。
# test/ 是 advisor 的测试 fixture（YAML，~16MB）：内化后 *_test.go 已排除，
# fixture 无消费者，不拷。
rsync -a --exclude='._*' --exclude='.DS_Store' --exclude='*_test.go' --exclude='test-data/' \
	--exclude='test/' \
	--exclude='utils_for_tests.go' \
	--exclude='bigquery/' \
	--exclude='mssql/' --exclude='oceanbase/' --exclude='oracle/' \
	--exclude='redshift/' --exclude='snowflake/' --exclude='tidb/' \
	"$SRC/plugin/advisor/" "$DST/plugin/advisor/"
# mariadb 是 3.24.0 起 parser/mysql 的依赖（statement_type / resource_change /
# mysql.go 都 import 它），必须一起拷；parser/bigquery 与 parser/googlesql 只有
# 被排除的 bigquery advisor 用到，不拷。tokenizer 零引用（trim 里也不再处理），不拷。
for d in base mariadb mysql pg; do
	rsync -a --exclude='._*' --exclude='.DS_Store' --exclude='*_test.go' --exclude='test-data/' \
		"$SRC/plugin/parser/$d/" "$DST/plugin/parser/$d/"
done
# plugin/db/transaction：3.23.0 起 plugin/parser/base 依赖它（仅 1 个文件、只用到 database/sql）。
# 只取这一个子包 —— backend/plugin/db 下还有各语言驱动的完整实现（含 AWS SDK 等重依赖）。
rsync -a --exclude='._*' --exclude='.DS_Store' --exclude='*_test.go' \
	"$SRC/plugin/db/transaction/" "$DST/plugin/db/transaction/"

# plugin/db/mysql：3.23.0 起有两条审核规则（INSERT / STATEMENT 影响行数上限）用它的
# ExplainJSON 与 EstimateAffectedRows 做 EXPLAIN 估算。只拷 stats.go 这一个文件：
# 它接收标准库 *sql.DB，依赖（omni/mysql/ast、mysqlparser、storepb、common/log）都在
# 内化范围内；该包其余文件是完整的 MySQL 驱动实现（连接、SSL、AWS RDS IAM、dump/restore），
# 会连带 aws-sdk-go-v2、golang.org/x/crypto/ssh 等重依赖。
mkdir -p "$DST/plugin/db/mysql"
cp "$SRC/plugin/db/mysql/stats.go" "$DST/plugin/db/mysql/stats.go"
rsync -a --exclude='._*' --exclude='.DS_Store' --exclude='*_test.go' \
	"$SRC/store/model/" "$DST/store/model/"
rsync -a --exclude='._*' --exclude='.DS_Store' --exclude='*_test.go' \
	"$SRC/generated-go/store/" "$DST/generated-go/store/"
rsync -a --exclude='._*' --exclude='.DS_Store' --exclude='*_test.go' \
	--exclude='cel*.go' --exclude='context.go' --exclude='retry.go' \
	--exclude='permission/' --exclude='testcontainer/' --exclude='yamltest/' --exclude='qb/' \
	"$SRC/common/" "$DST/common/"

echo "==> 改写 import 前缀"
# 用 perl 而非 sed -i：macOS(BSD) 与 CI(Ubuntu/GNU) 的 sed -i 语义不一致。
find "$DST" -name '*.go' ! -name '._*' -exec perl -pi -e "s{\Q$SRC_IMPORT_PREFIX\E}{engine/internal/bytebase/}g" {} +
if grep -rq "$SRC_IMPORT_PREFIX" --include='*.go' "$DST"; then
	echo "错误: 仍存在未改写的 import: $SRC_IMPORT_PREFIX" >&2
	exit 1
fi

echo "==> 重放裁剪"
python3 "$ROOT/scripts/trim-bytebase.py" "$DST"


echo "==> gofmt"
gofmt -w "$DST"

for f in README.md LICENSE; do
	if [ -f "$KEEP/$f" ]; then
		cp "$KEEP/$f" "$DST/$f"
	fi
done
cat >"$DST/UPSTREAM" <<EOF
repo: $REPO
ref:  $REF
commit: $COMMIT
synced: $(date -u +%Y-%m-%dT%H:%M:%SZ)
EOF

echo "==> 同步依赖（上游可能引入新依赖）"
cd "$ROOT"
if ! go mod tidy >/tmp/sync-tidy.log 2>&1; then
	# 离线/代理不通时 tidy 会失败，但若上游没引入新依赖，构建仍然能过，故只告警
	echo "    go mod tidy 未成功（可能是网络/代理问题），继续尝试构建："
	tail -3 /tmp/sync-tidy.log | sed 's/^/    /'
fi

echo "==> 校验 go build / go test"
go build ./...
# 上游可能新增规则：先把它们按「未启用」登记进 RULES.md，否则 archguard 的清单测试会失败，
# 而失败会让本脚本回滚、升级卡在半路。人工审阅 RULES.md 的 diff 决定是否接到开关上即可。
UPDATE_RULES_MANIFEST=1 go test ./internal/archguard -run TestRulesManifest >/dev/null
go test ./...
DONE=1 # 到这里才认为同步成功，失败时 cleanup 会还原旧目录

# 主程序「检测上游更新」把内化版本写死在 src/handler/manage/roles/upstream.go。
# 同步成功后一并更新，省掉一个必须手工同步的地方（TestBytebaseRefMatchesUpstream 会兜底）。
# 放在 DONE=1 之后：同步失败时不动主程序的文件，避免两边版本号各说各话。
APP_UPSTREAM="$ROOT/../src/handler/manage/roles/upstream.go"
if [ -f "$APP_UPSTREAM" ]; then
	if [[ "$REF" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
		perl -pi -e "s/^const bytebaseRef = .*/const bytebaseRef = \"$REF\"/" "$APP_UPSTREAM"
		echo "==> 已同步主程序内化版本号: $APP_UPSTREAM -> $REF"
	else
		echo "==> 提示: $REF 不是 x.y.z 版本号，跳过更新主程序内化版本号"
	fi
fi

echo
echo "同步完成: $REPO@$REF ($COMMIT)"
git -C "$ROOT/.." --no-pager diff --stat -- engine/internal/bytebase engine/go.mod engine/go.sum || true
grep -m1 '^## 已启用' "$ROOT/RULES.md" 2>/dev/null || true
grep -m1 '^## 未启用' "$ROOT/RULES.md" 2>/dev/null || true
cat <<'NEXT'

下一步：
  1) 审阅 git diff —— 重点看 engine/RULES.md（上游规则增删）与 engine/go.mod
  2) 对拍线上引擎与候选引擎（行为 diff 门禁）：
     go run ./tools/checkdiff -old <线上引擎> -new <候选引擎> -meta-dsn '...' -limit 200
  3) 构建镜像并同端口替换容器（见 engine/README.md「Docker 升级与回滚」），保留旧 tag 以便回滚
  4) 需要把新规则接到 Yearning 开关上时，按 engine/README.md「把上游规则接到开关上」的清单改
NEXT
