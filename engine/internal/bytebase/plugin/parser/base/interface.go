package base

import (
	"fmt"
	"sync"

	storepb "engine/internal/bytebase/generated-go/store"
)

var ()

var (
	mux                  sync.Mutex
	splitters            = make(map[storepb.Engine]SplitMultiSQLFunc)
	statementParsers     = make(map[storepb.Engine]ParseStatementsFunc)
	statementTypeGetters = make(map[storepb.Engine]GetStatementTypesFunc)
)

type SplitMultiSQLFunc func(string) ([]Statement, error)

// ParseStatementsFunc is the interface for parsing SQL statements and returning []ParsedStatement.
// This is the new unified parsing function that returns complete ParsedStatement objects with AST.
type ParseStatementsFunc func(statement string) ([]ParsedStatement, error)

// GetStatementTypesFunc returns the types of statements in the ASTs.
// Statement types include: INSERT, UPDATE, DELETE (DML), CREATE_TABLE, ALTER_TABLE, DROP_TABLE, etc. (DDL).
type GetStatementTypesFunc func([]AST) ([]storepb.StatementType, error)

func RegisterSplitterFunc(engine storepb.Engine, f SplitMultiSQLFunc) {
	mux.Lock()
	defer mux.Unlock()
	if _, dup := splitters[engine]; dup {
		panic(fmt.Sprintf("Register called twice %s", engine))
	}
	splitters[engine] = f
}

// SplitMultiSQL splits statement into a slice of the single SQL.
func SplitMultiSQL(engine storepb.Engine, statement string) ([]Statement, error) {
	f, ok := splitters[engine]
	if !ok {
		return nil, fmt.Errorf("engine %s is not supported", engine)
	}
	return f(statement)
}

func RegisterParseStatementsFunc(engine storepb.Engine, f ParseStatementsFunc) {
	mux.Lock()
	defer mux.Unlock()
	if _, dup := statementParsers[engine]; dup {
		panic(fmt.Sprintf("RegisterParseStatementsFunc called twice %s", engine))
	}
	statementParsers[engine] = f
}

// ParseStatements parses the SQL statement and returns ParsedStatement objects with both text and AST.
// This is the new unified parsing function.
func ParseStatements(engine storepb.Engine, statement string) ([]ParsedStatement, error) {
	f, ok := statementParsers[engine]
	if !ok {
		return nil, fmt.Errorf("engine %s is not supported for ParseStatements", engine)
	}
	return f(statement)
}

func RegisterGetStatementTypes(engine storepb.Engine, f GetStatementTypesFunc) {
	mux.Lock()
	defer mux.Unlock()
	if _, dup := statementTypeGetters[engine]; dup {
		panic(fmt.Sprintf("Register called twice %s", engine))
	}
	statementTypeGetters[engine] = f
}

// GetStatementTypes returns the types of statements in the ASTs.
func GetStatementTypes(engine storepb.Engine, asts []AST) ([]storepb.StatementType, error) {
	f, ok := statementTypeGetters[engine]
	if !ok {
		return nil, fmt.Errorf("engine %s is not supported", engine)
	}
	return f(asts)
}

const searchPathPrefix = "SET LOCAL search_path TO "

var ()
