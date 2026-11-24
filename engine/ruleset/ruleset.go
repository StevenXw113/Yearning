package ruleset

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const SchemaVersion = 2

type Dialect string

const (
	DialectGlobal   Dialect = "global"
	DialectMySQL    Dialect = "mysql"
	DialectPostgres Dialect = "postgres"
	DialectMongoDB  Dialect = "mongodb"
	DialectRedis    Dialect = "redis"
	DialectDoris    Dialect = "doris"
)

type Scope string

const (
	ScopeGlobal     Scope = "global"
	ScopeDataSource Scope = "data_source"
)

type Level string

const (
	LevelError   Level = "error"
	LevelWarn    Level = "warn"
	LevelObserve Level = "observe"
)

type Settings struct {
	ProductionAuditRequired bool   `json:"production_audit_required"`
	AuditUnavailable        string `json:"audit_unavailable"`
	ExecutionTimeoutMS      int    `json:"execution_timeout_ms"`
	MaxResultRows           int    `json:"max_result_rows"`
	MaxResultBytes          int    `json:"max_result_bytes"`
	MaxAffectRows           int    `json:"max_affect_rows"`
}

type Rule struct {
	ID      string          `json:"id"`
	Enabled bool            `json:"enabled"`
	Level   Level           `json:"level"`
	Params  json.RawMessage `json:"params,omitempty"`
	Reason  string          `json:"reason,omitempty"`
}

type Definition struct {
	SchemaVersion int      `json:"schema_version"`
	Dialect       Dialect  `json:"dialect"`
	Dialects      []string `json:"dialects,omitempty"`
	Scope         Scope    `json:"scope"`
	Settings      Settings `json:"settings"`
	Rules         []Rule   `json:"rules"`
}

func (d Definition) Validate(known map[string]struct{}) error {
	if d.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported rule set schema version %d", d.SchemaVersion)
	}
	if d.Dialect == "" {
		return errors.New("rule set dialect is required")
	}
	if d.Scope == "" {
		return errors.New("rule set scope is required")
	}
	switch d.Scope {
	case ScopeGlobal, ScopeDataSource:
	default:
		return fmt.Errorf("invalid rule set scope %q", d.Scope)
	}
	if d.Scope == ScopeGlobal && d.Dialect != DialectGlobal {
		return fmt.Errorf("global rule set dialect must be %q", DialectGlobal)
	}
	if d.Settings.AuditUnavailable != "" {
		switch strings.ToLower(d.Settings.AuditUnavailable) {
		case "block", "allow":
		default:
			return fmt.Errorf("invalid audit_unavailable setting %q", d.Settings.AuditUnavailable)
		}
	}
	if d.Settings.ExecutionTimeoutMS < 0 || d.Settings.MaxResultRows < 0 || d.Settings.MaxResultBytes < 0 || d.Settings.MaxAffectRows < 0 {
		return errors.New("rule set limits cannot be negative")
	}
	seen := make(map[string]struct{}, len(d.Rules))
	for _, rule := range d.Rules {
		if rule.ID == "" {
			return errors.New("rule id is required")
		}
		if _, duplicate := seen[rule.ID]; duplicate {
			return fmt.Errorf("duplicate rule id %q", rule.ID)
		}
		seen[rule.ID] = struct{}{}
		if known != nil {
			if _, ok := known[rule.ID]; !ok {
				return fmt.Errorf("unknown rule id %q", rule.ID)
			}
		}
		switch rule.Level {
		case LevelError, LevelWarn, LevelObserve:
		default:
			return fmt.Errorf("invalid level %q for rule %q", rule.Level, rule.ID)
		}
	}
	return nil
}

func (d Definition) Rule(id string) (Rule, bool) {
	for _, rule := range d.Rules {
		if rule.ID == id {
			return rule, true
		}
	}
	return Rule{}, false
}

func (d Definition) Enabled(id string) bool {
	rule, ok := d.Rule(id)
	return ok && rule.Enabled
}

func (d Definition) LevelOf(id string, fallback Level) Level {
	if rule, ok := d.Rule(id); ok && rule.Level != "" {
		return rule.Level
	}
	return fallback
}

func Decode(data []byte) (Definition, error) {
	var out Definition
	if len(data) == 0 {
		return out, errors.New("rule set definition is empty")
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, fmt.Errorf("invalid rule set definition: %w", err)
	}
	return out, nil
}
