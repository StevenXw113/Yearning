package personal

import (
	"context"
	"os"
	"strings"
	"testing"

	"Yearning-go/src/model"

	"github.com/redis/go-redis/v9"
)

// 结果转换：Redis 的返回值形态比 SQL / Mongo 都杂，逐个形态钉住
func TestRedisQueryShapes(t *testing.T) {
	// 标量字符串（GET）
	q := redisQuery("GET", []string{"GET", "k"}, "alice", "")
	if len(q.Field) != 1 || q.Field[0]["title"] != "value" {
		t.Fatalf("标量应只有 value 列: %v", q.Field)
	}
	if len(q.Data) != 1 || q.Data[0]["value"] != "alice" {
		t.Fatalf("标量值不对: %v", q.Data)
	}
	// MySQL 模板对首列做 fixed 处理，Redis 侧保持一致
	if q.Field[0]["fixed"] != "left" {
		t.Errorf("首列应 fixed=left: %v", q.Field[0])
	}
	if q.Field[0]["width"] != 200 {
		t.Errorf("列宽应与 SQL 侧一致(200): %v", q.Field[0]["width"])
	}

	// 标量整型（DBSIZE）
	q = redisQuery("DBSIZE", []string{"DBSIZE"}, int64(6), "")
	if q.Data[0]["value"] != "6" {
		t.Errorf("整型应转字符串: %v", q.Data[0])
	}

	// nil：键不存在
	q = redisQuery("GET", []string{"GET", "missing"}, nil, "")
	if !strings.Contains(q.Data[0]["value"].(string), "nil") {
		t.Errorf("nil 应有可读展示: %v", q.Data[0])
	}

	// 一维数组（LRANGE）：index / value 两列
	q = redisQuery("LRANGE", []string{"LRANGE", "l", "0", "-1"}, []interface{}{"a", "b"}, "")
	if len(q.Field) != 2 || q.Field[0]["title"] != "index" || q.Field[1]["title"] != "value" {
		t.Fatalf("数组应 index/value 两列: %v", q.Field)
	}
	if len(q.Data) != 2 || q.Data[1]["value"] != "b" {
		t.Fatalf("数组行不对: %v", q.Data)
	}

	// HGETALL：返回的是 field,value 交替，应转成两列（而不是当成普通数组）
	q = redisQuery("HGETALL", []string{"HGETALL", "h"}, []interface{}{"name", "alice", "age", "30"}, "")
	if q.Field[0]["title"] != "field" || q.Field[1]["title"] != "value" {
		t.Fatalf("HGETALL 应 field/value 两列: %v", q.Field)
	}
	if len(q.Data) != 2 || q.Data[0]["field"] != "name" || q.Data[1]["value"] != "30" {
		t.Fatalf("HGETALL 行不对: %v", q.Data)
	}

	// HGETALL 的真实返回形态：go-redis 的 Do 给的是 map[interface{}]interface{}
	q = redisQuery("HGETALL", []string{"HGETALL", "h"},
		map[interface{}]interface{}{"name": "alice", "age": "30"}, "")
	if q.Field[0]["title"] != "field" || q.Field[1]["title"] != "value" {
		t.Fatalf("HGETALL(map[interface{}]) 应 field/value 两列: %v", q.Field)
	}
	if len(q.Data) != 2 || q.Data[0]["field"] != "age" || q.Data[1]["value"] != "alice" {
		t.Fatalf("HGETALL(map[interface{}]) 行不对（应按字段名排序）: %v", q.Data)
	}

	// map[string]string 这条路径也保留
	q = redisQuery("HGETALL", []string{"HGETALL", "h"}, map[string]string{"name": "alice", "age": "30"}, "")
	if q.Field[0]["title"] != "field" || q.Field[1]["title"] != "value" {
		t.Fatalf("HGETALL(map) 应 field/value 两列: %v", q.Field)
	}
	if len(q.Data) != 2 || q.Data[0]["field"] != "age" || q.Data[1]["value"] != "alice" {
		t.Fatalf("HGETALL(map) 行不对（应按字段名排序）: %v", q.Data)
	}

	// SCAN：二维数组（游标 + 键列表），内层用 JSON 展示
	q = redisQuery("SCAN", []string{"SCAN", "0"}, []interface{}{"0", []interface{}{"k1", "k2"}}, "")
	if len(q.Data) != 2 {
		t.Fatalf("SCAN 应两行: %v", q.Data)
	}
	if !strings.Contains(q.Data[1]["value"].(string), "k1") {
		t.Errorf("内层数组应 JSON 展示: %v", q.Data[1])
	}
}

