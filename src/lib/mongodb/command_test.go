package mongodb

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	driver "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func cmd(s string) bson.D {
	var d bson.D
	if err := bson.UnmarshalExtJSON([]byte(s), false, &d); err != nil {
		panic(err)
	}
	return d
}

func TestClassify(t *testing.T) {
	cases := map[string]int{
		`{"find":"users"}`:                         KindQuery,
		`{"count":"users"}`:                        KindQuery,
		`{"insert":"users","documents":[{"a":1}]}`: KindDML,
		`{"update":"users","updates":[{"q":{"_id":1},"u":{"$set":{"a":2}}}]}`: KindDML,
		`{"delete":"users","deletes":[{"q":{"a":1},"limit":0}]}`:              KindDML,
		`{"createIndexes":"users","indexes":[{"key":{"a":1},"name":"a_1"}]}`:  KindDDL,
		`{"drop":"users"}`: KindDDL,
		// 管理命令按结构变更处理（不是查询）：否则提交时报「不是变更命令」
		`{"createUser":"x","pwd":"y"}`: KindDDL,
		// bulkWrite 是驱动层方法，不是服务端命令
		`{"bulkWrite":1}`: KindQuery,
	}
	for q, want := range cases {
		if got := Classify(cmd(q)); got != want {
			t.Errorf("Classify(%s) = %d, want %d", q, got, want)
		}
	}
}

func TestValidate(t *testing.T) {
	allow := []string{
		`{"update":"users","updates":[{"q":{"_id":1},"u":{"$set":{"a":2}}}]}`,
		`{"delete":"users","deletes":[{"q":{"age":{"$gt":30}},"limit":0}]}`,
		`{"findAndModify":"users","query":{"_id":1},"update":{"$set":{"a":1}}}`,
		`{"insert":"users","documents":[{"a":1}]}`,
		`{"createIndexes":"users","indexes":[{"key":{"a":1},"name":"a_1"}]}`,
		// 管理命令是变更命令：硬保底放行，拦不拦由规则集（MongoForbidAdminCommand）决定
		`{"createUser":"x","pwd":"y"}`,
	}
	for _, q := range allow {
		if _, err := Validate(cmd(q)); err != nil {
			t.Errorf("Validate(%s) 应通过, got %v", q, err)
		}
	}

	// 硬保底：这些即使规则集没配也要拦（可配置的审核在引擎侧，见 engine/internal/mongocheck）
	deny := map[string]string{
		`{"update":"users","updates":[{"q":{},"u":{"$set":{"a":1}}}]}`: "非空 filter",
		`{"update":"users","updates":[{"u":{"$set":{"a":1}}}]}`:        "非空 filter",
		`{"delete":"users","deletes":[{"limit":0}]}`:                   "非空 filter",
		`{"update":"users","q":{},"u":{"$set":{"a":1}}}`:               "非空 filter",
		`{"findAndModify":"users","update":{"$set":{"a":1}}}`:          "非空 filter",
		`{"dropDatabase":1}`:       "删除数据库",
		`{"eval":"while(true){}"}`: "服务端脚本",
		`{"delete":"users","deletes":[{"q":{"$where":"1"},"limit":0}]}`: "$where",
		// $function / $accumulator 与 $where 等价，硬保底也要拦（与引擎规则同一组）
		`{"update":"users","updates":[{"q":{"$function":{"body":"f"}},"u":{"$set":{"a":1}}}]}`: "$function",
		`{"update":"users","updates":[{"q":{"$accumulator":{"init":"f"}},"u":{"$set":{"a":1}}}]}`: "$accumulator",
		`{"find":"users"}`: "不是变更命令",
	}
	for q, want := range deny {
		_, err := Validate(cmd(q))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Validate(%s) 应报含 %q 的错误, got %v", q, want, err)
		}
	}
}

func TestRollbackGeneration(t *testing.T) {
	before := []bson.M{{"_id": 1, "name": "alice", "age": 30}}
	out := updateRollback("users", before)
	if len(out) != 1 {
		t.Fatalf("update 应生成 1 条回滚, got %v", out)
	}
	for _, want := range []string{`"update":"users"`, `"upsert":true`, `"name":"alice"`} {
		if !strings.Contains(out[0], want) {
			t.Errorf("回滚命令应含 %s, got %s", want, out[0])
		}
	}

	out = deleteRollback("users", before)
	if len(out) != 1 || !strings.Contains(out[0], `"insert":"users"`) {
		t.Errorf("delete 回滚应为重新插入, got %v", out)
	}

	out = insertRollback("users", cmd(`{"insert":"users","documents":[{"_id":1},{"_id":2}]}`))
	if len(out) != 1 || !strings.Contains(out[0], `"$in"`) {
		t.Errorf("insert 回滚应按 _id 删除, got %v", out)
	}
	// 文档未显式给 _id（服务端生成）时无法回滚
	if out := insertRollback("users", cmd(`{"insert":"users","documents":[{"a":1}]}`)); out != nil {
		t.Errorf("无 _id 时不应生成回滚, got %v", out)
	}

	// 无前镜像时不应生成空的回滚语句
	if out := updateRollback("users", nil); out != nil {
		t.Errorf("无前镜像应返回空, got %v", out)
	}
}

