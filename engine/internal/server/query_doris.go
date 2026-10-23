package server

import (
	enginev1 "engine/gen/engine/v1"
	storepb "engine/internal/bytebase/generated-go/store"
	"engine/internal/mysqlparse"
)

// queryDoris Doris 查询预检：拆分 + 语法检查 + 带回敏感词表。
//
// Doris 的查询/DML 语法兼容 MySQL，所以这里直接用 MySQL 方言拆分与语法检查；
// 建表 DDL 不会出现在查询页（查询页只执行只读查询），故无需处理 Doris 特有语法。
func queryDoris(req *enginev1.QueryRequest) *enginev1.QueryReply {
	const engine = storepb.Engine_MYSQL

	stmts, err := mysqlparse.SplitEngine(engine, req.Sql)
	if err != nil {
		return &enginev1.QueryReply{Ok: false, Error: err.Error()}
	}
	if len(stmts) == 0 {
		if perr := mysqlparse.CheckEngine(engine, req.Sql); perr != nil {
			return &enginev1.QueryReply{Ok: false, Error: "SQL 语法错误: " + perr.Error()}
		}
		return &enginev1.QueryReply{Ok: true, Records: nil}
	}

	recs := make([]*enginev1.Record, 0, len(stmts))
	for _, text := range stmts {
		rec := &enginev1.Record{
			Sql:   text,
			Level: 0,
		}
		if perr := mysqlparse.CheckEngine(engine, text); perr != nil {
			rec.Error = "SQL 语法错误: " + perr.Error()
			rec.Level = 1
		}
		// 敏感字段词表逐条附带，Yearning 本地据此收敛字段内容。
		if iwl := splitList(req.InsulateWordList); len(iwl) > 0 {
			rec.InsulateWordList = iwl
		}
		recs = append(recs, rec)
	}
	return &enginev1.QueryReply{Ok: true, Records: recs}
}
