package server

import (
	"context"
	"strings"

	enginev1 "engine/gen/engine/v1"
)

// Query 将用户的查询 SQL 拆分为若干独立语句并逐条做语法/合规预检。
// Yearning 侧随后对每条返回的 Record.SQL 在本地执行取数，因此本方法不真正连库，
// 仅负责拆分与校验；敏感字段词表原样带回供 Yearning 本地收敛展示。
//
// 方言按 kind 分派，各自独立实现、互不混用：
//   - postgres → queryPG（query_pg.go）
//   - 其余（含空值）→ querySQL（query_sql.go，MySQL）
func (e *Engine) Query(_ context.Context, req *enginev1.QueryRequest) (*enginev1.QueryReply, error) {
	if req == nil || strings.TrimSpace(req.Sql) == "" {
		return &enginev1.QueryReply{Ok: false, Error: "SQL 不能为空"}, nil
	}
	if strings.EqualFold(req.GetKind(), "postgres") {
		return queryPG(req), nil
	}
	return querySQL(req), nil
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
