package mongo

import (
	"context"
	"errors"
	"strings"

	"engine/api"
	"engine/internal/mongocheck"
	"engine/internal/mongodb"
	"engine/mongoguard"
	"engine/ruleset"

	"go.mongodb.org/mongo-driver/v2/bson"
	driver "go.mongodb.org/mongo-driver/v2/mongo"
)

type Mode = mongocheck.Mode

const (
	ModeWrite = mongocheck.ModeWrite
	ModeQuery = mongocheck.ModeQuery
)

type Finding = mongocheck.Finding

type Level = mongocheck.Level

const (
	LevelError   = mongocheck.LevelError
	LevelWarn    = mongocheck.LevelWarn
	LevelObserve = mongocheck.LevelObserve
)

type Input struct {
	Command  string
	Database string
	Mode     Mode
}

type Order struct {
	Command  string
	Database string
}

type Decision struct {
	Allowed bool
	// RequiresDBAApproval means the command hit a hard safety baseline. Such a
	// command can only execute after a complete DBA approval is supplied.
	RequiresDBAApproval bool
	DBAApprovalRuleID   string
	DBAApprovalMessage  string
	Findings            []Finding
	Error               error
}

type Result struct {
	Ok       bool
	Affected int64
	Rollback []string
	Warning  string
	Status   string
	Error    error
}

type Engine struct {
	client *driver.Client
}

func New(client *driver.Client) *Engine {
	return &Engine{client: client}
}

// Audit audits with the legacy wire contract. It remains available for old callers,
// but new data-source rule sets should use AuditDefinition.
func (e *Engine) Audit(ctx context.Context, input Input, rules *api.AuditRole) (Decision, error) {
	return e.auditAPI(ctx, input, rules, nil)
}

// AuditDefinition audits a v2 MongoDB data-source rule set. Invalid definitions
// fail closed before command execution.
func (e *Engine) AuditDefinition(ctx context.Context, input Input, def ruleset.Definition) (Decision, error) {
	return e.AuditDefinitionWithApproval(ctx, input, def, nil)
}

// AuditDefinitionWithApproval accepts a DBA approval only for commands on the hard
// safety baseline. Normal rule findings are still enforced.
func (e *Engine) AuditDefinitionWithApproval(ctx context.Context, input Input, def ruleset.Definition, approval *api.DBAApproval) (Decision, error) {
	ruleset.EnforceMongoDBAApprovalRules(&def)
	if err := ruleset.ValidateMongoDefinition(def); err != nil {
		return Decision{Allowed: false, Error: err}, err
	}
	return e.auditAPI(ctx, input, ruleset.MongoDefinitionToAPI(def), approval)
}

// Execute executes with the legacy wire contract. High-risk commands are blocked
// because this entry has no DBA approval.
func (e *Engine) Execute(ctx context.Context, order Order, rules *api.AuditRole) (Result, error) {
	def := ruleset.MongoDefinitionFromAPI(rules)
	return e.ExecuteDefinition(ctx, order, def, nil)
}

