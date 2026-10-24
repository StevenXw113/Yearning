package server

import (
	"strings"
	"testing"

	enginev1 "engine/gen/engine/v1"
)

// TestSplitDorisStatements 切分器对引号与注释的处理：注释里的分号属于注释文本，
// 切开会拼出残缺语句（check_doris 的逐条路由与 exec_doris 的逐条执行共用此切分）。
func TestSplitDorisStatements(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want []string
	}{
		{"普通多条", "SELECT 1; SELECT 2", []string{"SELECT 1", "SELECT 2"}},
		{"引号内分号", "SELECT ';' ; SELECT 2", []string{"SELECT ';'", "SELECT 2"}},
		{"行注释里的分号", "SELECT 1 -- note; more\nFROM t; SELECT 2", []string{"SELECT 1 \nFROM t", "SELECT 2"}},
		{"井号注释", "SELECT 1 # c;x\n; SELECT 2", []string{"SELECT 1", "SELECT 2"}},
		{"块注释里的分号", "SELECT 1 /* a;b */; SELECT 2", []string{"SELECT 1", "SELECT 2"}},
		{"未闭合块注释", "SELECT 1 /* a;b", []string{"SELECT 1"}},
		{"--x 不是注释", "SELECT 1--2; SELECT 3", []string{"SELECT 1--2", "SELECT 3"}},
		{"单引号双写", "SELECT 'a'';b'; SELECT 2", []string{"SELECT 'a'';b'", "SELECT 2"}},
		{"反引号双写", "SELECT `a``;b`; SELECT 2", []string{"SELECT `a``;b`", "SELECT 2"}},
		{"字符串反斜杠转义", `SELECT 'a\';b'; SELECT 2`, []string{`SELECT 'a\';b'`, "SELECT 2"}},
		{"空与纯注释", "  ; -- nothing\n", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := splitDorisStatements(c.sql)
			if len(got) != len(c.want) {
				t.Fatalf("切分数不符: got %d %q, want %d %q", len(got), got, len(c.want), c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("第 %d 条不符: got %q, want %q", i, got[i], c.want[i])
				}
			}
		})
	}
}

// TestCheckDorisCommentSemicolon 注释分号不切分的端到端回归：
// 审核输出的记录数与语句真实条数一致，且不误报语法错误。
func TestCheckDorisCommentSemicolon(t *testing.T) {
	sql := "CREATE TABLE t1 (id INT) DISTRIBUTED BY HASH(id) BUCKETS 8 -- k; v\n;"
	rep, err := testEngine().Check(t.Context(), &enginev1.CheckRequest{
		Sql:    sql,
		Rule:   &enginev1.AuditRole{},
		Source: &enginev1.DataSource{Kind: "doris"},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(rep.Records) != 1 {
		t.Fatalf("注释里的分号不应切出第二条语句: %+v", rep.Records)
	}
	if strings.Contains(rep.Records[0].Error, "语法错误") {
		t.Fatalf("不应报语法错误: %+v", rep.Records[0])
	}
}
