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
	// 标量字符串（GET）：键名 + 值两列，只看 value 不知道查的是哪个键
	q := redisQuery("GET", []string{"GET", "k"}, "alice", "")
	if len(q.Field) != 2 || q.Field[0]["title"] != "key" || q.Field[1]["title"] != "value" {
		t.Fatalf("GET 应 key/value 两列: %v", q.Field)
	}
	if len(q.Data) != 1 || q.Data[0]["key"] != "k" || q.Data[0]["value"] != "alice" {
		t.Fatalf("标量值不对: %v", q.Data)
	}
	// MySQL 模板对首列做 fixed 处理，Redis 侧保持一致
	if q.Field[0]["fixed"] != "left" {
		t.Errorf("首列应 fixed=left: %v", q.Field[0])
	}
	if q.Field[0]["width"] != 200 {
		t.Errorf("列宽应与 SQL 侧一致(200): %v", q.Field[0]["width"])
	}
	// 无键命令保持单列
	q = redisQuery("DBSIZE", []string{"DBSIZE"}, int64(2), "")
	if len(q.Field) != 1 || q.Field[0]["title"] != "value" {
		t.Errorf("DBSIZE 应只有 value 列: %v", q.Field)
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

	// SCAN：游标 + 键列表，按键逐行展开（不再是 index/value 两行 + JSON）
	q = redisQuery("SCAN", []string{"SCAN", "0"}, []interface{}{"17", []interface{}{"k1", "k2"}}, "")
	if q.Field[0]["title"] != "cursor" || q.Field[1]["title"] != "key" {
		t.Fatalf("SCAN 应 cursor/key 两列: %v", q.Field)
	}
	if len(q.Data) != 2 || q.Data[0]["key"] != "k1" || q.Data[1]["key"] != "k2" || q.Data[0]["cursor"] != "17" {
		t.Fatalf("SCAN 应按键逐行展开: %v", q.Data)
	}

	// MGET：值与参数里的键一一对应
	q = redisQuery("MGET", []string{"MGET", "a", "b"}, []interface{}{"1", nil}, "")
	if q.Field[0]["title"] != "key" || q.Field[1]["title"] != "value" {
		t.Fatalf("MGET 应 key/value 两列: %v", q.Field)
	}
	if len(q.Data) != 2 || q.Data[0]["key"] != "a" || q.Data[0]["value"] != "1" || q.Data[1]["key"] != "b" {
		t.Fatalf("MGET 行不对: %v", q.Data)
	}
}

// 「查看数据」按类型选命令：Redis 没有一条命令能读所有类型，选错就是 WRONGTYPE
func TestRedisPreviewSQL(t *testing.T) {
	cases := map[string]string{
		"string": "GET k",
		"hash":   "HGETALL k",
		"list":   "LRANGE k 0 99",
		"set":    "SSCAN k 0 COUNT 100",
		"zset":   "ZRANGE k 0 99 WITHSCORES",
		"stream": "XRANGE k - + COUNT 20",
		"none":   "",
	}
	for typ, want := range cases {
		if got := redisPreviewSQL(typ, "k"); got != want {
			t.Errorf("类型 %s 的命令 = %q, want %q", typ, got, want)
		}
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

// 查询页只能执行只读命令：写命令必须在借客户端、建连之前就被拒绝
func TestPrepareRedisCommandRejectsWrites(t *testing.T) {
	for _, cmd := range []string{"SET k v", "DEL k", "FLUSHALL", "KEYS *", "HSET h f v"} {
		if _, _, err := prepareRedisCommand("db0", cmd, false); err == nil {
			t.Errorf("prepareRedisCommand(%q) 应被拒绝", cmd)
		}
	}
	// 空命令同样是用法错误
	if _, _, err := prepareRedisCommand("db0", "  ", false); err == nil {
		t.Error("空命令应报错")
	}
}

func TestRedisParseDBForQuerySchema(t *testing.T) {
	// 库名从 WS 传来，解析成整数后参与建连（不是拼进命令），不存在注入面；
	// 但非法值必须报错——静默回落 db0 会让命令跑到错误的库上。
	cases := map[string]int{"db0": 0, "db15": 15, "db3": 3, "DB3": 3, "": 0}
	for in, want := range cases {
		got, err := model.RedisParseDB(in)
		if err != nil {
			t.Errorf("RedisParseDB(%q) 不应报错: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("RedisParseDB(%q) = %d, want %d", in, got, want)
		}
	}
	for _, bad := range []string{"xx", "db-1", "dbx", "db1x"} {
		if _, err := model.RedisParseDB(bad); err == nil {
			t.Errorf("RedisParseDB(%q) 应报错", bad)
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
		_, args, perr := prepareRedisCommand("db0", cmd, false)
		if perr != nil {
			t.Errorf("prepareRedisCommand(%q) 失败: %v", cmd, perr)
			continue
		}
		q, err := runRedisCommand(client, args, "")
		if err != nil {
			t.Errorf("runRedisCommand(%q) 失败: %v", cmd, err)
			continue
		}
		if len(q.Field) == 0 {
			t.Errorf("runRedisCommand(%q) 应产出列定义", cmd)
		}
	}
	// 写命令即使能连上也必须被拦
	if _, _, err := prepareRedisCommand("db0", "SET yrn:test:x 1", false); err == nil {
		t.Error("写命令应被拒绝")
	}
}

// 键名含空格/引号时拼命令必须转义，否则切词会把键名拆错
func TestRedisPreviewSQLQuotesKey(t *testing.T) {
	if got := redisPreviewSQL("string", "a b"); got != `GET "a b"` {
		t.Errorf("含空格的键名应加引号: %q", got)
	}
	if got := redisPreviewSQL("hash", `a"b`); got != `HGETALL "a\"b"` {
		t.Errorf("含引号的键名应转义: %q", got)
	}
	if got := redisPreviewSQL("hash", "app:h"); got != "HGETALL app:h" {
		t.Errorf("普通键名不应加引号: %q", got)
	}
}

// 查询页执行授权：诊断命令默认拦，INFO 只读分节放行，开关打开后放行
func TestPrepareRedisCommandDiagPolicy(t *testing.T) {
	for _, cmd := range []string{"SLOWLOG GET 1", "DUMP demo", "INFO", "LATENCY HISTORY x"} {
		if _, _, err := prepareRedisCommand("db0", cmd, false); err == nil {
			t.Errorf("%q 应在策略层被拦截", cmd)
		}
	}
	// INFO 的只读 section 不算诊断
	if _, _, err := prepareRedisCommand("db0", "INFO keyspace", false); err != nil {
		t.Errorf("INFO keyspace 应放行，实际: %v", err)
	}
	// 打开诊断开关后放行
	if _, _, err := prepareRedisCommand("db0", "SLOWLOG GET 1", true); err != nil {
		t.Errorf("开启诊断后 SLOWLOG 应放行，实际: %v", err)
	}
}
