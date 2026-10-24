package server

import (
	enginev1 "engine/gen/engine/v1"
	storepb "engine/internal/bytebase/generated-go/store"
	"engine/internal/mysqlparse"
)

// queryPostgreSQL PostgreSQL 查询预检：拆分 + 语法检查 + 带回敏感词表。方言固定 PostgreSQL。
func queryPostgreSQL(req *enginev1.QueryRequest) *enginev1.QueryReply {
	const engine = storepb.Engine_POSTGRES

	stmts, err := mysqlparse.SplitEngine(engine, req.Sql)
	if err != nil {
		return &enginev1.QueryReply{Ok: false, Error: err.Error()}
	}
	// 空返回：整体语法失败，报错阻断。
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
		} else if !isReadOnlyQuery(text) {
			// 只读白名单（引擎侧纵深防御）；PostgreSQL 的 WITH 可接写语句，扫描器会识别
			rec.Error = "查询页仅允许只读语句（SELECT/SHOW/DESC/EXPLAIN 等），变更请走工单申请"
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
