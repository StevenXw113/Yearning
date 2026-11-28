package mongo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"engine/api"
	"engine/internal/mongodb"
	"engine/ruleset"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	driver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Explicit write opt-in; random resources are cleaned using fresh contexts.
func TestMongoWriteLimitsIntegration(t *testing.T) {
	uri, database := os.Getenv("MONGO_TEST_URI"), os.Getenv("MONGO_TEST_DB")
	if uri == "" || database == "" || os.Getenv("MONGO_TEST_WRITE") != "1" {
		t.Skip("set MONGO_TEST_URI, MONGO_TEST_DB and MONGO_TEST_WRITE=1 for isolated live writes")
	}
	var mu sync.Mutex
	writes := 0
	monitor := &event.CommandMonitor{Started: func(_ context.Context, e *event.CommandStartedEvent) {
		switch strings.ToLower(e.CommandName) {
		case "insert", "update", "delete", "findandmodify":
			mu.Lock()
			writes++
			mu.Unlock()
		}
	}}
	writeCount := func() int { mu.Lock(); defer mu.Unlock(); return writes }
	client, err := driver.Connect(options.Client().ApplyURI(uri).SetMonitor(monitor).SetServerSelectionTimeout(12 * time.Second))
	if err != nil {
		t.Fatal("invalid MongoDB integration configuration")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = client.Disconnect(ctx)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	if err := client.Ping(ctx, nil); err != nil {
		t.Fatal(err)
	}
	db := client.Database(database)
	engine := New(client)
	token := make([]byte, 12)
	if _, err := rand.Read(token); err != nil {
		t.Fatal(err)
	}
	prefix := "__yearning_live_" + hex.EncodeToString(token)
	approval := &api.DBAApproval{Approved: true, Approver: "integration-test", Reason: "user-authorized isolated live validation"}
	execute := func(t *testing.T, command bson.D, cap int, approved bool) Result {
		t.Helper()
		data, err := bson.MarshalExtJSON(command, false, false)
		if err != nil {
			t.Fatal(err)
		}
		def := ruleset.MongoDefinitionFromAPI(nil)
		def.Settings.MaxAffectRows = cap
		var a *api.DBAApproval
		if approved {
			a = approval
		}
		result, err := engine.ExecuteDefinition(ctx, Order{Database: database, Command: string(data)}, def, a)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	success := func(t *testing.T, result Result) {
		t.Helper()
		if !result.Ok || result.Status != "executed" || result.Error != nil {
			t.Fatalf("execution: status=%s error=%v", result.Status, result.Error)
		}
	}
	fresh := func(t *testing.T, suffix string, n int) *driver.Collection {
		t.Helper()
		name := prefix + "_" + suffix
		// Never pre-drop: a name collision must fail without touching existing data.
		success(t, execute(t, bson.D{{Key: "create", Value: name}}, 0, false))
		c := db.Collection(name)
		t.Cleanup(func() {
			cleanup, done := context.WithTimeout(context.Background(), 15*time.Second)
			defer done()
			if err := c.Drop(cleanup); err != nil {
				t.Errorf("cleanup collection %s: %v", name, err)
			}
			names, err := db.ListCollectionNames(cleanup, bson.D{{Key: "name", Value: name}})
			if err != nil || len(names) != 0 {
				t.Errorf("collection cleanup verification %s: names=%v error=%v", name, names, err)
			}
		})
		if n > 0 {
			docs := make([]interface{}, n)
			for i := range docs {
				docs[i] = bson.M{"_id": i, "v": 0}
			}
			if _, err := c.InsertMany(ctx, docs); err != nil {
				t.Fatal(err)
			}
		}
		return c
	}
	snapshot := func(t *testing.T, c *driver.Collection) []bson.M {
		t.Helper()
		cur, err := c.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
		if err != nil {
			t.Fatal(err)
		}
		defer cur.Close(ctx)
		var docs []bson.M
		if err := cur.All(ctx, &docs); err != nil {
			t.Fatal(err)
		}
		return docs
	}
	rollback := func(t *testing.T, c *driver.Collection, out Result, before []bson.M) {
		t.Helper()
		if len(out.Rollback) == 0 {
			t.Fatalf("missing rollback: affected=%d warning=%s", out.Affected, out.Warning)
		}
		for _, text := range out.Rollback {
			var cmd bson.D
			if err := bson.UnmarshalExtJSON([]byte(text), false, &cmd); err != nil {
				t.Fatal(err)
			}
			success(t, execute(t, cmd, 0, true))
		}
		if after := snapshot(t, c); !reflect.DeepEqual(before, after) {
			t.Fatalf("rollback differs: before=%d after=%d", len(before), len(after))
		}
	}
	command := func(kind, coll string, n int) bson.D {
		filter := bson.D{{Key: "_id", Value: bson.D{{Key: "$lt", Value: n}}}}
		if kind == "update" {
			return bson.D{{Key: "update", Value: coll}, {Key: "updates", Value: bson.A{bson.D{{Key: "q", Value: filter}, {Key: "u", Value: bson.M{"$set": bson.M{"v": 1}}}, {Key: "multi", Value: true}}}}}
		}
		return bson.D{{Key: "delete", Value: coll}, {Key: "deletes", Value: bson.A{bson.D{{Key: "q", Value: filter}, {Key: "limit", Value: 0}}}}}
	}
	for _, kind := range []string{"update", "delete"} {
		for _, tc := range []struct {
			name    string
			n, cap  int
			blocked bool
			message string
		}{
			{"below", 2, 3, false, ""}, {"equal", 3, 3, false, ""}, {"over", 4, 3, true, "上限 3"},
			{"snapshot_equal", 1000, 0, false, ""}, {"snapshot_over", 1001, 0, true, "1000"}, {"snapshot_over_large_rule", 1001, 2000, true, "1000"},
		} {
			t.Run(kind+"_"+tc.name, func(t *testing.T) {
				c := fresh(t, kind+"_"+tc.name, tc.n)
				before := snapshot(t, c)
				sent := writeCount()
				out := execute(t, command(kind, c.Name(), tc.n), tc.cap, false)
				if tc.blocked {
					if out.Ok || out.Error == nil || !strings.Contains(out.Error.Error(), tc.message) || out.Affected != 0 || len(out.Rollback) != 0 {
						t.Fatalf("expected pre-write block: %+v", out)
					}
					if writeCount() != sent {
						t.Fatal("blocked command reached MongoDB")
					}
					if !reflect.DeepEqual(before, snapshot(t, c)) {
						t.Fatal("blocked command changed data")
					}
					t.Logf("blocked before network write: matched=%d rule_cap=%d snapshot_cap=%d", tc.n, tc.cap, mongodb.MaxSnapshot)
					return
				}
				success(t, out)
				if out.Affected != int64(tc.n) {
					t.Fatalf("affected=%d want=%d", out.Affected, tc.n)
				}
				if kind == "update" {
					count, err := c.CountDocuments(ctx, bson.M{"v": 1})
					if err != nil || count != int64(tc.n) {
						t.Fatalf("actual updated=%d error=%v", count, err)
					}
				} else if len(snapshot(t, c)) != 0 {
					t.Fatal("delete did not remove all matches")
				}
				rollback(t, c, out, before)
				t.Logf("affected=%d; rollback restored %d documents", out.Affected, len(before))
			})
		}
	}
	t.Run("overlapping_filters", func(t *testing.T) {
		c := fresh(t, "overlap", 3)
		before := snapshot(t, c)
		cmd := command("update", c.Name(), 3)
		clauses := cmd[1].Value.(bson.A)
		cmd[1].Value = append(clauses, clauses[0])
		out := execute(t, cmd, 3, false)
		success(t, out)
		if out.Affected != 6 {
			t.Fatalf("affected=%d want=6 matched operations on 3 distinct documents", out.Affected)
		}
		rollback(t, c, out, before)
		t.Log("overlap uses 3 distinct snapshots; server reports n=6")
	})
	for _, kind := range []string{"update", "delete"} {
		for _, tc := range []struct {
			name          string
			n, split, cap int
			message       string
		}{
			{"rule", 4, 2, 3, "上限 3"}, {"snapshot", 1001, 600, 0, "1000"},
		} {
			t.Run(kind+"_disjoint_"+tc.name, func(t *testing.T) {
				c := fresh(t, kind+"_disjoint_"+tc.name, tc.n)
				before := snapshot(t, c)
				cmd := command(kind, c.Name(), tc.split)
				second := command(kind, c.Name(), tc.n)[1].Value.(bson.A)[0].(bson.D)
				second[0].Value = bson.M{"_id": bson.M{"$gte": tc.split}}
				cmd[1].Value = append(cmd[1].Value.(bson.A), second)
				sent := writeCount()
				out := execute(t, cmd, tc.cap, false)
				if out.Ok || out.Error == nil || !strings.Contains(out.Error.Error(), tc.message) || writeCount() != sent {
					t.Fatalf("disjoint union bypassed cap: %+v", out)
				}
				if !reflect.DeepEqual(before, snapshot(t, c)) {
					t.Fatal("blocked disjoint batch changed data")
				}
			})
		}
	}
	t.Run("update_noop", func(t *testing.T) {
		c := fresh(t, "noop", 3)
		cmd := command("update", c.Name(), 3)
		clause := cmd[1].Value.(bson.A)[0].(bson.D)
		clause[1].Value = bson.M{"$set": bson.M{"v": 0}}
		before := snapshot(t, c)
		out := execute(t, cmd, 3, false)
		success(t, out)
		if out.Affected != 3 || !reflect.DeepEqual(before, snapshot(t, c)) {
			t.Fatalf("no-op matched rows=%d want=3", out.Affected)
		}
		rollback(t, c, out, before)
	})
	t.Run("typed_ids", func(t *testing.T) {
		c := fresh(t, "typed", 0)
		docs := []interface{}{bson.M{"_id": 1, "v": 0}, bson.M{"_id": "1", "v": 0}}
		if _, err := c.InsertMany(ctx, docs); err != nil {
			t.Fatal(err)
		}
		before := snapshot(t, c)
		cmd := bson.D{{Key: "delete", Value: c.Name()}, {Key: "deletes", Value: bson.A{bson.D{{Key: "q", Value: bson.M{"v": 0}}, {Key: "limit", Value: 0}}}}}
		sent := writeCount()
		blocked := execute(t, cmd, 1, false)
		if blocked.Ok || blocked.Error == nil || writeCount() != sent {
			t.Errorf("different BSON IDs must count separately and block: %+v", blocked)
		}
		if !reflect.DeepEqual(before, snapshot(t, c)) {
			if _, err := c.InsertMany(ctx, docs); err != nil {
				t.Fatal(err)
			}
		}
		out := execute(t, cmd, 2, false)
		success(t, out)
		if out.Affected != 2 {
			t.Fatalf("affected=%d", out.Affected)
		}
		rollback(t, c, out, before)
	})
	t.Run("insert_limit_and_rollback", func(t *testing.T) {
		c := fresh(t, "insert", 0)
		before := snapshot(t, c)
		cmd := bson.D{{Key: "insert", Value: c.Name()}, {Key: "documents", Value: bson.A{bson.M{"_id": 1}, bson.M{"_id": 2}, bson.M{"_id": 3}}}}
		sent := writeCount()
		out := execute(t, cmd, 2, false)
		if out.Ok || out.Error == nil || writeCount() != sent {
			t.Fatalf("insert must honor affected-row cap before writing: %+v", out)
		}
		out = execute(t, cmd, 3, false)
		success(t, out)
		if out.Affected != 3 {
			t.Fatalf("affected=%d want=3", out.Affected)
		}
		rollback(t, c, out, before)
	})
	t.Run("findAndModify_rollback", func(t *testing.T) {
		c := fresh(t, "fam", 2)
		for _, remove := range []bool{false, true} {
			before := snapshot(t, c)
			cmd := bson.D{{Key: "findAndModify", Value: c.Name()}, {Key: "query", Value: bson.M{"_id": 0}}, {Key: "new", Value: true}, {Key: "fields", Value: bson.M{"_id": 1}}}
			if remove {
				cmd = cmd[:2]
				cmd = append(cmd, bson.E{Key: "remove", Value: true})
			} else {
				cmd = append(cmd, bson.E{Key: "update", Value: bson.M{"$set": bson.M{"v": 1}}})
			}
			out := execute(t, cmd, 1, false)
			success(t, out)
			if out.Affected != 1 {
				t.Fatalf("affected=%d", out.Affected)
			}
			rollback(t, c, out, before)
		}
		out := execute(t, bson.D{{Key: "findAndModify", Value: c.Name()}, {Key: "query", Value: bson.M{"_id": -1}}, {Key: "remove", Value: true}}, 1, false)
		success(t, out)
		if out.Affected != 0 {
			t.Fatalf("unmatched findAndModify affected=%d want=0", out.Affected)
		}
		before := snapshot(t, c)
		out = execute(t, bson.D{{Key: "findAndModify", Value: c.Name()}, {Key: "query", Value: bson.M{"_id": 99}}, {Key: "upsert", Value: true}, {Key: "update", Value: bson.M{"$set": bson.M{"v": 1}}}, {Key: "new", Value: true}}, 1, false)
		success(t, out)
		if out.Affected != 1 {
			t.Fatalf("upserted findAndModify affected=%d want=1", out.Affected)
		}
		rollback(t, c, out, before)
	})
	t.Run("write_error", func(t *testing.T) {
		c := fresh(t, "error", 1)
		out := execute(t, bson.D{{Key: "insert", Value: c.Name()}, {Key: "documents", Value: bson.A{bson.M{"_id": 0}}}}, 1, false)
		if out.Ok || out.Error == nil {
			t.Fatalf("duplicate key reported success: %+v", out)
		}
	})
	t.Run("ddl_and_index_rollback", func(t *testing.T) {
		c := fresh(t, "ddl", 2)
		index := bson.D{{Key: "key", Value: bson.M{"v": 1}}, {Key: "name", Value: "live_v_1"}}
		create := bson.D{{Key: "createIndexes", Value: c.Name()}, {Key: "indexes", Value: bson.A{index}}}
		indexNames := func() []string {
			cur, err := c.Indexes().List(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer cur.Close(ctx)
			var specs []bson.M
			if err := cur.All(ctx, &specs); err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, s := range specs {
				names = append(names, s["name"].(string))
			}
			return names
		}
		out := execute(t, create, 0, false)
		success(t, out)
		if len(indexNames()) != 2 || len(out.Rollback) != 1 {
			t.Fatal("index create/rollback missing")
		}
		var undo bson.D
		if err := bson.UnmarshalExtJSON([]byte(out.Rollback[0]), false, &undo); err != nil {
			t.Fatal(err)
		}
		dropped := execute(t, undo, 0, false)
		success(t, dropped)
		if len(indexNames()) != 1 || len(dropped.Rollback) != 1 {
			t.Fatal("index drop/rollback missing")
		}
		if err := bson.UnmarshalExtJSON([]byte(dropped.Rollback[0]), false, &undo); err != nil {
			t.Fatal(err)
		}
		success(t, execute(t, undo, 0, false))
		if len(indexNames()) != 2 {
			t.Fatal("index rollback did not restore index")
		}
		rename := bson.D{{Key: "renameCollection", Value: database + "." + c.Name()}, {Key: "to", Value: database + "." + c.Name() + "_renamed"}}
		text, _ := bson.MarshalExtJSON(rename, false, false)
		def := ruleset.MongoDefinitionFromAPI(nil)
		blocked, err := engine.ExecuteDefinition(ctx, Order{Database: "admin", Command: string(text)}, def, nil)
		if err != nil || blocked.Ok || blocked.Status != "blocked" {
			t.Fatalf("rename approval block: %+v %v", blocked, err)
		}
		renamed := db.Collection(c.Name() + "_renamed")

		result, err := engine.ExecuteDefinition(ctx, Order{Database: "admin", Command: string(text)}, def, approval)
		if err != nil {
			t.Fatal(err)
		}
		success(t, result)
		t.Cleanup(func() {
			cleanup, done := context.WithTimeout(context.Background(), 15*time.Second)
			defer done()
			if err := renamed.Drop(cleanup); err != nil {
				t.Errorf("rename cleanup: %v", err)
			}
		})
		count, err := renamed.CountDocuments(ctx, bson.D{})
		if err != nil || count != 2 {
			t.Fatalf("rename data count=%d error=%v", count, err)
		}
		rename[0].Value, rename[1].Value = rename[1].Value, rename[0].Value
		text, _ = bson.MarshalExtJSON(rename, false, false)
		result, err = engine.ExecuteDefinition(ctx, Order{Database: "admin", Command: string(text)}, def, approval)
		if err != nil {
			t.Fatal(err)
		}
		success(t, result)
		drop := bson.D{{Key: "drop", Value: c.Name()}}
		if out := execute(t, drop, 0, false); out.Ok || out.Status != "blocked" {
			t.Fatal("drop without approval allowed")
		}
		success(t, execute(t, drop, 0, true))
		names, err := db.ListCollectionNames(ctx, bson.M{"name": c.Name()})
		if err != nil || len(names) != 0 {
			t.Fatal("approved drop did not remove collection")
		}
	})
	t.Run("permissions", func(t *testing.T) {
		c := fresh(t, "auth", 1)
		user, role := prefix+"_user", prefix+"_role"
		secret := make([]byte, 24)
		if _, err := rand.Read(secret); err != nil {
			t.Fatal(err)
		}
		pwd := hex.EncodeToString(secret)
		roles := bson.A{bson.D{{Key: "role", Value: role}, {Key: "db", Value: database}}}
		createRole := bson.D{{Key: "createRole", Value: role}, {Key: "privileges", Value: bson.A{bson.D{{Key: "resource", Value: bson.M{"db": database, "collection": c.Name()}}, {Key: "actions", Value: bson.A{"find"}}}}}, {Key: "roles", Value: bson.A{}}}
		createUser := bson.D{{Key: "createUser", Value: user}, {Key: "pwd", Value: pwd}, {Key: "roles", Value: bson.A{}}}
		if out := execute(t, createUser, 0, false); out.Ok || out.Status != "blocked" {
			t.Fatal("user without approval allowed")
		}
		success(t, execute(t, createRole, 0, true))
		roleDropped := false
		t.Cleanup(func() {
			cleanup, done := context.WithTimeout(context.Background(), 15*time.Second)
			defer done()
			if roleDropped {
				return
			}
			if err := db.RunCommand(cleanup, bson.D{{Key: "dropRole", Value: role}}).Err(); err != nil {
				t.Errorf("cleanup role: %v", err)
			}
		})
		success(t, execute(t, createUser, 0, true))
		userDropped := false
		t.Cleanup(func() {
			if userDropped {
				return
			}
			cleanup, done := context.WithTimeout(context.Background(), 15*time.Second)
			defer done()
			if err := db.RunCommand(cleanup, bson.D{{Key: "dropUser", Value: user}}).Err(); err != nil {
				t.Errorf("cleanup user: %v", err)
			}
		})
		readAsUser := func(wantAllowed bool) {
			t.Helper()
			u, err := driver.Connect(options.Client().ApplyURI(uri).SetAuth(options.Credential{AuthSource: database, Username: user, Password: pwd}).SetServerSelectionTimeout(10 * time.Second))
			if err != nil {
				t.Fatal("temporary user connection configuration")
			}
			defer func() {
				cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
				defer done()
				_ = u.Disconnect(cleanup)
			}()
			err = u.Database(database).Collection(c.Name()).FindOne(ctx, bson.M{"_id": 0}).Err()
			if wantAllowed && err != nil {
				t.Fatalf("granted read failed: %v", err)
			}
			if !wantAllowed {
				var ce driver.CommandError
				if !errors.As(err, &ce) || ce.Code != 13 {
					t.Fatalf("expected authorization denial, got %v", err)
				}
			}
		}
		readAsUser(false)
		success(t, execute(t, bson.D{{Key: "grantRolesToUser", Value: user}, {Key: "roles", Value: roles}}, 0, true))
		readAsUser(true)
		success(t, execute(t, bson.D{{Key: "revokeRolesFromUser", Value: user}, {Key: "roles", Value: roles}}, 0, true))
		readAsUser(false)
		success(t, execute(t, bson.D{{Key: "dropUser", Value: user}}, 0, true))
		userDropped = true
		success(t, execute(t, bson.D{{Key: "dropRole", Value: role}}, 0, true))
		roleDropped = true
		for _, check := range []struct{ command, name, field string }{{"usersInfo", user, "users"}, {"rolesInfo", role, "roles"}} {
			var raw bson.M
			if err := db.RunCommand(ctx, bson.D{{Key: check.command, Value: check.name}}).Decode(&raw); err != nil {
				t.Fatal(err)
			}
			if items, ok := raw[check.field].(bson.A); !ok || len(items) != 0 {
				t.Fatalf("temporary %s remains", check.field)
			}
		}
		t.Log("verified deny/grant/revoke with a collection-scoped read role; verified user and role removed")
	})
	names, err := db.ListCollectionNames(ctx, bson.M{"name": bson.M{"$regex": "^" + prefix}})
	if err != nil || len(names) != 0 {
		t.Errorf("owned collections remain: %v error=%v", names, err)
	}
	t.Logf("live validation finished; namespace prefix=%s", prefix)
}
