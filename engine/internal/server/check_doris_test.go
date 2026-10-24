package server

import (
	"context"
	"strings"
	"testing"

	enginev1 "engine/gen/engine/v1"
)

// dorisReq 构造 Doris 审核请求。
func dorisReq(sql string) *enginev1.CheckRequest {
	return &enginev1.CheckRequest{
		Sql:    sql,
		Rule:   &enginev1.AuditRole{},
		Source: &enginev1.DataSource{Kind: "doris"},
	}
}

// TestCheckDorisMixedOrderRoutesPerStatement 回归：Doris 审核此前按「整单首句是否
// CREATE TABLE」路由，混合工单（DML + Doris 建表 DDL）会被 MySQL 拆分器整单误报
// 语法错误。现在逐条语句路由，与 exec_doris 的切分一致。
func TestCheckDorisMixedOrderRoutesPerStatement(t *testing.T) {
	sql := "INSERT INTO t1 (id) VALUES (1);\n" +
		"CREATE TABLE `db`.`t2` (id INT) DISTRIBUTED BY HASH(id) BUCKETS 10 PROPERTIES(\"replication_num\" = \"1\")"
	rep, err := testEngine().Check(context.Background(), dorisReq(sql))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !rep.Ok {
		t.Fatalf("混合工单应整体通过: %+v", rep)
	}
	if len(rep.Records) != 2 {
		t.Fatalf("DML + DDL 应逐条出记录（2 条），got %d: %+v", len(rep.Records), rep.Records)
	}
	if strings.Contains(rep.Records[0].Error, "语法错误") {
		t.Fatalf("DML 部分不应报语法错误: %+v", rep.Records[0])
	}
	if !strings.Contains(strings.ToUpper(rep.Records[1].Sql), "CREATE TABLE") {
		t.Fatalf("第二条记录应是建表语句: %+v", rep.Records[1])
	}
	if rep.Records[1].Level != 0 || rep.Records[1].Error != "" {
		t.Fatalf("符合规范的建表应通过: %+v", rep.Records[1])
	}
}

// TestCheckDorisMultiCreateOneRecordEach 回归：多条建表语句此前被并成一条 Record
// （与 exec 侧逐条执行明细对不上），现在一条语句一条记录。
func TestCheckDorisMultiCreateOneRecordEach(t *testing.T) {
	sql := "CREATE TABLE t1 (id INT) DISTRIBUTED BY HASH(id) BUCKETS 8;" +
		"CREATE TABLE t2 (id INT) DISTRIBUTED BY HASH(id) BUCKETS 8"
	rep, err := testEngine().Check(context.Background(), dorisReq(sql))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(rep.Records) != 2 {
		t.Fatalf("两条建表应逐条出记录（2 条），got %d: %+v", len(rep.Records), rep.Records)
	}
	for i, r := range rep.Records {
		if !strings.Contains(r.Sql, "t"+string(rune('1'+i))) {
			t.Fatalf("记录 %d 的语句不对: %+v", i, r)
		}
	}
}

// TestCheckDorisCreateWithoutDistribution 建表规则仍生效（逐条路由后不丢 dorischeck）。
func TestCheckDorisCreateWithoutDistribution(t *testing.T) {
	// 规则未开启：无 DISTRIBUTED BY 也通过
	rep, _ := testEngine().Check(context.Background(), dorisReq("CREATE TABLE t1 (id INT)"))
	if len(rep.Records) != 1 || rep.Records[0].Level != 0 {
		t.Fatalf("规则未开启时应通过: %+v", rep.Records)
	}
	// 规则开启：无 DISTRIBUTED BY 拦截
	rep2, _ := testEngine().Check(context.Background(), &enginev1.CheckRequest{
		Sql:    "CREATE TABLE t1 (id INT)",
		Rule:   &enginev1.AuditRole{DorisRequireDistribution: true},
		Source: &enginev1.DataSource{Kind: "doris"},
	})
	if len(rep2.Records) != 1 || rep2.Records[0].Level != 1 || !strings.Contains(rep2.Records[0].Error, "DISTRIBUTED") {
		t.Fatalf("开启规则后无 DISTRIBUTED BY 应拦截: %+v", rep2.Records)
	}
}
