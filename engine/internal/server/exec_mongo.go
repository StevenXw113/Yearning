package server

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"engine/api"
	mongoe "engine/mongo"
	"engine/ruleset"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// mongoTimeout 建立连接与探活的超时。
const mongoTimeout = 15 * time.Second

// mongoURI 拼 MongoDB 连接串。用户名与口令走 url.UserPassword 转义，
// 否则口令里的 @ : / % 会把连接串拆坏；认证库（authSource）来自数据源配置，
// 未配置时用 MongoDB 默认的 admin——账号建在业务库上的部署必须显式指定，
// 否则认证必失败（此前硬编码 admin，这类部署连不上）。
func mongoURI(username, password, ip string, port int32, authDB string) string {
	if authDB == "" {
		authDB = "admin"
	}
	u := &url.URL{
		Scheme:   "mongodb",
		User:     url.UserPassword(username, password),
		Host:     net.JoinHostPort(ip, strconv.Itoa(int(port))),
		Path:     "/",
		RawQuery: "authSource=" + url.QueryEscape(authDB),
	}
	return u.String()
}

// connectMongo 按 DataSource 建立 MongoDB 客户端并 Ping 探活。调用方负责 Disconnect。
// 口令是明文：主程序在发起 RPC 前已经把口令列解密好了，引擎侧不再持有主程序密钥。
func connectMongo(ds *api.DataSource) (*mongo.Client, error) {
	if ds == nil || ds.Ip == "" {
		return nil, fmt.Errorf("数据源 IP 为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), mongoTimeout)
	defer cancel()
	client, err := mongo.Connect(options.Client().
		ApplyURI(mongoURI(ds.Username, ds.Password, ds.Ip, ds.Port, ds.AuthDb)).
		SetConnectTimeout(mongoTimeout).
		SetServerSelectionTimeout(mongoTimeout))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	return client, nil
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
	if blocked != nil {
		if blocked.RequiresDBAApproval {
			return nil, fmt.Errorf("执行前复核发现 DBA 特批未完成: %s", decision.DBAApprovalMessage)
		}
		return nil, fmt.Errorf("执行前复核发现阻断级问题: %s", mongoe.Messages(decision.Findings))
	}
	var warns []string
	for _, finding := range decision.Findings {
		if finding.Message != "" {
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
