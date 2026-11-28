package mongoguard

import (
	"go.mongodb.org/mongo-driver/v2/bson"
	"testing"
)

func TestQueryBoundary(t *testing.T) {
	cases := []struct {
		name, command, auditName string
		allowed                  bool
	}{
		{"find", `{"find":"users","filter":{"_id":1},"limit":1}`, "find", true},
		{"aggregate", `{"aggregate":"users","pipeline":[{"$match":{"_id":1}},{"$limit":1}],"cursor":{}}`, "aggregate", true},
		{"count", `{"count":"users","query":{"_id":1}}`, "count", true},
		{"distinct", `{"distinct":"users","key":"name","query":{"_id":1}}`, "distinct", true},
		{"catalog", `{"listCollections":1,"nameOnly":true}`, "listCollections", true},
		{"indexes", `{"listIndexes":"users"}`, "listIndexes", true},
		{"stats", `{"collStats":"users"}`, "collStats", true},
		{"cursor", `{"getMore":{"$numberLong":"123"},"collection":"users"}`, "getMore", true},
		{"ping", `{"ping":1}`, "ping", true},
		{"explain_find", `{"explain":{"find":"users","filter":{"_id":1}}}`, "find", true},
		{"explain_aggregate", `{"explain":{"aggregate":"users","pipeline":[{"$match":{"x":1}}],"cursor":{}}}`, "aggregate", true},
		{"insert", `{"insert":"users","documents":[{"x":1}]}`, "", false},
		{"update", `{"update":"users","updates":[{"q":{"x":1},"u":{"$set":{"x":2}}}]}`, "", false},
		{"delete", `{"delete":"users","deletes":[{"q":{"x":1},"limit":1}]}`, "", false},
		{"drop", `{"drop":"users"}`, "", false},
		{"account", `{"createUser":"reader","pwd":"test-only","roles":[]}`, "", false},
		{"unknown", `{"unknownCommand":1}`, "", false},
		{"database_enumeration", `{"listDatabases":1}`, "", false},
		{"aggregate_out", `{"aggregate":"users","pipeline":[{"$out":"backup"}],"cursor":{}}`, "", false},
		{"aggregate_merge", `{"aggregate":"users","pipeline":[{"$merge":{"into":"backup"}}],"cursor":{}}`, "", false},
		{"explain_write", `{"explain":{"delete":"users","deletes":[{"q":{},"limit":0}]}}`, "", false},
		{"explain_out", `{"explain":{"aggregate":"users","pipeline":[{"$out":"backup"}],"cursor":{}}}`, "", false},
		{"explain_script", `{"explain":{"find":"users","filter":{"$where":"true"}}}`, "", false},
		{"explain_recursive", `{"explain":{"explain":{"find":"users"}}}`, "", false},
		{"explain_scalar", `{"explain":"find"}`, "", false},
		{"explain_empty", `{"explain":{}}`, "", false},
		{"where", `{"find":"users","filter":{"$where":"true"}}`, "", false},
		{"nested_where", `{"find":"users","filter":{"$and":[{"x":1},{"$where":"true"}]}}`, "", false},
		{"function", `{"aggregate":"users","pipeline":[{"$project":{"x":{"$function":{"body":"function(){return 1}","args":[],"lang":"js"}}}}],"cursor":{}}`, "", false},
		{"accumulator", `{"aggregate":"users","pipeline":[{"$group":{"_id":null,"x":{"$accumulator":{"init":"function(){return 0}"}}}}],"cursor":{}}`, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd, err := Parse(tc.command)
			if err != nil {
				t.Fatal(err)
			}
			audit, err := ValidateQuery(cmd)
			if (err == nil) != tc.allowed {
				t.Fatalf("allowed=%v, error=%v", tc.allowed, err)
			}
			if tc.allowed && (len(audit) == 0 || audit[0].Key != tc.auditName) {
				t.Fatalf("wrong document audited: %+v", audit)
			}
		})
	}
	if _, err := ValidateQuery(nil); err == nil {
		t.Fatal("empty command must be rejected")
	}
}

func TestParseRejectsAmbiguousCommands(t *testing.T) {
	cases := map[string]string{
		"empty": " ", "array": `[{"ping":1}]`, "scalar": `1`, "empty_object": `{}`,
		"no_command": `{"":1}`, "trailing": `{"ping":1} {"hello":1}`, "syntax": `{"ping":}`,
		"duplicate_command":        `{"find":"users","find":"orders"}`,
		"duplicate_filter":         `{"find":"users","filter":{"x":1,"x":2}}`,
		"duplicate_array_document": `{"aggregate":"users","pipeline":[{"$match":{"x":1,"x":2}}]}`,
		"duplicate_scope":          `{"find":"users","filter":{"x":{"$code":"x","$scope":{"x":1,"x":2}}}}`,
		"invalid_object_id":        `{"find":"users","filter":{"_id":{"$oid":"invalid"}}}`,
	}
	for name, command := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(command); err == nil {
				t.Fatal("ambiguous or malformed command was accepted")
			}
		})
	}
}

func TestParsePreservesCommandOrderAndExtendedJSON(t *testing.T) {
	cmd, err := Parse(`{"find":"users","filter":{"_id":{"$oid":"507f1f77bcf86cd799439011"},"n":{"$numberLong":"9007199254740993"}},"limit":{"$numberInt":"1"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmd) != 3 || cmd[0].Key != "find" || cmd[1].Key != "filter" || cmd[2].Key != "limit" {
		t.Fatalf("command order changed: %+v", cmd)
	}
	filter := cmd[1].Value.(bson.D)
	if _, ok := filter[0].Value.(bson.ObjectID); !ok {
		t.Fatalf("ObjectID type lost: %T", filter[0].Value)
	}
	if n, ok := filter[1].Value.(int64); !ok || n != 9007199254740993 {
		t.Fatalf("int64 precision lost: %v", filter[1].Value)
	}
	if n, ok := cmd[2].Value.(int32); !ok || n != 1 {
		t.Fatalf("int32 type lost: %v", cmd[2].Value)
	}
}
