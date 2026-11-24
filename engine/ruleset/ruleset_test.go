package ruleset

import (
	"encoding/json"
	"testing"
)

func TestValidateRejectsUnknownVersionAndRules(t *testing.T) {
	base := Definition{SchemaVersion: SchemaVersion, Dialect: DialectMongoDB, Scope: ScopeDataSource}
	cases := []struct {
		name string
		set  Definition
		want string
	}{
		{"version", Definition{SchemaVersion: 1, Dialect: DialectMongoDB}, "unsupported"},
		{"dialect", Definition{SchemaVersion: SchemaVersion}, "dialect"},
		{"unknown-rule", func() Definition {
			d := base
			d.Rules = []Rule{{ID: "mongo.unknown", Enabled: true, Level: LevelWarn}}
			return d
		}(), "unknown rule"},
		{"level", func() Definition {
			d := base
			d.Rules = []Rule{{ID: "mongo.multi_write", Enabled: true, Level: "fatal"}}
			return d
		}(), "invalid level"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.set.Validate(map[string]struct{}{"mongo.multi_write": {}})
			if err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestDecodeAndLookup(t *testing.T) {
	data := []byte(`{"schema_version":2,"dialect":"mongodb","rules":[{"id":"mongo.multi_write","enabled":true,"level":"warn"}]}`)
	got, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled("mongo.multi_write") || got.LevelOf("mongo.multi_write", LevelError) != LevelWarn {
		t.Fatalf("unexpected definition: %+v", got)
	}
	if _, err := json.Marshal(got); err != nil {
		t.Fatal(err)
	}
}
