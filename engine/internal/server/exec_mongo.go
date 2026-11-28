package server

import (
	"context"
	"fmt"
	"strings"

	"engine/api"
	mongoe "engine/mongo"
	"engine/mongoconn"
	"engine/ruleset"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// connectMongo receives plaintext credentials over the authenticated engine RPC.
func connectMongo(ds *api.DataSource) (*mongo.Client, error) {
	if ds == nil {
		return nil, fmt.Errorf("MongoDB 数据源 IP / 节点地址为空")
	}
	return mongoconn.Connect(mongoconn.Config{
		Hosts: ds.Ip, Port: int(ds.Port),
		Username: ds.Username, Password: ds.Password,
		AuthDatabase: ds.AuthDb, ReplicaSet: ds.MongoReplicaSet,
		TLS: ds.MongoTLS, AuthMechanism: ds.MongoAuthMechanism,
		CA: ds.Ca, Certificate: ds.Cert, Key: ds.Key,
	})
}

// mongoExecDefinition resolves the effective v2 rule set. RuleSet wins; Rules is
// converted through the compatibility layer for old callers.
func mongoExecDefinition(req *api.ExecRequest) (ruleset.Definition, error) {
	if req == nil || req.Order == nil {
		return ruleset.Definition{}, fmt.Errorf("MongoDB 工单为空")
	}
	if len(req.RuleSet) > 0 {
		def, err := ruleset.Decode(req.RuleSet)
		if err != nil {
			return def, err
		}
		ruleset.EnforceMongoDBAApprovalRules(&def)
		return def, ruleset.ValidateMongoDefinition(def)
	}
	def := ruleset.MongoDefinitionFromAPI(req.Rules)
	ruleset.EnforceMongoDBAApprovalRules(&def)
	return def, ruleset.ValidateMongoDefinition(def)
}

// execMongo 执行 MongoDB 变更工单：规则集校验 → DBA 特批 → 规则复检 → 连接 → 执行。
// 所有不满足条件的情况都发生在 connectMongo 之前。
func (e *Engine) execMongo(ctx context.Context, req *api.ExecRequest) (*api.ExecReply, error) {
	def, err := mongoExecDefinition(req)
	if err != nil {
		return &api.ExecReply{Ok: false, Error: err.Error()}, nil
	}
	warns, recheckErr := recheckMongoRules(req, def)
	if recheckErr != nil {
		return &api.ExecReply{Ok: false, Error: recheckErr.Error()}, nil
	}

	client, err := connectMongo(req.Source)
	if err != nil {
		return &api.ExecReply{Ok: false, Error: "连接 MongoDB 失败: " + err.Error()}, nil
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	engine := mongoe.New(client)
	out, err := engine.ExecuteDefinition(ctx, mongoe.Order{
		Command:  req.Order.Sql,
		Database: req.Order.DataBase,
	}, def, req.DBAApproval)
	if err != nil || !out.Ok {
		msg := "执行失败"
		if out.Error != nil {
			msg = out.Error.Error()
		} else if err != nil {
			msg = err.Error()
		}
		return &api.ExecReply{Ok: false, Error: msg,
			Records: []*api.Record{{Sql: req.Order.Sql, Status: "执行失败", Level: 1, Error: msg}}}, nil
	}

	rec := &api.Record{Sql: req.Order.Sql, Status: "已执行", AffectRows: uint32(out.Affected)}
	notes := append(warns, out.Warning)
	rec.Error = strings.Join(nonEmpty(notes), "；")
	return &api.ExecReply{Ok: true, Records: []*api.Record{rec}, Rollback: out.Rollback}, nil
}

// recheckMongoRules 执行前按当前规则集复检。有效 DBA 特批只放行硬底线，
// 不豁免普通 error 级规则；warning 级说明随执行明细返回。
func recheckMongoRules(req *api.ExecRequest, def ruleset.Definition) ([]string, error) {
	ruleset.EnforceMongoDBAApprovalRules(&def)
	engine := mongoe.New(nil)
	decision, err := engine.AuditDefinitionWithApproval(nil, mongoe.Input{
		Command:  req.Order.Sql,
		Database: req.Order.DataBase,
		Mode:     mongoe.ModeWrite,
	}, def, req.DBAApproval)
	if err != nil {
		return nil, fmt.Errorf("执行前复核失败: %w", err)
	}
	if decision.Error != nil {
		return nil, fmt.Errorf("执行前复核失败: %w", decision.Error)
	}
	blocked := mongoe.Blocked(decision.Findings)
	// Findings retain the approval audit trail; Allowed accounts for completed DBA approval.
	if blocked != nil && !decision.Allowed {
		if blocked.RequiresDBAApproval {
			return nil, fmt.Errorf("执行前复核发现 DBA 特批未完成: %s", decision.DBAApprovalMessage)
		}
		return nil, fmt.Errorf("执行前复核发现阻断级问题: %s", mongoe.Messages(decision.Findings))
	}
	var warns []string
	for _, finding := range decision.Findings {
		if !finding.RequiresDBAApproval && finding.Message != "" {
			warns = append(warns, finding.Message)
		}
	}
	return warns, nil
}

func nonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}
