package server

import (
	"context"
	"strings"
	"testing"

	enginev1 "engine/gen/engine/v1"

	storepb "engine/internal/bytebase/generated-go/store"
	bbadvisor "engine/internal/bytebase/plugin/advisor"
)

// auditRoleAllOn 返回所有规则开关都打开的 AuditRole：
// 覆盖"强制/检查 X"类规则（开启即生效）。
func auditRoleAllOn() *enginev1.AuditRole {
	return &enginev1.AuditRole{
		DmlAllowLimitStmt:              true,
		DmlInsertColumns:               true,
		DmlMaxInsertRows:               10,
		DmlWhere:                       true,
		DmlWhereExprValueIsNull:        true,
		DmlOrder:                       true,
		DmlInsertMustExplicitly:        true,
		DdlEnablePrimaryKey:            true,
		DdlCheckTableComment:           true,
		DdlCheckColumnComment:          true,
		DdlCheckColumnNullable:         true,
		DdlCheckColumnDefault:          true,
		DdlEnableAutoincrementInit:     true,
		DdlEnableAutoincrementUnsigned: true,
		DdlEnableDropTable:             true,
		DdlEnableDropDatabase:          true,
		DdlIndexNameSpec:               true,
		DdlMaxKeyParts:                 10,
		DdlMaxKey:                      10,
		DdlMaxCharLength:               10,
		MaxTableNameLen:                64,
		SupportCharset:                 "utf8mb4",
		SupportCollation:               "utf8mb4_general_ci",
		CheckIdentifier:                true,
		MustHaveColumns:                "id",
		DdlAllowColumnType:             true,
		DdlAllowPriNotInt:              true,
		DdlAllowMultiAlter:             true,
		DdlEnableForeignKey:            true,
		DdlTablePrefix:                 "app_",
		DdlAllowChangeColumnPosition:   true,
		DdlCheckFloatDouble:            true,
		AllowCreateView:                true,
		AllowCreatePartition:           true,
		AllowSpecialType:               true,
	}
}

// TestReviewRulesAreRegisteredForMySQL 动态校验 reviewRules 产出的每条规则
// 都能在 Bytebase MySQL 侧找到实现。上游若删除/改名规则，这里会失败，
// 而不是等到用户提交 SQL 时才以 "unknown advisor" 暴露。
func TestReviewRulesAreRegisteredForMySQL(t *testing.T) {
	cases := map[string]*enginev1.AuditRole{
		"全部开启": auditRoleAllOn(),
		"全部关闭": {}, // 覆盖"允许 X"类开关取反得到的禁止规则
	}

	seen := map[storepb.SQLReviewRule_Type]bool{}
	for name, role := range cases {
		caseSeen := map[storepb.SQLReviewRule_Type]bool{}
		for _, r := range reviewRules(role).all() {
			// 同一份配置内不允许重复；不同配置之间取并集统计覆盖。
			if caseSeen[r.GetType()] {
				t.Errorf("[%s] 规则 %v 重复生成", name, r.GetType())
			}
			caseSeen[r.GetType()] = true
			seen[r.GetType()] = true

			if got := r.GetEngine(); got != storepb.Engine_MYSQL {
				t.Errorf("[%s] 规则 %v 的 Engine 应为 MYSQL，实际 %v", name, r.GetType(), got)
			}

			_, err := bbadvisor.Check(context.Background(), storepb.Engine_MYSQL, r.GetType(), bbadvisor.Context{Rule: r})
			if err != nil && strings.Contains(err.Error(), "unknown advisor") {
				t.Errorf("[%s] 规则 %v 未在 Bytebase MySQL 侧注册，升级后需调整 rules.go: %v", name, r.GetType(), err)
			}
		}
	}

	// 守住基本覆盖面：规则开关一处失效不应悄无声息。
	if len(seen) < 20 {
		t.Fatalf("reviewRules 覆盖的规则过少（%d 条），映射可能已大面积失效", len(seen))
	}
	t.Logf("reviewRules 覆盖 %d 条规则，全部已在 MySQL 侧注册", len(seen))
}
