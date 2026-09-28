package personal

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestMongoRows(t *testing.T) {
	// find / aggregate：取 cursor.firstBatch
	raw := bson.M{"cursor": bson.M{"firstBatch": bson.A{
		bson.M{"_id": 1, "name": "a"},
		bson.M{"_id": 2, "name": "b"},
	}}, "ok": 1}
	rows := mongoRows(raw)
	if len(rows) != 2 || rows[1]["name"] != "b" {
		t.Fatalf("firstBatch 解析失败: %v", rows)
	}

	// count / distinct 等非游标命令：整个响应当一行
	rows = mongoRows(bson.M{"n": 5, "ok": 1})
	if len(rows) != 1 || rows[0]["n"] != 5 {
		t.Fatalf("标量响应应作为单行: %v", rows)
	}

	// 驱动也可能给 bson.D 形态
	rows = mongoRows(bson.M{"cursor": bson.D{{Key: "firstBatch", Value: bson.A{bson.D{{Key: "x", Value: 1}}}}}})
	if len(rows) != 1 || rows[0]["x"] != 1 {
		t.Fatalf("bson.D 形态解析失败: %v", rows)
	}
}

func TestMongoDisplay(t *testing.T) {
	id := primitive.NewObjectID()
	if got := mongoDisplay(id); got != id.Hex() {
		t.Errorf("ObjectID 应转 hex, got %v", got)
	}
	if got, ok := mongoDisplay(bson.M{"a": 1}).(string); !ok || !strings.Contains(got, `"a"`) {
		t.Errorf("嵌套文档应转 JSON 文本, got %v", got)
	}
	if got := mongoDisplay(int64(7)); got != int64(7) {
		t.Errorf("标量应原样返回, got %v", got)
	}
	// Mongo 的 Date 以 UTC 存储，展示按本地时区（东八区下同一时刻是 11:04:05）
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if got, want := mongoDisplay(primitive.NewDateTimeFromTime(ts)), ts.Local().Format("2006-01-02 15:04:05"); got != want {
		t.Errorf("时间应按本地时区格式化, got %v want %v", got, want)
	}
}

// TestRunMongoCommandRejectsWrites 查询页的执行入口必须挡住写命令。
// 这条测试走的是**真正被调用的那个函数**，且不需要连库：
// ValidateQuery 在解析之后、用上 client 之前就返回，所以传 nil client 是安全的。
func TestRunMongoCommandRejectsWrites(t *testing.T) {
	deny := map[string]string{
		`{"delete":"users","deletes":[{"q":{"age":1},"limit":0}]}`:            "只读",
		`{"update":"users","updates":[{"q":{"_id":1},"u":{"$set":{"a":2}}}]}`: "只读",
		`{"dropDatabase":1}`: "只读",
		`{"aggregate":"users","pipeline":[{"$out":"backup"}],"cursor":{}}`: "$out",
	}
	for q, want := range deny {
		_, err := runMongoCommand(nil, "yearning_demo", q, "")
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("runMongoCommand(%s) 应报含 %q 的错误, got %v", q, want, err)
		}
	}
	// 非法库名先于一切判定被拦下
	if _, err := runMongoCommand(nil, "bad;name", `{"find":"users"}`, ""); err == nil {
		t.Error("非法库名应报错")
	}
}

// TestMongoCommandIntegration 需要一个真实实例：
//
//	MONGO_TEST_URI='mongodb://user:pwd@host:port/?authSource=admin' MONGO_TEST_DB=admin \
//	  go test ./src/handler/personal/ -run MongoCommandIntegration -v
func TestMongoCommandIntegration(t *testing.T) {
	uri, dbName := os.Getenv("MONGO_TEST_URI"), os.Getenv("MONGO_TEST_DB")
	if uri == "" || dbName == "" {
		t.Skip("未设置 MONGO_TEST_URI / MONGO_TEST_DB，跳过集成测试")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Disconnect(ctx) }()

	// listCollections 是只读的非游标命令：验证 RunCommand → mongoRows → 表格契约这条链路
	q, err := runMongoCommand(client, dbName, `{"listCollections":1}`, "")
	if err != nil {
		t.Fatalf("执行命令失败: %v", err)
	}
	if len(q.Data) == 0 {
		t.Error("应至少返回一行")
	}
	var titles []string
	for _, f := range q.Field {
		titles = append(titles, f["title"].(string))
	}
	t.Logf("字段=%v 行数=%d", titles, len(q.Data))

	// 非法命令要报出可读原因，而不是 panic
	if _, err := runMongoCommand(client, dbName, `not-json`, ""); err == nil {
		t.Error("非法 JSON 应报错")
	}
}
