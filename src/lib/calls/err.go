package calls

import (
	"errors"
)

// ReplyError 是 engine.v1 各 Reply 的通用错误接口（均由 proto 生成）。
type ReplyError interface {
	GetError() string
}

// CombineReplyErr 合并传输层错误与业务层错误。
// 两者皆空返回 nil；否则返回描述性错误，便于调用方回显。
func CombineReplyErr(r ReplyError, err error) error {
	if err != nil {
		return err
	}
	if r != nil && r.GetError() != "" {
		return errors.New(r.GetError())
	}
	return nil
}