// 脱敏按**键名**判定：Redis 的值没有「字段名」，只能以键为粒度
func TestRedisQueryInsulate(t *testing.T) {
	q := redisQuery("GET", []string{"GET", "user:email"}, "a@b.com", "email")
	if q.Data[0]["value"] == "a@b.com" {
		t.Error("键名命中脱敏词表时值应被替换")
	}

	// 未命中的键不动
	q = redisQuery("GET", []string{"GET", "user:1"}, "alice", "email")
	if q.Data[0]["value"] != "alice" {
		t.Errorf("未命中不应替换: %v", q.Data[0])
	}

	// 词表大小写不敏感（与 SQL 侧 lowerList 的口径一致）
	q = redisQuery("GET", []string{"GET", "user:EMAIL"}, "a@b.com", "Email")
	if q.Data[0]["value"] == "a@b.com" {
		t.Error("脱敏词表应大小写不敏感")
	}
}

// 超长值不往前端发：与 SQL 侧对 BLOB 的处理一致
func TestRedisDisplayBlobGuard(t *testing.T) {
	big := strings.Repeat("x", BUF+10)
	got := redisDisplay(big)
	if got == big {
		t.Error("超过 BUF 的值应替换为不可展示提示")
	}
	if redisDisplay("short") != "short" {
		t.Error("正常长度不该被替换")
	}
}

// 查询页只能执行只读命令：写命令必须在建连之前就被拒绝
// （getClient 传 nil —— 如果校验没拦住，这里会 panic 而不是断言失败）
func TestRunRedisCommandRejectsWrites(t *testing.T) {
	nilClient := func(int) (*redis.Client, error) { return nil, nil }
	for _, cmd := range []string{"SET k v", "DEL k", "FLUSHALL", "KEYS *", "HSET h f v"} {
		if _, err := runRedisCommand(nilClient, "db0", cmd, ""); err == nil {
			t.Errorf("runRedisCommand(%q) 应被拒绝", cmd)
		}
	}
	// 空命令同样是用法错误
	if _, err := runRedisCommand(nilClient, "db0", "  ", ""); err == nil {
		t.Error("空命令应报错")
	}
}

func TestRedisParseDBForQuerySchema(t *testing.T) {
	// 库名从 WS 传来，解析成整数后参与建连（不是拼进命令），
	// 所以非法值只会落到 db0，不存在注入面
	cases := map[string]int{"db0": 0, "db15": 15, "db3": 3, "xx": 0, "": 0, "db-1": 0}
	for in, want := range cases {
		if got := model.RedisParseDB(in); got != want {
			t.Errorf("RedisParseDB(%q) = %d, want %d", in, got, want)
		}
	}
}

// 集成测试：需要真实实例，未设置 REDIS_TEST_ADDR 时跳过
func TestRedisQueryIntegration(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("未设置 REDIS_TEST_ADDR，跳过集成测试")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, Password: os.Getenv("REDIS_TEST_PASS")})
	defer func() { _ = client.Close() }()
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("连接 %s 失败: %v", addr, err)
	}
	// 只读命令走通，且结果能被转成表格
	for _, cmd := range []string{"PING", "DBSIZE", "SCAN 0 MATCH yrn:test:* COUNT 10"} {
		q, err := runRedisCommand(func(int) (*redis.Client, error) { return client, nil }, "db0", cmd, "")
		if err != nil {
			t.Errorf("runRedisCommand(%q) 失败: %v", cmd, err)
			continue
		}
		if len(q.Field) == 0 {
			t.Errorf("runRedisCommand(%q) 应产出列定义", cmd)
		}
	}
	// 写命令即使能连上也必须被拦
	if _, err := runRedisCommand(func(int) (*redis.Client, error) { return client, nil }, "db0", "SET yrn:test:x 1", ""); err == nil {
		t.Error("写命令应被拒绝")
	}
}
