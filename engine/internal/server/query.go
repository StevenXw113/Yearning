package server

import (
	"context"
	"strings"

	"engine/api"
)

// Query 将用户的查询 SQL 拆分为若干独立语句并逐条做语法/合规预检。
// Yearning 侧随后对每条返回的 Record.SQL 在本地执行取数，因此本方法不真正连库，
// 仅负责拆分与校验；敏感字段词表原样带回供 Yearning 本地收敛展示。
//
// 方言按 kind 分派，各自独立实现、互不混用：
//   - postgres → queryPostgreSQL（query_pg.go）
//   - doris    → queryDoris（query_doris.go，查询语法兼容 MySQL）
//   - mysql / 空 kind → querySQL（query_sql.go；不填 kind 的历史调用方按 MySQL）
//
// 其余未知 kind 显式拒绝（fail-closed），与 Check / Exec 保持一致。
func (e *Engine) Query(_ context.Context, req *api.QueryRequest) (*api.QueryReply, error) {
	if req == nil || strings.TrimSpace(req.Sql) == "" {
		return &api.QueryReply{Ok: false, Error: "SQL 不能为空"}, nil
	}
	switch k := strings.ToLower(strings.TrimSpace(req.GetKind())); k {
	case "", "mysql":
		return querySQL(req), nil
	case "postgres":
		return queryPostgreSQL(req), nil
	case "doris":
		return queryDoris(req), nil
	default:
		return &api.QueryReply{Ok: false, Error: "引擎暂不支持该数据源类型: " + k + "，请升级引擎或检查数据源配置"}, nil
	}
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
