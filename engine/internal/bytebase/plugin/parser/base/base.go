package base

import (
	"context"

	storepb "engine/internal/bytebase/generated-go/store"
)

// SyntaxError is a syntax error.
type SyntaxError struct {
	Position   *storepb.Position
	Message    string
	RawMessage string
}

// Error returns the error message.
func (e *SyntaxError) Error() string {
	return e.Message
}

func GetOffsetLength(total int) int {
	length := 1
	for {
		if total < 10 {
			return length
		}
		total /= 10
		length++
	}
}

// GetLineOffset returns the 0-based line offset from a StartPosition.
// This is useful for converting from the 1-based StartPosition to the 0-based offset
// needed for some calculations.
func GetLineOffset(startPosition *storepb.Position) int {
	if startPosition == nil {
		return 0
	}
	return int(startPosition.Line) - 1
}

// ListDatabaseNamesFunc 原在 query-span 机器（已删）里声明，advisor 的 Context 字段仍在用。
// ponytail: 引擎侧从未注入该实现，advisor.DatabaseExists 恒走 nil 分支；接入真实实现时再评估去留。
type ListDatabaseNamesFunc func(context.Context, string) ([]string, error)
