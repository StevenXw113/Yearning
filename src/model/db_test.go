package model

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	mmsql "github.com/go-sql-driver/mysql"
)

type stubQuerier struct {
	calls int
	errs  []error
}

func (s *stubQuerier) QueryContext(_ context.Context, _ string, _ ...interface{}) (*sql.Rows, error) {
	s.calls++
	if s.calls <= len(s.errs) {
		return nil, s.errs[s.calls-1]
	}
	return nil, nil
}

func TestQueryWithReadRetry(t *testing.T) {
	ctx := context.Background()

	// 连接已被对端关掉：重试一次就成功
	dead := &stubQuerier{errs: []error{mmsql.ErrInvalidConn}}
	if _, err := queryWithReadRetry(ctx, dead, "SELECT 1"); err != nil || dead.calls != 2 {
		t.Fatalf("坏连接应重试一次: calls=%d err=%v", dead.calls, err)
	}

	// 两次都坏：把错误交出去，不要无限重试
	always := &stubQuerier{errs: []error{mmsql.ErrInvalidConn, mmsql.ErrInvalidConn}}
	if _, err := queryWithReadRetry(ctx, always, "SELECT 1"); !errors.Is(err, mmsql.ErrInvalidConn) || always.calls != 2 {
		t.Fatalf("只应重试一次: calls=%d err=%v", always.calls, err)
	}

	// 语法/业务错误：原样返回，不重试
	bad := errors.New("Error 1064: You have an error in your SQL syntax")
	other := &stubQuerier{errs: []error{bad}}
	if _, err := queryWithReadRetry(ctx, other, "SELECT"); !errors.Is(err, bad) || other.calls != 1 {
		t.Fatalf("业务错误不该重试: calls=%d err=%v", other.calls, err)
	}
}