// ExecuteDefinition validates the rule set, re-audits the command, and then executes.
// A missing or incomplete DBA approval blocks before mongodb.Execute is reached.
func (e *Engine) ExecuteDefinition(ctx context.Context, order Order, def ruleset.Definition, approval *api.DBAApproval) (Result, error) {
	ruleset.EnforceMongoDBAApprovalRules(&def)
	if err := ruleset.ValidateMongoDefinition(def); err != nil {
		return Result{Status: "blocked", Error: err}, nil
	}
	decision, err := e.AuditDefinitionWithApproval(ctx, Input{
		Command:  order.Command,
		Database: order.Database,
		Mode:     ModeWrite,
	}, def, approval)
	if err != nil {
		return Result{Status: "failed", Error: err}, nil
	}
	if decision.Error != nil {
		return Result{Status: "failed", Error: decision.Error}, nil
	}
	if !decision.Allowed {
		var msgs []string
		for _, finding := range decision.Findings {
			msgs = append(msgs, finding.Message)
		}
		if decision.RequiresDBAApproval && !approvalValid(approval) {
			return Result{
				Status: "blocked",
				Error:  errors.New(decision.DBAApprovalMessage + "；DBA 特批信息不完整或未批准"),
			}, nil
		}
		return Result{Status: "blocked", Error: errors.New(strings.Join(nonEmptyStrings(msgs), "；"))}, nil
	}

	cmd, err := mongoguard.Parse(order.Command)
	if err != nil {
		return Result{Status: "failed", Error: err}, nil
	}
	if _, err := mongodb.Validate(cmd); err != nil {
		return Result{Status: "failed", Error: err}, nil
	}
	out, err := mongodb.Execute(ctx, e.client, order.Database, cmd, def.Settings.MaxAffectRows)
	if err != nil {
		return Result{Status: "failed", Error: err}, nil
	}
	return Result{Ok: true, Affected: out.Affected, Rollback: out.Rollback, Warning: out.Warning, Status: "executed"}, nil
}

func (e *Engine) Query(ctx context.Context, input Input) (bson.M, error) {
	cmd, err := mongoguard.Parse(input.Command)
	if err != nil {
		return nil, err
	}
	if _, err := mongoguard.ValidateQuery(cmd); err != nil {
		return nil, err
	}
	var raw bson.M
	if err := e.client.Database(input.Database).RunCommand(ctx, cmd).Decode(&raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (e *Engine) auditAPI(ctx context.Context, input Input, rules *api.AuditRole, approval *api.DBAApproval) (Decision, error) {
	_ = ctx
	mode := input.Mode
	if mode == "" {
		mode = ModeWrite
	}
	_, findings, err := mongocheck.Check(input.Command, mongocheck.ConfigFromAPI(rules, mode))
	if err != nil {
		return Decision{Allowed: false, Error: err}, nil
	}
	decision := dbaDecision(findings)
	if decision.RequiresDBAApproval && approvalValid(approval) {
		findings = withoutDBAFindings(findings)
		decision.Allowed = mongocheck.Blocked(findings) == nil
		return decision, nil
	}
	decision.Allowed = mongocheck.Blocked(findings) == nil
	return decision, nil
}

// RequiresDBAApproval is the public pre-flight check used by the main application.
// It never imports engine/internal from the root module and returns the stable
// v2 rule ID for the special-approval workflow.
func RequiresDBAApproval(command string) (bool, string, string, error) {
	_, findings, err := mongocheck.Check(command, mongocheck.Config{Mode: ModeWrite})
	if err != nil {
		return false, "", "", err
	}
	for _, finding := range findings {
		if finding.RequiresDBAApproval {
			return true, finding.RuleID, finding.Message, nil
		}
	}
	return false, "", "", nil
}

func dbaDecision(findings []Finding) Decision {
	out := Decision{Findings: findings}
	for _, finding := range findings {
		if finding.RequiresDBAApproval {
			out.RequiresDBAApproval = true
			out.DBAApprovalRuleID = finding.RuleID
			out.DBAApprovalMessage = finding.Message
			break
		}
	}
	return out
}

func withoutDBAFindings(in []Finding) []Finding {
	out := make([]Finding, 0, len(in))
	for _, finding := range in {
		if !finding.RequiresDBAApproval {
			out = append(out, finding)
		}
	}
	return out
}

func approvalValid(approval *api.DBAApproval) bool {
	return approval != nil && approval.Approved &&
		strings.TrimSpace(approval.Approver) != "" &&
		strings.TrimSpace(approval.Reason) != ""
}

func nonEmptyStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, value := range in {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}

func Messages(findings []Finding) string { return mongocheck.Messages(findings) }

func Severity(findings []Finding) int {
	return mongocheck.Severity(findings)
}

func Blocked(findings []Finding) *Finding { return mongocheck.Blocked(findings) }
