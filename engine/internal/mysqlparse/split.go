package mysqlparse

import (
	"fmt"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	mysql "github.com/bytebase/parser/mysql"
)

// Statement 是拆分后的一条 SQL 语句。
type Statement struct {
	Text string // 单条语句原文（去首尾空白与分号）
	Type string // 语句大类：select/insert/update/delete/create/alter/drop/other
}

// errorListener 累计语法错误信息。
type errorListener struct {
	errors []string
}

func (l *errorListener) SyntaxError(_ antlr.Recognizer, _ interface{}, line, column int, msg string, _ antlr.RecognitionException) {
	l.errors = append(l.errors, fmt.Sprintf("line %d:%d %s", line, column, msg))
}
func (l *errorListener) ReportAmbiguity(antlr.Parser, *antlr.DFA, int, int, bool, *antlr.BitSet, *antlr.ATNConfigSet) {
}
func (l *errorListener) ReportAttemptingFullContext(antlr.Parser, *antlr.DFA, int, int, *antlr.BitSet, *antlr.ATNConfigSet) {
}
func (l *errorListener) ReportContextSensitivity(antlr.Parser, *antlr.DFA, int, int, int, *antlr.ATNConfigSet) {
}

// ParseError 描述一条语句的语法错误。
type ParseError struct {
	Line int
	Col  int
	Msg  string
}

// Split 将整段 SQL 拆分为若干条语句，并报告每条是否存在语法错误。
// 每条语句用独立的 parser 单独校验，因此错误可精确定位到具体语句。
func Split(sql string) ([]Statement, error) {
	if strings.TrimSpace(sql) == "" {
		return nil, nil
	}
	input := antlr.NewInputStream(strings.TrimRight(sql, " \t\r\n") + "\n;")
	lexer := mysql.NewMySQLLexer(input)
	stream := antlr.NewCommonTokenStream(lexer, 0)
	parser := mysql.NewMySQLParser(stream)
	parser.BuildParseTrees = true
	// 静默语法错误，交由调用方按逐条校验报错。
	silent := &errorListener{}
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(silent)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(silent)
	tree := parser.Script()

	var out []Statement
	for _, q := range tree.AllQuery() {
		text := stream.GetTextFromRuleContext(q)
		text = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text), ";"))
		if text == "" {
			continue
		}
		out = append(out, Statement{
			Text: text,
			Type: classify(text),
		})
	}
	return out, nil
}

// Check 对单条 SQL 做语法校验，返回首个语法错误。
func Check(sql string) *ParseError {
	if strings.TrimSpace(sql) == "" {
		return nil
	}
	input := antlr.NewInputStream(strings.TrimRight(sql, " \t\r\n") + "\n;")
	lexer := mysql.NewMySQLLexer(input)
	stream := antlr.NewCommonTokenStream(lexer, 0)
	parser := mysql.NewMySQLParser(stream)
	parser.BuildParseTrees = true

	le := &errorListener{}
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(le)
	pe := &errorListener{}
	parser.RemoveErrorListeners()
	parser.AddErrorListener(pe)

	parser.Script()
	if len(le.errors) > 0 || len(pe.errors) > 0 {
		errs := append(le.errors, pe.errors...)
		first := errs[0]
		return &ParseError{Msg: first}
	}
	return nil
}

// classify 依据语句首个有效关键字粗分类。仅用于前置提示，不替代 AST 判定。
func classify(text string) string {
	s := strings.ToLower(strings.TrimSpace(text))
	// 去掉常见注释与括号前缀影响
	for _, kw := range []string{"insert", "update", "delete", "select", "create", "alter", "drop", "truncate", "replace", "set", "show", "use", "grant", "revoke", "call", "rename"} {
		if s == kw || strings.HasPrefix(s, kw+" ") || strings.HasPrefix(s, kw+"(") {
			return kw
		}
	}
	return "other"
}
