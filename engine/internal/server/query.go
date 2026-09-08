package server

import (
	"context"
	"strings"

	enginev1 "engine/gen/engine/v1"
	"engine/internal/mysqlparse"
)

// Query 将用户的查询 SQL 拆分为若干独立语句并逐条做语法/合规预检。
// Yearning 侧随后对每条返回的 Record.SQL 在本地执行取数，因此本方法不真正连库，
// 仅负责拆分与校验；敏感字段词表原样带回供 Yearning 本地收敛展示。
func (e *Engine) Query(_ context.Context, req *enginev1.QueryRequest) (*enginev1.QueryReply, error) {
	if req == nil || strings.TrimSpace(req.Sql) == "" {
		return &enginev1.QueryReply{Ok: false, Error: "SQL 不能为空"}, nil
	}
	stmts, err := mysqlparse.Split(req.Sql)
	if err != nil {
		return &enginev1.QueryReply{Ok: false, Error: err.Error()}, nil
	}
	// 空返回：整体语法失败，报错阻断。
	if len(stmts) == 0 {
		if perr := mysqlparse.Check(req.Sql); perr != nil {
			return &enginev1.QueryReply{Ok: false, Error: "SQL 语法错误: " + perr.Msg}, nil
		}
		return &enginev1.QueryReply{Ok: true, Records: nil}, nil
	}

	recs := make([]*enginev1.Record, 0, len(stmts))
	for _, st := range stmts {
		rec := &enginev1.Record{
			Sql:   st.Text,
			Level: 3,
		}
		if perr := mysqlparse.Check(st.Text); perr != nil {
			rec.Error = "SQL 语法错误: " + perr.Msg
			rec.Level = 1
		}
		// 敏感字段词表逐条附带，Yearning 本地据此收敛字段内容。
		if iwl := splitList(req.InsulateWordList); len(iwl) > 0 {
			rec.InsulateWordList = iwl
		}
		recs = append(recs, rec)
	}
	return &enginev1.QueryReply{Ok: true, Records: recs}, nil
}

// splitList 将逗号分隔的词表拆成切片。
func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
