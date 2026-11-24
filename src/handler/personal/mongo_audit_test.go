package personal

import (
	"errors"
	"strings"
	"testing"

	"Yearning-go/src/model"
)

func TestQueryAuditUnavailable(t *testing.T) {
	err := errors.New("engine down")

	note, blocked := queryAuditUnavailable(false, "规则集加载失败", err)
	if blocked != nil || !strings.Contains(note, "未做查询侧规则审核") {
		t.Fatalf("非生产环境应留痕放行: note=%q blocked=%v", note, blocked)
	}

	_, blocked = queryAuditUnavailable(true, "规则集加载失败", err)
	if blocked == nil || !strings.Contains(blocked.Error(), "已阻断查询") {
		t.Fatalf("生产环境应 fail-closed: %v", blocked)
	}

	if note, blocked = queryAuditUnavailable(true, "规则集加载失败", nil); blocked != nil || note != "" {
		t.Fatalf("无错误不应返回阻断: note=%q blocked=%v", note, blocked)
	}
}

func TestMarkMongoDBAApproval(t *testing.T) {
	order := &model.CoreSqlOrder{SQL: `{"drop":"users"}`}
	if err := markMongoDBAApproval(order, model.DBTypeMongoDB); err != nil {
		t.Fatalf("markMongoDBAApproval(drop) error = %v", err)
	}
	if !order.RequiresDBAApproval || order.DBAApprovalStatus != "pending" {
		t.Fatalf("dropCollection 应标记 DBA 特批: %#v", order)
	}

	normal := &model.CoreSqlOrder{SQL: `{"createIndexes":"users","indexes":[{"name":"x_1","key":{"x":1}}]}`}
	if err := markMongoDBAApproval(normal, model.DBTypeMongoDB); err != nil {
		t.Fatalf("markMongoDBAApproval(createIndexes) error = %v", err)
	}
	if normal.RequiresDBAApproval || normal.DBAApprovalStatus != "" {
		t.Fatalf("普通命令不应标记 DBA 特批: %#v", normal)
	}

	mysql := &model.CoreSqlOrder{SQL: "drop table t", RequiresDBAApproval: true}
	if err := markMongoDBAApproval(mysql, 0); err != nil || !mysql.RequiresDBAApproval {
		t.Fatalf("非 Mongo 命令不应执行 Mongo 预检: err=%v order=%#v", err, mysql)
	}

	invalid := &model.CoreSqlOrder{SQL: "bad"}
	if err := markMongoDBAApproval(invalid, model.DBTypeMongoDB); err == nil {
		t.Fatal("非法 Mongo 命令应报错")
	}
}
