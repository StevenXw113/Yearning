package common

import (
	"Yearning-go/src/model"
)

// SOAR 错误码 1900-1999

func ERR_SOAR_ALTER_MERGE() Resp {
	return Resp{
		Code: 1901,
		Text: "sql is empty",
	}
}

func ERR_COMMON_MESSAGE(err error) Resp {
	// 记录原始错误便于排查；响应体保留给前端展示，
	// 但调用方不应把数据库凭据、连接串等敏感内容放进 error
	if err != nil {
		model.DefaultLogger.Errorf("%v", err)
	}
	text := ""
	if err != nil {
		text = err.Error()
	}
	return Resp{
		Code: 5555,
		Text: text,
	}
}

func ERR_COMMON_TEXT_MESSAGE(err string) Resp {
	return Resp{
		Code: 5555,
		Text: err,
	}
}
