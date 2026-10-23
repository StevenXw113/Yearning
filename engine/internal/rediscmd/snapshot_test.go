package rediscmd

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestAffectedKeys(t *testing.T) {
	cases := []struct {
		line string
		want []string
	}{
		{"SET k v", []string{"k"}},
		{"SET k v EX 60", []string{"k"}},
		{"DEL a b c", []string{"a", "b", "c"}},
		{"UNLINK a b", []string{"a", "b"}},
		{"MSET k1 v1 k2 v2", []string{"k1", "k2"}},
		{"RENAME src dst", []string{"src", "dst"}},
		{"COPY src dst", []string{"src", "dst"}},
		{"LMOVE a b LEFT RIGHT", []string{"a", "b"}},
		{"BITOP AND dest s1 s2", []string{"dest", "s1", "s2"}},
		{"PFMERGE dst s1 s2", []string{"dst", "s1", "s2"}},
		{"EXPIRE k 60", []string{"k"}},
		{"PERSIST k", []string{"k"}},
		{"HSET h f v", []string{"h"}},
		{"LPUSH l a", []string{"l"}},
		{"ZADD z 1 m", []string{"z"}},
		{"SADD s m", []string{"s"}},
		{"GETDEL k", []string{"k"}},
		{"DEL a a b", []string{"a", "b"}}, // 去重
		{"GET k", nil},                    // 纯读不抓
		{"SCAN 0", nil},                   // 纯读不抓
		{"HGETALL h", nil},                // 纯读不抓
	}
	for _, c := range cases {
		args, err := Split(c.line)
		if err != nil {
			t.Fatalf("Split(%q): %v", c.line, err)
		}
		if got := AffectedKeys(args); !reflect.DeepEqual(got, c.want) {
			t.Errorf("AffectedKeys(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}

// Quote 与 Split 必须互逆：否则键名里有空格/引号时回滚会打偏
func TestQuoteSplitRoundTrip(t *testing.T) {
	keys := []string{
		"plain", "with space", `with"quote`, "with'quote", `with\backslash`,
		"tab\there", "new\nline", "", "中文键",
	}
	for _, k := range keys {
		q := Quote(k)
		args, err := Split("GET " + q)
		if err != nil {
			t.Errorf("Split(Quote(%q)) 报错: %v", k, err)
			continue
		}
		if len(args) != 2 || args[1] != k {
			t.Errorf("往返失败: %q → %q → %q", k, q, args)
		}
	}
}

func TestRollbackText(t *testing.T) {
	snaps := []Snapshot{
		{Key: "gone", Type: "none"}, // 原本不存在
		{Key: "s", Type: "string", Exists: true, TTLms: 3600000, Value: []string{"alice"}},
		{Key: "l", Type: "list", Exists: true, Value: []string{"a", "b"}},
		{Key: "h", Type: "hash", Exists: true, Value: []string{"name", "alice"}},
		{Key: "z", Type: "zset", Exists: true, Value: []string{"alice", "100"}},
		{Key: "st", Type: "set", Exists: true, Value: []string{"x", "y"}},
	}
	got := Rollback(snaps)
	want := []string{
		"DEL gone",
		"DEL s", "SET s alice", "PEXPIRE s 3600000",
		"DEL l", "RPUSH l a b",
		"DEL h", "HSET h name alice",
		"DEL z", "ZADD z 100 alice",
		"DEL st", "SADD st x y",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Rollback =\n%q\nwant\n%q", got, want)
	}
	// 生成的回滚命令必须自己也能通过校验（保证「重新提交即可回滚」）
	for _, line := range got {
		if _, _, err := Validate(line); err != nil {
			t.Errorf("回滚命令 %q 未通过校验: %v", line, err)
		}
	}
}

func TestBlockedOnUnsupportedType(t *testing.T) {
	// Unsupported 由 Capture 按类型填写，这里是手工构造快照
	if err := Blocked([]Snapshot{{Key: "s", Type: "stream", Exists: true, Unsupported: true}}); err == nil {
		t.Error("不支持的类型应被拦下（无法保证回滚）")
	}
	if err := Blocked([]Snapshot{{Key: "s", Type: "string", Exists: true}}); err != nil {
		t.Errorf("支持的类型不该被拦: %v", err)
	}
}

// ---------- 集成测试：需要真实实例 ----------

func testClient(t *testing.T) (*redis.Client, context.Context) {
	t.Helper()
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("未设置 REDIS_TEST_ADDR，跳过集成测试")
	}
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: os.Getenv("REDIS_TEST_PASS"),
		DB:       0,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(func() {
		cancel()
		_ = client.Close()
	})
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("连接 %s 失败: %v", addr, err)
	}
	return client, ctx
}

func do(t *testing.T, ctx context.Context, client *redis.Client, line string) {
	t.Helper()
	args, err := Split(line)
	if err != nil {
		t.Fatalf("Split(%q): %v", line, err)
	}
	iface := make([]interface{}, 0, len(args))
	for _, a := range args {
		iface = append(iface, a)
	}
	if err := client.Do(ctx, iface...).Err(); err != nil {
		t.Fatalf("执行 %q 失败: %v", line, err)
	}
}

// TestSnapshotRestoreIntegration 走真实实例验证核心承诺：
// 抓快照 → 改数据 → 用 Rollback 生成的回滚命令恢复 → 快照应与最初完全一致。
func TestSnapshotRestoreIntegration(t *testing.T) {
	client, ctx := testClient(t)
	prefix := "yrn:snap:test:"
	keys := []string{
		prefix + "str", prefix + "list", prefix + "hash", prefix + "set", prefix + "zset",
	}
	cleanup := func() {
		for _, k := range keys {
			_ = client.Del(ctx, k).Err()
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	// 造 5 种类型，string 带 TTL
	do(t, ctx, client, "SET "+prefix+"str alice")
	do(t, ctx, client, "PEXPIRE "+prefix+"str 3600000")
	do(t, ctx, client, "RPUSH "+prefix+"list a b c")
	do(t, ctx, client, "HSET "+prefix+"hash name alice age 30")
	do(t, ctx, client, "SADD "+prefix+"set x y z")
	do(t, ctx, client, "ZADD "+prefix+"zset 100 alice 90 bob")

	before, err := Capture(ctx, client, keys)
	if err != nil {
		t.Fatalf("抓快照失败: %v", err)
	}
	if err := Blocked(before); err != nil {
		t.Fatalf("快照被拦: %v", err)
	}

	// 破坏现场：覆盖、删除、追加
	do(t, ctx, client, "SET "+prefix+"str changed")
	do(t, ctx, client, "DEL "+prefix+"list")
	do(t, ctx, client, "HSET "+prefix+"hash name bob")
	do(t, ctx, client, "SADD "+prefix+"set extra")
	do(t, ctx, client, "ZADD "+prefix+"zset 55 carol")

	// 用回滚命令恢复
	for _, line := range Rollback(before) {
		do(t, ctx, client, line)
	}

	after, err := Capture(ctx, client, keys)
	if err != nil {
		t.Fatalf("恢复后抓快照失败: %v", err)
	}
	// TTL 是相对时间：抓快照 → 破坏 → 回滚本身要花掉几百毫秒到几秒，
	// 所以只要求「剩余时间不比原来长、且差在容差内」，其余字段必须完全一致。
	const ttlToleranceMs = 10000
	if len(before) != len(after) {
		t.Fatalf("快照数量不一致: before=%d after=%d", len(before), len(after))
	}
	for i := range before {
		b, a := before[i], after[i]
		bt, at := b.TTLms, a.TTLms
		b.TTLms, a.TTLms = 0, 0
		if !reflect.DeepEqual(b, a) {
			t.Errorf("键 %s 恢复后不一致:\n before=%+v\n after =%+v", before[i].Key, before[i], after[i])
			continue
		}
		switch {
		case bt <= 0 && at <= 0:
		case bt > 0 && at > 0 && at <= bt && bt-at < ttlToleranceMs:
		default:
			t.Errorf("键 %s 的 TTL 恢复得不对: before=%dms after=%dms", before[i].Key, bt, at)
		}
	}

	// 原本不存在的键：快照应记为 Exists=false，回滚后仍不存在
	absent := []string{prefix + "absent"}
	snap, err := Capture(ctx, client, absent)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap) != 1 || snap[0].Exists {
		t.Errorf("不存在的键应记为 Exists=false, got %+v", snap)
	}
	do(t, ctx, client, "SET "+prefix+"absent new")
	for _, line := range Rollback(snap) {
		do(t, ctx, client, line)
	}
	if n, _ := client.Exists(ctx, prefix+"absent").Result(); n != 0 {
		t.Error("回滚后新建的键应被删除")
	}
}
