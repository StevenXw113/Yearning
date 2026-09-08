package server

import (
	"context"
	"regexp"
	"strings"

	enginev1 "engine/gen/engine/v1"
	"engine/internal/mysqlparse"
)

// checkSQL 对一段 SQL 做逐条拆分 + 语法校验 + 基于规则的基础判定，返回审核记录。
func (e *Engine) checkSQL(sql, schema string, rule *enginev1.AuditRole) []*enginev1.Record {
	stmts, err := mysqlparse.Split(sql)
	if err != nil {
		return []*enginev1.Record{{Sql: sql, Error: err.Error(), Level: 1}}
	}
	// 整段整体解析失败（如首句即语法错误），退化按单条校验并报错。
	if len(stmts) == 0 {
		if perr := mysqlparse.Check(sql); perr != nil {
			return []*enginev1.Record{{Sql: sql, Error: "SQL 语法错误: " + perr.Msg, Status: "语法错误", Level: 1}}
		}
		return nil
	}
	recs := make([]*enginev1.Record, 0, len(stmts))
	for _, st := range stmts {
		rec := &enginev1.Record{
			Sql:    st.Text,
			Schema: schema,
			Status: "审核通过",
			Level:  3,
		}
		if perr := mysqlparse.Check(st.Text); perr != nil {
			rec.Error = "SQL 语法错误: " + perr.Msg
			rec.Status = "语法错误"
			rec.Level = 1
			recs = append(recs, rec)
			continue
		}
		// 基础危险/规范规则判定（依据 AuditRole 开关）
		checkBasics(st, rule, rec)
		recs = append(recs, rec)
	}
	return recs
}

// sqlCommentFree 去除字符串与注释，保留控制结构以便做 where/drop 等关键字判断。
var reMultiSpace = regexp.MustCompile(`\s+`)

func checkBasics(st mysqlparse.Statement, rule *enginev1.AuditRole, rec *enginev1.Record) {
	norm := normalize(st.Text)
	switch st.Type {
	case "drop":
		dropDB := hasPrefixWord(norm, "drop database") || hasPrefixWord(norm, "drop schema")
		dropTbl := hasPrefixWord(norm, "drop table") || hasPrefixWord(norm, "drop view")
		if dropDB && rule.GetDdlEnableDropDatabase() {
			rec.Error = "禁止执行 DROP DATABASE 操作"
			rec.Status = "审核不通过"
			rec.Level = 1
			return
		}
		if dropTbl && rule.GetDdlEnableDropTable() {
			rec.Error = "禁止执行 DROP TABLE 操作"
			rec.Status = "审核不通过"
			rec.Level = 1
			return
		}
		if dropTbl || dropDB {
			rec.Error = "高危操作：删除库/表将不可恢复"
			rec.Status = "高危"
			rec.Level = 2
		}
	case "delete", "update":
		// where 缺失是最高危风险。仅当规则开启且明确缺失时才阻断，否则给高危提示。
		hasWhere := containsWhere(norm)
		if rule.GetDmlWhere() && !hasWhere {
			rec.Error = "禁止执行无 WHERE 条件的 " + strings.ToUpper(st.Type) + " 语句"
			rec.Status = "审核不通过"
			rec.Level = 1
			return
		}
		if !hasWhere {
			rec.Error = "高危操作：" + strings.ToUpper(st.Type) + " 语句缺少 WHERE 条件"
			rec.Status = "高危"
			rec.Level = 2
		}
	case "create":
		if hasPrefixWord(norm, "create database") && rule.GetDdlEnableDropDatabase() {
			rec.Error = "禁止在线上创建数据库"
			rec.Status = "审核不通过"
			rec.Level = 1
		}
	}
}

// normalize 折叠空白并转为小写，便于关键字前缀判断。
func normalize(sql string) string {
	return strings.ToLower(reMultiSpace.ReplaceAllString(sql, " "))
}

func hasPrefixWord(s, prefix string) bool {
	return strings.HasPrefix(s, prefix+" ") || strings.HasPrefix(s, prefix+"(")
}

// containsWhere 判断是否存在 where 子句关键字（单词边界）。
var reWhere = regexp.MustCompile(`(?i)\bwhere\b`)

func containsWhere(s string) bool {
	return reWhere.MatchString(s)
}

// Check 实现 gRPC Check：SQL 静态审核。
func (e *Engine) Check(_ context.Context, req *enginev1.CheckRequest) (*enginev1.CheckReply, error) {
	if req == nil || req.Sql == "" {
		return &enginev1.CheckReply{Ok: false, Error: "SQL 不能为空"}, nil
	}
	recs := e.checkSQL(req.Sql, req.Schema, req.Rule)
	return &enginev1.CheckReply{Ok: true, Records: recs}, nil
}