func TestAffected(t *testing.T) {
	if got := affected("update", bson.M{"n": 3, "nModified": 2}); got != 3 {
		t.Errorf("affected(update) = %d, want 3", got)
	}
	if got := affected("insert", bson.M{"n": bson.M{"inserted": 5}}); got != 5 {
		t.Errorf("affected(insert) = %d, want 5", got)
	}
}

// TestExecuteRollbackIntegration 需要真实实例：
//
//	MONGO_TEST_URI='mongodb://user:pwd@host:port/?authSource=admin' MONGO_TEST_DB=admin \
//	  go test ./src/lib/mongodb/ -run Integration -v
func TestExecuteRollbackIntegration(t *testing.T) {
	uri, dbName := os.Getenv("MONGO_TEST_URI"), os.Getenv("MONGO_TEST_DB")
	if uri == "" || dbName == "" {
		t.Skip("未设置 MONGO_TEST_URI / MONGO_TEST_DB，跳过集成测试")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := driver.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Disconnect(ctx) }()

	coll := "yearning_it_tmp"
	c := client.Database(dbName).Collection(coll)
	_ = c.Drop(ctx)
	defer func() { _ = c.Drop(ctx) }()

	if _, err := c.InsertMany(ctx, []interface{}{
		bson.M{"_id": 1, "name": "alice", "age": 30},
		bson.M{"_id": 2, "name": "bob", "age": 25},
	}); err != nil {
		t.Fatal(err)
	}

	// 1) update：改一条，再用生成的回滚命令撤销
	out, err := Execute(ctx, client, dbName, cmd(`{"update":"yearning_it_tmp","updates":[{"q":{"_id":1},"u":{"$set":{"age":99}}}]}`), 0)
	if err != nil {
		t.Fatalf("update 执行失败: %v", err)
	}
	if out.Affected != 1 || len(out.Rollback) != 1 {
		t.Fatalf("update 结果异常: %+v", out)
	}
	var after bson.M
	_ = c.FindOne(ctx, bson.M{"_id": 1}).Decode(&after)
	if after["age"] != int32(99) {
		t.Fatalf("update 未生效: %v", after["age"])
	}
	if err := rollbackAll(ctx, client, dbName, out.Rollback); err != nil {
		t.Fatalf("回滚失败: %v", err)
	}
	_ = c.FindOne(ctx, bson.M{"_id": 1}).Decode(&after)
	if after["age"] != int32(30) {
		t.Errorf("回滚后应恢复为 30, got %v", after["age"])
	}

	// 2) delete：删一条再回滚，文档应原样回来
	out, err = Execute(ctx, client, dbName, cmd(`{"delete":"yearning_it_tmp","deletes":[{"q":{"_id":2},"limit":1}]}`), 0)
	if err != nil {
		t.Fatalf("delete 执行失败: %v", err)
	}
	if err := rollbackAll(ctx, client, dbName, out.Rollback); err != nil {
		t.Fatalf("delete 回滚失败: %v", err)
	}
	var back bson.M
	if err := c.FindOne(ctx, bson.M{"_id": 2}).Decode(&back); err != nil {
		t.Fatalf("回滚后文档应存在: %v", err)
	}
	if back["name"] != "bob" {
		t.Errorf("回滚后内容应一致, got %v", back)
	}

	// 3) insert：插入再回滚，应被删掉
	out, err = Execute(ctx, client, dbName, cmd(`{"insert":"yearning_it_tmp","documents":[{"_id":3,"name":"carol"}]}`), 0)
	if err != nil {
		t.Fatalf("insert 执行失败: %v", err)
	}
	if err := rollbackAll(ctx, client, dbName, out.Rollback); err != nil {
		t.Fatalf("insert 回滚失败: %v", err)
	}
	if err := c.FindOne(ctx, bson.M{"_id": 3}).Err(); err == nil {
		t.Error("回滚后插入的文档应被删除")
	}
}

// rollbackAll 顺序执行回滚命令（回滚语句就是普通变更命令）
func rollbackAll(ctx context.Context, client *driver.Client, db string, cmds []string) error {
	for _, s := range cmds {
		var d bson.D
		if err := bson.UnmarshalExtJSON([]byte(s), false, &d); err != nil {
			return err
		}
		if err := client.Database(db).RunCommand(ctx, d).Err(); err != nil {
			return err
		}
	}
	return nil
}
