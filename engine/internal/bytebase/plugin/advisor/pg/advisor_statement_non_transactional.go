package pg

import (
	"context"
	"regexp"

	"github.com/bytebase/omni/pg/ast"

	storepb "engine/internal/bytebase/generated-go/store"
	"engine/internal/bytebase/plugin/advisor"
	"engine/internal/bytebase/plugin/advisor/code"
)

var (
	_ advisor.Advisor = (*NonTransactionalAdvisor)(nil)
)

func init() {
	advisor.Register(storepb.Engine_POSTGRES, storepb.SQLReviewRule_STATEMENT_NON_TRANSACTIONAL, &NonTransactionalAdvisor{})
}

// NonTransactionalAdvisor is the advisor checking for non-transactional statements.
type NonTransactionalAdvisor struct {
}

// Check checks for non-transactional statements.
func (*NonTransactionalAdvisor) Check(_ context.Context, checkCtx advisor.Context) ([]*storepb.Advice, error) {
	level, err := advisor.NewStatusBySQLReviewRuleLevel(checkCtx.Rule.Level)
	if err != nil {
		return nil, err
	}

	rule := &nonTransactionalRule{
		OmniBaseRule: OmniBaseRule{
			Level: level,
			Title: checkCtx.Rule.Type.String(),
		},
	}

	return RunRules(checkCtx.ParsedStatements, []OmniRule{rule}), nil
}

type nonTransactionalRule struct {
	OmniBaseRule
}

func (*nonTransactionalRule) Name() string {
	return "statement.non-transactional"
}

func (r *nonTransactionalRule) OnStatement(node ast.Node) {
	switch node.(type) {
	case *ast.DropdbStmt, *ast.IndexStmt, *ast.DropStmt, *ast.VacuumStmt:
	default:
		return
	}

	stmtText := r.TrimmedStmtText()
	if isNonTransactionStatement(stmtText) {
		r.AddAdvice(&storepb.Advice{
			Status:  r.Level,
			Code:    code.StatementNonTransactional.Int32(),
			Title:   r.Title,
			Content: "This statement is non-transactional",
			StartPosition: &storepb.Position{
				Line:   r.ContentStartLine(),
				Column: 0,
			},
		})
	}
}

// isNonTransactionStatement 内联自上游 backend/plugin/db/pg.IsNonTransactionStatement。
// 内化版本不引入 backend/plugin/db/pg（那是完整 pg 驱动、会拖重依赖），
// 该函数只做 4 个正则判断、不依赖连接。
var (
	isNonTransactionalDropDatabaseReg = regexp.MustCompile(`(?i)DROP DATABASE`)
	isNonTransactionalCreateIndexReg  = regexp.MustCompile(`(?i)CREATE(\s+(UNIQUE\s+)?)INDEX(\s+)CONCURRENTLY`)
	isNonTransactionalDropIndexReg    = regexp.MustCompile(`(?i)DROP(\s+)INDEX(\s+)CONCURRENTLY`)
	isNonTransactionalVacuumReg       = regexp.MustCompile(`(?i)^\s*VACUUM`)
)

func isNonTransactionStatement(stmt string) bool {
	if len(isNonTransactionalDropDatabaseReg.FindString(stmt)) > 0 {
		return true
	}
	if len(isNonTransactionalCreateIndexReg.FindString(stmt)) > 0 {
		return true
	}
	if len(isNonTransactionalDropIndexReg.FindString(stmt)) > 0 {
		return true
	}
	return len(isNonTransactionalVacuumReg.FindString(stmt)) > 0
}
