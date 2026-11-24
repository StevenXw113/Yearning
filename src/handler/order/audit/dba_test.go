package audit

import (
	"errors"
	"testing"
)

func TestValidateDBAApprovalInput(t *testing.T) {
	cases := []struct {
		name    string
		action  string
		reason  string
		wantErr bool
	}{
		{"approve", "approve", "已核对备份", false},
		{"reject", "reject", "风险过高", false},
		{"action 空", "", "reason", true},
		{"action 非法", "pending", "reason", true},
		{"reason 空", "approve", " ", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateDBAApprovalInput(c.action, c.reason)
			if (err != nil) != c.wantErr {
				t.Fatalf("validateDBAApprovalInput() error = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}

func TestDBAApprovalUpdates(t *testing.T) {
	approve := dbaApprovalUpdates("approve", "dba1", "reason", "2026-01-01 00:00:00")
	if approve["dba_approval_status"] != "approved" || approve["dba_approver"] != "dba1" {
		t.Fatalf("approve updates = %#v", approve)
	}
	if _, exists := approve["status"]; exists {
		t.Fatalf("approve 不应改变审批流状态: %#v", approve)
	}

	reject := dbaApprovalUpdates("reject", "dba1", "reason", "2026-01-01 00:00:00")
	if reject["dba_approval_status"] != "rejected" || reject["status"] != 0 {
		t.Fatalf("reject updates = %#v", reject)
	}
}

func TestDBADepartmentAllowed(t *testing.T) {
	if !dbaDepartmentAllowed(" DBA ") || !dbaDepartmentAllowed("dba") {
		t.Fatal("DBA 部门应允许特批")
	}
	if dbaDepartmentAllowed("开发") {
		t.Fatal("非 DBA 部门不应允许特批")
	}
	_ = errors.New
}
