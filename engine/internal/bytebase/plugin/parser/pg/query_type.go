package pg

import (
	"strings"

	"github.com/bytebase/omni/pg/ast"
)

// 原文件还含 QueryType 分类器（query span/编辑器校验链路），机器已删，只留 statement-type
// 仍在用的这几个纯 AST 辅助。

// getWithClause returns the WITH clause of a query-carrying statement.
func getWithClause(node ast.Node) *ast.WithClause {
	switch n := node.(type) {
	case *ast.SelectStmt:
		return n.WithClause
	case *ast.InsertStmt:
		return n.WithClause
	case *ast.UpdateStmt:
		return n.WithClause
	case *ast.DeleteStmt:
		return n.WithClause
	case *ast.MergeStmt:
		return n.WithClause
	case *ast.CreateTableAsStmt:
		return getWithClause(n.Query)
	default:
		return nil
	}
}

// hasOmniIntoClause checks if a SelectStmt has an INTO clause (SELECT INTO).
func hasOmniIntoClause(n *ast.SelectStmt) bool {
	return omniIntoClause(n) != nil
}

// omniIntoClause returns the INTO clause of n, searching set-operation arms
// (UNION/INTERSECT/EXCEPT) — the parser attaches INTO to the first arm, not the root.
func omniIntoClause(n *ast.SelectStmt) *ast.IntoClause {
	if n == nil {
		return nil
	}
	if n.IntoClause != nil {
		return n.IntoClause
	}
	if c := omniIntoClause(n.Larg); c != nil {
		return c
	}
	return omniIntoClause(n.Rarg)
}

// isExplainAnalyzeOmni reports whether an ExplainStmt executes its query: its last ANALYZE option
// is not FALSE, OFF, or 0, the values PostgreSQL reads as false.
func isExplainAnalyzeOmni(n *ast.ExplainStmt) bool {
	if n.Options == nil {
		return false
	}
	analyze := false
	for _, item := range n.Options.Items {
		de, ok := item.(*ast.DefElem)
		if !ok || !strings.EqualFold(de.Defname, "analyze") {
			continue
		}
		switch arg := de.Arg.(type) {
		case *ast.String:
			analyze = !strings.EqualFold(arg.Str, "false") && !strings.EqualFold(arg.Str, "off")
		case *ast.Integer:
			analyze = arg.Ival != 0
		default:
			analyze = true
		}
	}
	return analyze
}

// UnwrapExplainAnalyze returns the statement that an EXPLAIN ANALYZE executes and that statement's text
// within text, the text of the EXPLAIN. It returns any other node and text unchanged.
func UnwrapExplainAnalyze(node ast.Node, text string) (ast.Node, string) {
	explain, ok := node.(*ast.ExplainStmt)
	if !ok || !isExplainAnalyzeOmni(explain) {
		return node, text
	}
	inner, ok := explainedText(explain, text)
	if !ok {
		return explain.Query, text
	}
	return explain.Query, inner
}

// explainedText returns the text of the statement an EXPLAIN runs, within the EXPLAIN's own text.
func explainedText(explain *ast.ExplainStmt, text string) (string, bool) {
	// The statement runs from its WITH clause, which the location of a SELECT leaves out, to the end
	// of the EXPLAIN, which its location can also leave out, as for ORDER BY.
	start := ast.NodeLoc(explain.Query).Start
	if with := getWithClause(explain.Query); with != nil && with.Loc.Start >= 0 && with.Loc.Start < start {
		start = with.Loc.Start
	}
	if start < 0 || start >= explain.Loc.End || explain.Loc.End > len(text) {
		return "", false
	}
	return text[start:explain.Loc.End], true
}
