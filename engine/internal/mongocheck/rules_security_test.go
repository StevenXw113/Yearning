package mongocheck

import (
	"strings"
	"testing"
)

// securityCfg 只开本批次相关的开关
func securityCfg() Config {
	return Config{
		ForbidSystemCollection: true,
		ForbidAdminCommand:     true,
		ForbidImmutableID:      true,
		ForbidWhere:            true,
	}
}

func TestCheckSystemCollection(t *testing.T) {
	deny := []string{
		`{"update":"system.users","updates":[{"q":{"a":1},"u":{"$set":{"b":2}}}]}`,
		`{"drop":"system.profile"}`,
		`{"delete":"system.views","deletes":[{"q":{"a":1},"limit":0}]}`,
		`{"createIndexes":"system.js","indexes":[{"key":{"a":1},"name":"a_1"}]}`,
	}
	for _, q := range deny {
		_, findings, err := Check(q, securityCfg())
		if err != nil {
			t.Errorf("Check(%s) 不该报用法错误: %v", q, err)
			continue
		}
		if hit := Blocked(findings); hit == nil || !strings.Contains(hit.Message, "system") {
			t.Errorf("Check(%s) 应被 system.* 规则拦下, got %+v", q, findings)
		}
	}
	// 前缀相近但不同的集合名不能误伤
	allow := []string{
		`{"update":"systemic","updates":[{"q":{"a":1},"u":{"$set":{"b":2}}}]}`,
		`{"update":"mysystem.users","updates":[{"q":{"a":1},"u":{"$set":{"b":2}}}]}`,
		`{"update":"users","updates":[{"q":{"a":1},"u":{"$set":{"b":2}}}]}`,
	}
	for _, q := range allow {
		if _, findings, _ := Check(q, securityCfg()); Blocked(findings) != nil {
			t.Errorf("Check(%s) 不该被拦, got %+v", q, findings)
		}
	}
	// 开关关闭时不生效
	if _, findings, _ := Check(deny[0], Config{}); Blocked(findings) != nil {
		t.Error("开关关闭时不该命中")
	}
}

func TestCheckAdminCommand(t *testing.T) {
	deny := []string{
		`{"createUser":"x","pwd":"y"}`,
		`{"dropUser":"x"}`,
		`{"updateUser":"x","pwd":"y"}`,
		`{"grantRolesToUser":"x","roles":[]}`,
		`{"createRole":"r","privileges":[]}`,
		`{"dropAllRolesFromDatabase":1}`,
		`{"grantPrivilegesToRole":"r","privileges":[]}`,
	}
	for _, q := range deny {
		_, findings, err := Check(q, securityCfg())
		if err != nil {
			t.Errorf("Check(%s) 不该报用法错误（管理命令应按 DDL 审核，而不是「不是变更命令」）: %v", q, err)
			continue
		}
		if hit := Blocked(findings); hit == nil || hit.Rule != RuleAdminCommand {
			t.Errorf("Check(%s) 应命中 %s, got %+v", q, RuleAdminCommand, findings)
		}
	}
	// 分类修正：开关关闭时也不该报「不是变更命令」，它是变更命令而不是查询
	if _, _, err := Check(`{"createUser":"x"}`, Config{}); err != nil {
		t.Errorf("管理命令的分类应是变更命令, got %v", err)
	}
	// 普通变更命令不受影响
	if _, findings, _ := Check(`{"insert":"users","documents":[{"a":1}]}`, securityCfg()); Blocked(findings) != nil {
		t.Error("insert 不该被管理命令规则拦下")
	}
}

func TestCheckImmutableID(t *testing.T) {
	deny := []string{
		// $set 改 _id
		`{"update":"users","updates":[{"q":{"_id":1},"u":{"$set":{"_id":2}}}]}`,
		// $unset 删 _id
		`{"update":"users","updates":[{"q":{"_id":1},"u":{"$unset":{"_id":""}}}]}`,
		// $rename 把别的字段改成 _id
		`{"update":"users","updates":[{"q":{"_id":1},"u":{"$rename":{"name":"_id"}}}]}`,
		// 整体替换文档里带 _id
		`{"update":"users","updates":[{"q":{"_id":1},"u":{"_id":2,"name":"a"}}]}`,
		// 旧式写法（条件与更新在顶层）
		`{"update":"users","q":{"_id":1},"u":{"$set":{"_id":2}}}`,
	}
	for _, q := range deny {
		_, findings, err := Check(q, securityCfg())
		if err != nil {
			t.Errorf("Check(%s) 不该报用法错误: %v", q, err)
			continue
		}
		if hit := Blocked(findings); hit == nil || hit.Rule != RuleImmutableID {
			t.Errorf("Check(%s) 应命中 %s, got %+v", q, RuleImmutableID, findings)
		}
	}
	allow := []string{
		// $setOnInsert 里的 _id 是插入时的合法写法
		`{"update":"users","updates":[{"q":{"name":"a"},"u":{"$setOnInsert":{"_id":7}},"upsert":true}]}`,
		// insert 带 _id 合法
		`{"insert":"users","documents":[{"_id":1,"a":2}]}`,
		// filter 里用 _id 是常态
		`{"update":"users","updates":[{"q":{"_id":1},"u":{"$set":{"name":"b"}}}]}`,
		`{"delete":"users","deletes":[{"q":{"_id":1},"limit":1}]}`,
	}
	for _, q := range allow {
		_, findings, err := Check(q, securityCfg())
		if err != nil {
			t.Errorf("Check(%s) 不该报用法错误: %v", q, err)
			continue
		}
		if Blocked(findings) != nil {
			t.Errorf("Check(%s) 不该被拦, got %+v", q, findings)
		}
	}
}

func TestCheckServerScriptFamily(t *testing.T) {
	// $function / $accumulator 与 $where 等价（都能在服务端执行脚本），
	// 只拦 $where 等于留了后门
	deny := []string{
		`{"update":"users","updates":[{"q":{"$where":"1"},"u":{"$set":{"a":1}}}]}`,
		`{"update":"users","updates":[{"q":{"$expr":{"$function":{"body":"function(){}","args":[],"lang":"js"}}},"u":{"$set":{"a":1}}}]}`,
		`{"delete":"users","deletes":[{"q":{"x":{"$accumulator":{"init":"function(){}"}}},"limit":0}]}`,
	}
	for _, q := range deny {
		_, findings, err := Check(q, securityCfg())
		if err != nil {
			t.Errorf("Check(%s) 不该报用法错误: %v", q, err)
			continue
		}
		if hit := Blocked(findings); hit == nil || hit.Rule != RuleWhere {
			t.Errorf("Check(%s) 应命中 %s, got %+v", q, RuleWhere, findings)
		}
	}
}

// TestBulkWriteIsNotAServerCommand bulkWrite 是驱动层方法，不是服务端命令：
// 留在 dmlCommands 里会让它「能过审但执行不了」，应当报「不是变更命令」。
func TestBulkWriteIsNotAServerCommand(t *testing.T) {
	_, _, err := Check(`{"bulkWrite":1}`, Config{})
	if err == nil || !strings.Contains(err.Error(), "不是变更命令") {
		t.Errorf("bulkWrite 应报「不是变更命令」, got %v", err)
	}
}
