package audit

import (
	"Yearning-go/src/i18n"
	"Yearning-go/src/model"
	"strings"
	"testing"
)

// isAuditor 必须是整名精确匹配：原实现的子串匹配（strings.Contains(拼接串, user)）
// 会让用户名恰好是他人审批人子串的人越权通过审批校验。
func TestIsAuditor(t *testing.T) {
	cases := []struct {
		name     string
		auditors []string
		user     string
		want     bool
	}{
		{"命中", []string{"alice", "bob"}, "bob", true},
		{"未命中", []string{"alice", "bob"}, "carol", false},
		{"用户名是审批人前缀", []string{"liming"}, "li", false},
		{"用户名是审批人子串", []string{"liming"}, "min", false},
		{"空格容错", []string{" alice "}, "alice", true},
		{"空审批人列表", nil, "alice", false},
		{"占位文案不匹配真实用户", []string{"提交人"}, "alice", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isAuditor(c.auditors, c.user); got != c.want {
				t.Fatalf("isAuditor(%v, %q) = %v, want %v", c.auditors, c.user, got, c.want)
			}
		})
	}
}

func rec(sql string) model.CoreSqlRecord { return model.CoreSqlRecord{SQL: sql, State: "已执行"} }

// remainingSQL 决定失败重试执行什么，多执行/漏执行都是事故：
// 已执行语句必须按序在原文中定位并排除，定位不到必须报错而不是猜。
func TestRemainingSQL(t *testing.T) {
	i18n.MakeBuild(i18n.CN)
	cases := []struct {
		name     string
		sql      string
		executed []model.CoreSqlRecord
		want     string
		wantErr  bool
	}{
		{
			name: "全量重试（无已执行记录）",
			sql:  "update a set x=1;\nupdate b set y=2;",
			want: "update a set x=1;\nupdate b set y=2;",
		},
		{
			name:     "跳过已执行前缀",
			sql:      "update a set x=1;\nupdate b set y=2;\nupdate c set z=3;",
			executed: []model.CoreSqlRecord{rec("update a set x=1")},
			want:     "update b set y=2;\nupdate c set z=3;",
		},
		{
			name: "多轮执行累积后取剩余",
			sql:  "update a set x=1; update b set y=2; update c set z=3;",
			executed: []model.CoreSqlRecord{
				rec("update a set x=1"), // 第一次执行到第一条后失败
				rec("update b set y=2"), // 重试后又成功一条
			},
			want: "update c set z=3;",
		},
		{
			name:     "同一语句出现两次时按顺序定位",
			sql:      "update a set x=1; update a set x=2;",
			executed: []model.CoreSqlRecord{rec("update a set x=1")},
			want:     "update a set x=2;",
		},
		{
			name:     "全部已执行报无可重试",
			sql:      "update a set x=1;",
			executed: []model.CoreSqlRecord{rec("update a set x=1")},
			wantErr:  true,
		},
		{
			name:     "记录与原文不匹配时拒绝",
			sql:      "update a set x=1;",
			executed: []model.CoreSqlRecord{rec("delete from b where id=2")},
			wantErr:  true,
		},
		{
			name:     "空记录文本拒绝",
			sql:      "update a set x=1;",
			executed: []model.CoreSqlRecord{rec("")},
			wantErr:  true,
		},
		{
			name: "Mongo/Redis 整段命令失败后全量重试",
			sql:  `db.user.updateMany({"age":{$gt:18}},{$set:{"status":1}})`,
			want: `db.user.updateMany({"age":{$gt:18}},{$set:{"status":1}})`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := remainingSQL(c.sql, c.executed)
			if c.wantErr {
				if err == nil {
					t.Fatalf("remainingSQL(%q, %v) 期望报错, got %q", c.sql, c.executed, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("remainingSQL(%q, %v) 意外报错: %v", c.sql, c.executed, err)
			}
			if got != c.want {
				t.Fatalf("remainingSQL = %q, want %q", got, c.want)
			}
		})
	}
	// 错误信息可读（非空），便于直接回给前端
	if _, err := remainingSQL("update a set x=1;", []model.CoreSqlRecord{rec("update a set x=1")}); err == nil || strings.TrimSpace(err.Error()) == "" {
		t.Fatalf("全部已执行的错误信息不应为空")
	}
}

func frec(sql string) model.CoreSqlRecord {
	return model.CoreSqlRecord{SQL: sql, State: "执行失败"}
}

// remainingSQLAfter 决定「跳过失败语句继续」执行什么：失败语句视为已人工处理，
// 重复记录（先失败后重试成功）要能被容错跳过，真正对不上的记录必须拒绝。
func TestRemainingSQLAfter(t *testing.T) {
	i18n.MakeBuild(i18n.CN)
	cases := []struct {
		name    string
		sql     string
		records []model.CoreSqlRecord
		want    string
		wantErr bool
	}{
		{
			name:    "首条失败后跳过继续",
			sql:     "update a set x=1;\nupdate b set y=2;",
			records: []model.CoreSqlRecord{frec("update a set x=1")},
			want:    "update b set y=2;",
		},
		{
			name: "多轮尝试：失败→重试成功→再失败，跳过最后失败继续",
			sql:  "update a set x=1; update b set y=2; update c set z=3; update d set w=4;",
			records: []model.CoreSqlRecord{
				frec("update a set x=1"), // 第一次尝试失败于 a
				rec("update a set x=1"),  // 重试 a 成功
				rec("update b set y=2"),  // b 成功
				frec("update c set z=3"), // c 失败
			},
			want: "update d set w=4;",
		},
		{
			name:    "失败的是最后一条：无可继续",
			sql:     "update a set x=1; update b set y=2;",
			records: []model.CoreSqlRecord{rec("update a set x=1"), frec("update b set y=2")},
			wantErr: true,
		},
		{
			name:    "记录与原文不匹配拒绝",
			sql:     "update a set x=1;",
			records: []model.CoreSqlRecord{frec("delete from b where id=2")},
			wantErr: true,
		},
		{
			name:    "同一语句文本出现两次时按序定位",
			sql:     "update a set x=1; update a set x=1;",
			records: []model.CoreSqlRecord{rec("update a set x=1"), frec("update a set x=1")},
			wantErr: true, // 两条同文本都已消费，无可继续
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := remainingSQLAfter(c.sql, c.records)
			if c.wantErr {
				if err == nil {
					t.Fatalf("remainingSQLAfter 期望报错, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("remainingSQLAfter 意外报错: %v", err)
			}
			if got != c.want {
				t.Fatalf("remainingSQLAfter = %q, want %q", got, c.want)
			}
		})
	}
}
