package mongo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"engine/ruleset"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	driver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Opt-in and read-only: use a fresh namespace without creating any collections.
func TestMongoRulesReadOnlyIntegration(t *testing.T) {
	uri := os.Getenv("MONGO_TEST_URI")
	if uri == "" {
		t.Skip("set MONGO_TEST_URI to run read-only rule integration")
	}
	database := os.Getenv("MONGO_TEST_DB")
	if database == "" {
		database = "test"
	}
	var mu sync.Mutex
	var commands []string
	monitor := &event.CommandMonitor{Started: func(_ context.Context, e *event.CommandStartedEvent) {
		mu.Lock()
		defer mu.Unlock()
		commands = append(commands, e.CommandName)
	}}
	client, err := driver.Connect(options.Client().ApplyURI(uri).SetConnectTimeout(8 * time.Second).SetServerSelectionTimeout(12 * time.Second).SetMonitor(monitor))
	if err != nil {
		t.Fatal("invalid MongoDB integration configuration")
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = client.Disconnect(ctx)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := client.Ping(ctx, nil); err != nil {
		t.Fatalf("authenticated ping: %v", err)
	}
	before, err := client.Database(database).ListCollectionNames(ctx, bson.D{})
	if err != nil {
		t.Fatalf("list collections: %v", err)
	}
	coll := fmt.Sprintf("__yearning_rules_readonly_%d", time.Now().UnixNano())
	find := func(filter string, sort bool, limit bool) string {
		command := `{"find":"` + coll + `","filter":` + filter + `,"singleBatch":true`
		if sort {
			command += `,"sort":{"_id":1}`
		}
		if limit {
			command += `,"limit":1`
		}
		return command + `}`
	}
	aggregate := func(lookup bool) string {
		pipeline := `[{"$match":{"_id":"__yearning_nonexistent__"}}`
		if lookup {
			pipeline += `,{"$lookup":{"from":"` + coll + `","localField":"_id","foreignField":"_id","as":"matches"}}`
		}
		return `{"aggregate":"` + coll + `","pipeline":` + pipeline + `,{"$limit":1}],"cursor":{}}`
	}
	cases := []struct{ id, bad, good, params string }{
		{"mongo.regex_unanchored", find(`{"name":{"$regex":"abc"}}`, false, true), find(`{"name":{"$regex":"^abc"}}`, false, true), ""},
		{"mongo.negation_operator", find(`{"x":{"$ne":1}}`, false, true), find(`{"x":{"$eq":1}}`, false, true), ""},
		{"mongo.or_clause", find(`{"$or":[{"x":1},{"x":2}]}`, false, true), find(`{"$and":[{"x":1},{"y":2}]}`, false, true), ""},
		{"mongo.large_in_list", find(`{"x":{"$in":[1,2,3]}}`, false, true), find(`{"x":{"$in":[1,2]}}`, false, true), `{"max_items":2}`},
		{"mongo.query.no_filter", find(`{}`, false, true), find(`{"_id":1}`, false, true), ""},
		{"mongo.query.no_lookup", aggregate(true), aggregate(false), ""},
		{"mongo.query.no_limit", find(`{"_id":1}`, true, false), find(`{"_id":1}`, true, true), ""},
	}
	engine := New(client)
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			for _, level := range []ruleset.Level{ruleset.LevelError, ruleset.LevelWarn, ruleset.LevelObserve} {
				t.Run(string(level), func(t *testing.T) {
					def := ruleset.Definition{SchemaVersion: ruleset.SchemaVersion, Dialect: ruleset.DialectMongoDB, Scope: ruleset.ScopeDataSource,
						Rules: []ruleset.Rule{{ID: tc.id, Enabled: true, Level: level, Params: json.RawMessage(tc.params)}}}
					mu.Lock()
					count := len(commands)
					mu.Unlock()
					input := Input{Database: database, Mode: ModeQuery, Command: tc.bad}
					decision, err := engine.AuditDefinition(ctx, input, def)
					mu.Lock()
					afterAudit := len(commands)
					mu.Unlock()
					if afterAudit != count {
						t.Fatal("static rule audit unexpectedly contacted MongoDB")
					}
					if err != nil || decision.Error != nil || len(decision.Findings) != 1 || decision.Findings[0].RuleID != tc.id || decision.Allowed != (level != ruleset.LevelError) {
						t.Fatalf("unexpected rule decision: %+v, %v", decision, err)
					}
					if decision.Allowed {
						if _, err := engine.Query(ctx, input); err != nil {
							t.Fatalf("allowed read-only command: %v", err)
						}
					}
					input.Command = tc.good
					safe, err := engine.AuditDefinition(ctx, input, def)
					if err != nil || safe.Error != nil || !safe.Allowed || len(safe.Findings) != 0 {
						t.Fatalf("safe query rejected: %+v, %v", safe, err)
					}
					if _, err := engine.Query(ctx, input); err != nil {
						t.Fatalf("safe read-only command: %v", err)
					}
				})
			}
		})
	}
	after, err := client.Database(database).ListCollectionNames(ctx, bson.D{})
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("collection count changed during read-only test: before=%d after=%d", len(before), len(after))
	}
	mu.Lock()
	defer mu.Unlock()
	for _, command := range commands {
		switch command {
		case "ping", "listCollections", "find", "aggregate", "killCursors":
		default:
			t.Errorf("unexpected command sent to live source: %s", command)
		}
	}
	t.Logf("read-only rule matrix complete: rules=%d, monitored commands=%d, collections=%d", len(cases), len(commands), len(after))
}
