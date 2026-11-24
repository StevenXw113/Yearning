package mongo

import (
	"testing"

	"engine/api"
)

func TestAuditRejectsWriteCommandInQueryMode(t *testing.T) {
	e := New(nil)
	decision, err := e.Audit(nil, Input{Command: `{"insert":"users","documents":[]}`, Mode: ModeQuery}, &api.AuditRole{})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.Error == nil {
		t.Fatalf("expected query write rejection: %+v", decision)
	}
}

func TestAuditAppliesRuleLevel(t *testing.T) {
	e := New(nil)
	rules := &api.AuditRole{MongoForbidMultiWrite: true}
	decision, err := e.Audit(nil, Input{Command: `{"update":"users","updates":[{"q":{"_id":1},"u":{"$set":{"a":1}},"multi":true}]}`, Mode: ModeWrite}, rules)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed {
		t.Fatalf("expected multi-write rejection: %+v", decision)
	}
}
