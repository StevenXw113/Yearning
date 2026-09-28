package mongodb

import (
	"strings"
	"testing"
)

// TestValidateQueryAllowsReadOnly 查询页只读白名单：正常的查询命令都要放行
func TestValidateQueryAllowsReadOnly(t *testing.T) {
	allow := []string{
		`{"find":"users"}`,
		`{"find":"users","filter":{"age":{"$gt":30}},"limit":20}`,
		`{"Find":"users"}`, // 命令名大小写不敏感
		`{"aggregate":"users","pipeline":[{"$match":{"a":1}}],"cursor":{}}`,
		`{"count":"users","query":{"a":1}}`,
		`{"distinct":"users","key":"age"}`,
		`{"listCollections":1}`,
		`{"listIndexes":"users"}`,
		`{"collStats":"users"}`,
		`{"dbStats":1}`,
		`{"explain":{"find":"users"}}`, // explain 只解释不执行
		`{"getMore":123,"collection":"users"}`,
		`{"ping":1}`,
		`{"hello":1}`,
	}
	for _, q := range allow {
		if err := ValidateQuery(cmd(q)); err != nil {
			t.Errorf("ValidateQuery(%s) 应放行, got %v", q, err)
		}
	}
}

// TestValidateQueryRejectsWrites 变更命令绝不允许从查询页执行——这是硬保底，不读规则集。
// 拿到查询权限的用户若能在查询页跑 update / delete / dropDatabase，
// 等于绕过了整个工单审批链路。
func TestValidateQueryRejectsWrites(t *testing.T) {
	deny := map[string]string{
		`{"update":"users","updates":[{"q":{"_id":1},"u":{"$set":{"a":2}}}]}`: "只读",
		`{"delete":"users","deletes":[{"q":{"age":1},"limit":0}]}`:            "只读",
		`{"insert":"users","documents":[{"a":1}]}`:                           "只读",
		`{"findAndModify":"users","query":{"_id":1},"update":{"$set":{"a":1}}}`: "只读",
		`{"createIndexes":"users","indexes":[{"key":{"a":1},"name":"a_1"}]}`:  "只读",
		`{"drop":"users"}`:          "只读",
		`{"dropDatabase":1}`:        "只读",
		`{"createUser":"x","pwd":"y"}`: "只读",
		// 这两个"看起来像查询"，实际上能在服务端执行脚本/写数据，白名单天然挡住
		`{"mapReduce":"users","map":"function(){}","reduce":"function(){}","out":"r"}`: "只读",
		`{"eval":"db.users.drop()"}`: "只读",
		// 不认识的命令一律拒绝：Mongo 命令空间是开放的，猜不出来就说明不该在查询页跑
		`{"foobar":1}`: "只读",
	}
	for q, want := range deny {
		err := ValidateQuery(cmd(q))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ValidateQuery(%s) 应报含 %q 的错误, got %v", q, want, err)
		}
	}
}

// TestValidateQueryRejectsAggregateWriteStages
// aggregate 在白名单里，但 $out / $merge 会把结果写回集合——白名单挡不住，
// 必须在 pipeline 上单独判定，否则查询页多了一条"用聚合写数据"的旁路。
func TestValidateQueryRejectsAggregateWriteStages(t *testing.T) {
	deny := []string{
		`{"aggregate":"users","pipeline":[{"$match":{}},{"$out":"backup"}],"cursor":{}}`,
		`{"aggregate":"users","pipeline":[{"$merge":{"into":"backup"}}],"cursor":{}}`,
		`{"aggregate":"users","pipeline":[{"$match":{"a":1}},{"$Merge":"backup"}],"cursor":{}}`,
	}
	for _, q := range deny {
		err := ValidateQuery(cmd(q))
		if err == nil || !strings.Contains(err.Error(), "$out") {
			t.Errorf("ValidateQuery(%s) 应拦住 $out/$merge, got %v", q, err)
		}
	}
	// 普通 pipeline 不受影响
	ok := `{"aggregate":"users","pipeline":[{"$match":{"a":1}},{"$group":{"_id":"$a"}}],"cursor":{}}`
	if err := ValidateQuery(cmd(ok)); err != nil {
		t.Errorf("普通聚合应放行, got %v", err)
	}
}

func TestValidateQueryEmptyCommand(t *testing.T) {
	if err := ValidateQuery(nil); err == nil {
		t.Error("空命令应报错")
	}
}
