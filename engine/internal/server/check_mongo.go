package server

import (
	"strings"

	"engine/api"
	mongoe "engine/mongo"
	"engine/ruleset"
)

// isMongoSource 判断审核目标是否为 MongoDB 数据源（kind 由主程序按 db_type 映射）。
func isMongoSource(src *api.DataSource) bool {
	return src != nil && strings.EqualFold(src.GetKind(), "mongodb")
}

// checkMongo 审核 MongoDB 命令。RuleSet 是 v2 数据源规则集，优先级高于 Rule；
// Rule 为空规则集时仅作为旧调用方兼容层。
func checkMongo(req *api.CheckRequest) *api.CheckReply {
	def := ruleset.MongoDefinitionFromAPI(req.Rule)
	if len(req.RuleSet) > 0 {
		decoded, err := ruleset.Decode(req.RuleSet)
		if err != nil {
			return &api.CheckReply{Ok: false, Error: err.Error()}
		}
		def = decoded
	}
	ruleset.EnforceMongoDBAApprovalRules(&def)
	engine := mongoe.New(nil)
	decision, err := engine.AuditDefinition(nil, mongoe.Input{
		Command:  req.Sql,
		Database: req.Schema,
		Mode:     mongoe.Mode(req.GetMode()),
	}, def)
	if err != nil {
		return &api.CheckReply{Ok: false, Error: err.Error()}
	}
	if decision.Error != nil {
		return &api.CheckReply{Ok: false, Error: decision.Error.Error()}
	}
	blocked := mongoe.Blocked(decision.Findings) != nil
	rec := findingsRecord(strings.TrimSpace(req.Sql), req.Schema,
		mongoe.Messages(decision.Findings), mongoe.Severity(decision.Findings), blocked)
	applyMongoFindingMeta(rec, decision)
	return &api.CheckReply{Ok: true, Records: []*api.Record{rec}}
}

func applyMongoFindingMeta(rec *api.Record, decision mongoe.Decision) {
	if rec == nil {
		return
	}
	seen := map[string]struct{}{}
	rec.RuleIDs = make([]string, 0, len(decision.Findings))
	for _, finding := range decision.Findings {
		if finding.RuleID == "" {
			continue
		}
		if _, ok := seen[finding.RuleID]; ok {
			continue
		}
		seen[finding.RuleID] = struct{}{}
		rec.RuleIDs = append(rec.RuleIDs, finding.RuleID)
	}
	if len(rec.RuleIDs) > 0 {
		rec.RuleID = rec.RuleIDs[0]
	}
	rec.RequiresDBAApproval = decision.RequiresDBAApproval
}
