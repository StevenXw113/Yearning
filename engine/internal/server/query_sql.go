package server

import (
	"engine/api"
	storepb "engine/internal/bytebase/generated-go/store"
	"engine/internal/mysqlparse"
)

// queryDialect 查询预检（拆分 + 语法检查 + 只读白名单 + 带回敏感词表），方言由调用方给定。
// Doris 的查询/DML 语法兼容 MySQL，走 MySQL 方言；PostgreSQL 的 WITH 可接写语句，扫描器会识别。
func queryDialect(engine storepb.Engine, req *api.QueryRequest) *api.QueryReply {
	stmts, err := mysqlparse.SplitEngine(engine, req.Sql)
	if err != nil {
		return &api.QueryReply{Ok: false, Error: err.Error()}
	}
	// 空返回：整体语法失败，报错阻断。
	if len(stmts) == 0 {
		if perr := mysqlparse.CheckEngine(engine, req.Sql); perr != nil {
			return &api.QueryReply{Ok: false, Error: "SQL 语法错误: " + perr.Error()}
		}
		return &api.QueryReply{Ok: true, Records: nil}
	}

	recs := make([]*api.Record, 0, len(stmts))
	for _, text := range stmts {
		rec := &api.Record{
			Sql:   text,
			Level: 0,
		}
		if perr := mysqlparse.CheckEngine(engine, text); perr != nil {
			rec.Error = "SQL 语法错误: " + perr.Error()
			rec.Level = 1
		} else if !isReadOnlyQuery(text) {
			// 只读白名单（引擎侧纵深防御）：主程序本地执行是第一道，这里再拦一次
			rec.Error = "查询页仅允许只读语句（SELECT/SHOW/DESC/EXPLAIN 等），变更请走工单申请"
			rec.Level = 1
		}
		// 敏感字段词表逐条附带，Yearning 本地据此收敛字段内容。
		if iwl := splitList(req.InsulateWordList); len(iwl) > 0 {
			rec.InsulateWordList = iwl
		}
		recs = append(recs, rec)
	}
	return &api.QueryReply{Ok: true, Records: recs}
}

func querySQL(req *api.QueryRequest) *api.QueryReply {
	return queryDialect(storepb.Engine_MYSQL, req)
}

func queryPostgreSQL(req *api.QueryRequest) *api.QueryReply {
	return queryDialect(storepb.Engine_POSTGRES, req)
}

// queryDoris：查询/DML 语法兼容 MySQL，直接走 MySQL 方言；建表 DDL 不会出现在
// 查询页（只执行只读查询），故无需处理 Doris 特有语法。
func queryDoris(req *api.QueryRequest) *api.QueryReply {
	return queryDialect(storepb.Engine_MYSQL, req)
}
