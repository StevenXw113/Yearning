package server

import (
	"strings"
	"testing"
)

func TestDeleteByImage(t *testing.T) {
	cols := []string{"id", "name"}

	// 有主键：按主键精确定位，无需 LIMIT
	if got := deleteByImage("demo", "t", cols, []any{int64(7), "a"}, []string{"id"}); got != "DELETE FROM `demo`.`t` WHERE `id` = 7;" {
		t.Fatalf("有主键时回滚语句不符预期: %s", got)
	}
	// 无主键：全列 NULL 安全匹配 + LIMIT 1，避免连带删掉相同的行
	got := deleteByImage("demo", "t", cols, []any{int64(7), "a"}, nil)
	if !strings.Contains(got, "`id` <=> 7") || !strings.Contains(got, "`name` <=> 'a'") || !strings.HasSuffix(got, "LIMIT 1;") {
		t.Fatalf("无主键时回滚语句不符预期: %s", got)
	}
	// 含单引号的值走十六进制字面量（NO_BACKSLASH_ESCAPES 下安全）：无主键时全列进 WHERE
	got = deleteByImage("demo", "t", cols, []any{int64(7), "a'b"}, nil)
	if !strings.Contains(got, "0x612762") {
		t.Fatalf("含单引号的值应渲染为十六进制: %s", got)
	}
	// 带反引号的标识符双写，不闭合引用
	got = deleteByImage("de`mo", "t", []string{"id"}, []any{int64(7)}, []string{"id"})
	if !strings.Contains(got, "`de``mo`.`t`") {
		t.Fatalf("含反引号的标识符应双写: %s", got)
	}
}

// TestNextBinlogServerID 回归：复制 server_id 进程内必须唯一。此前用纳秒时间戳
// 取模，同源库并发工单撞号后 MySQL 踢掉先建立的复制连接，回滚静默丢失。
func TestNextBinlogServerID(t *testing.T) {
	seen := map[uint32]struct{}{}
	prev := nextBinlogServerID()
	for i := 0; i < 1000; i++ {
		id := nextBinlogServerID()
		if _, dup := seen[id]; dup {
			t.Fatalf("server_id 撞号: %d", id)
		}
		seen[id] = struct{}{}
	}
	if next := nextBinlogServerID(); next == prev {
		t.Fatal("连续分配的 server_id 不应相同")
	}
}

func TestInsertByImage(t *testing.T) {
	got := insertByImage("demo", "t", []string{"id", "name", "note"}, []any{int64(1), "a", nil})
	want := "INSERT INTO `demo`.`t` (`id`, `name`, `note`) VALUES (1, 'a', NULL);"
	if got != want {
		t.Fatalf("回滚语句不符预期: %s", got)
	}
}

func TestUpdateByImage(t *testing.T) {
	cols := []string{"id", "name", "note"}

	// 有主键且主键未改：只还原变化的列，用 after 的主键定位当前行
	got := updateByImage("demo", "t", cols, []any{int64(1), "old", "keep"}, []any{int64(1), "new", "keep"}, []string{"id"})
	want := "UPDATE `demo`.`t` SET `name` = 'old' WHERE `id` = 1;"
	if got != want {
		t.Fatalf("回滚语句不符预期: %s", got)
	}

	// 主键被改：主键也要还原，定位仍用 after 镜像
	got = updateByImage("demo", "t", cols, []any{int64(1), "a", "c"}, []any{int64(2), "a", "c"}, []string{"id"})
	want = "UPDATE `demo`.`t` SET `id` = 1 WHERE `id` = 2;"
	if got != want {
		t.Fatalf("主键变更回滚语句不符预期: %s", got)
	}

	// 无变化（如 UPDATE 命中 0 行不会产生事件，这里防御性处理）
	if got := updateByImage("demo", "t", cols, []any{int64(1), "a", "c"}, []any{int64(1), "a", "c"}, []string{"id"}); got != "" {
		t.Fatalf("无变化的行不应生成语句: %s", got)
	}

	// 无主键：全列匹配 + LIMIT 1
	got = updateByImage("demo", "t", cols, []any{int64(1), "old", "c"}, []any{int64(1), "new", "c"}, nil)
	if !strings.Contains(got, "SET `name` = 'old'") || !strings.Contains(got, "`name` <=> 'new'") || !strings.HasSuffix(got, "LIMIT 1;") {
		t.Fatalf("无主键回滚语句不符预期: %s", got)
	}
}

func TestTargetTables(t *testing.T) {
	stmts := []string{
		"UPDATE a1 u JOIN a2 ON u.id=a2.id SET u.x=1",
		"INSERT INTO other.tb (id) SELECT id FROM a3",
		"DELETE u FROM a4 u WHERE u.id=1",
	}
	got := targetTables("demo", stmts)
	for _, want := range []string{"demo.a1", "demo.a2", "other.tb", "demo.a4"} {
		if !got[want] {
			t.Fatalf("缺少目标表 %s，实际为 %v", want, got)
		}
	}
	// DELETE u FROM a4 u 里的 u 会被解析成表引用，由调用方用存在性校验剔除
	if !got["demo.u"] {
		t.Logf("别名未被解析成表引用（%v），行为随解析器版本可能变化", got)
	}
}
