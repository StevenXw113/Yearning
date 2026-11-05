package api

import "testing"

// 命令分档：语义只回答「读的是什么」，授权由 RedisPolicy 决定
func TestRedisSemantics(t *testing.T) {
	cases := map[string]RedisScope{
		"GET": ScopeData, "hgetall": ScopeData, "SCAN": ScopeData,
		"TYPE": ScopeMeta, "TTL": ScopeMeta, "DBSIZE": ScopeMeta,
		"SLOWLOG": ScopeDiag, "DUMP": ScopeDiag, "INFO": ScopeDiag, "COMMAND": ScopeDiag,
		"SET": ScopeAdmin, "FLUSHALL": ScopeAdmin, "CONFIG": ScopeAdmin,
		"JSON.SET": ScopeAdmin, // 未登记的模块命令按管理类处理（fail-closed）
	}
	for name, want := range cases {
		if got := RedisSemantics(name); got != want {
			t.Errorf("RedisSemantics(%q) = %v, want %v", name, got, want)
		}
	}
	for _, name := range []string{"GET", "TYPE", "INFO", "SLOWLOG"} {
		if !RedisIsReadCommand(name) {
			t.Errorf("%s 应算只读", name)
		}
	}
	if RedisIsReadCommand("SET") {
		t.Error("SET 不应算只读")
	}
}

// 查询页授权：诊断命令默认关；INFO 只放行 keyspace/replication/memory 三个 section
func TestRedisPolicyQueryAllowed(t *testing.T) {
	deny := RedisPolicy{}
	allow := RedisPolicy{AllowDiag: true}

	ok := func(p RedisPolicy, args ...string) bool {
		got, _ := p.QueryAllowed(args)
		return got
	}

	if !ok(deny, "GET", "k") {
		t.Error("GET 应放行")
	}
	if !ok(deny, "TYPE", "k") {
		t.Error("TYPE 应放行")
	}
	if ok(deny, "SET", "k", "v") {
		t.Error("SET 不应放行")
	}
	if ok(deny, "FLUSHALL") {
		t.Error("FLUSHALL 不应放行")
	}
	if ok(deny, "SLOWLOG", "GET") {
		t.Error("诊断命令默认应拒绝")
	}
	if !ok(allow, "SLOWLOG", "GET") {
		t.Error("开启诊断后 SLOWLOG 应放行")
	}

	// INFO 分节：只读 section 放行，全量或含敏感 section 拒绝
	if ok(deny, "INFO") {
		t.Error("INFO 不带 section 应拒绝")
	}
	if !ok(deny, "INFO", "keyspace") {
		t.Error("INFO keyspace 应放行")
	}
	if !ok(deny, "INFO", "keyspace", "memory") {
		t.Error("INFO 多个只读 section 应放行")
	}
	if ok(deny, "INFO", "server") {
		t.Error("INFO server 应拒绝")
	}
	if ok(deny, "INFO", "keyspace", "server") {
		t.Error("含敏感 section 时应整体拒绝")
	}
	if !ok(allow, "INFO", "server") {
		t.Error("开启诊断后 INFO server 应放行")
	}
}
