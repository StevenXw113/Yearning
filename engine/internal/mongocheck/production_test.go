package mongocheck

import "testing"

func TestProductionQuerySafety(t *testing.T) {
	for _, text := range []string{
		`{"unrecognized":1}`,
		`{"aggregate":"orders","pipeline":[{"$out":"backup"}],"cursor":{}}`,
		`{"explain":{"aggregate":"orders","pipeline":[{"$merge":"backup"}],"cursor":{}}}`,
		`{"find":"orders","filter":{"$where":"true"}}`,
		`{"aggregate":"orders","pipeline":[{"$project":{"x":{"$function":{"body":"function(){}","args":[],"lang":"js"}}}}]}`,
		`{"find":"orders","filter":{"x":1,"x":2}}`,
		`{"find":"orders","find":"other"}`,
		`{"find":"orders"} {"find":"other"}`,
	} {
		if _, _, err := Check(text, Config{Mode: ModeQuery}); err == nil {
			t.Errorf("查询硬校验应拒绝 %s", text)
		}
	}
	for _, text := range []string{
		`{"find":"orders","filter":{"_id":{"$oid":"507f1f77bcf86cd799439011"}}}`,
		`{"explain":{"find":"orders","filter":{"_id":1}}}`,
	} {
		if _, _, err := Check(text, Config{Mode: ModeQuery}); err != nil {
			t.Errorf("查询硬校验拒绝了 %s: %v", text, err)
		}
	}
}

func TestProductionQueryHints(t *testing.T) {
	cfg := Config{Mode: ModeQuery, QueryForbidNoFilter: true, QueryForbidNoLimit: true}
	for _, tc := range []struct {
		text, rule string
		want       bool
	}{
		{`{"aggregate":"orders","pipeline":[{"$match":{"tenant":1}}]}`, RuleQueryNoFilter, false},
		{`{"aggregate":"orders","pipeline":[{"$match":{}}]}`, RuleQueryNoFilter, true},
		{`{"aggregate":"orders","pipeline":[{"$sort":{"a":1}},{"$match":{"tenant":1}}]}`, RuleQueryNoFilter, true},
		{`{"aggregate":"orders","pipeline":[{"$lookup":{"from":"items","pipeline":[{"$match":{"a":1}}],"as":"items"}}]}`, RuleQueryNoFilter, true},
		{`{"find":"orders","filter":{"sort":1}}`, RuleQueryNoLimit, false},
		{`{"find":"orders","filter":{"x":1},"sort":{"_id":1},"limit":{"$numberLong":"10"}}`, RuleQueryNoLimit, false},
		{`{"find":"orders","filter":{"x":1},"sort":{"_id":1},"limit":1.5}`, RuleQueryNoLimit, true},
		{`{"explain":{"find":"orders","sort":{"_id":1}}}`, RuleQueryNoLimit, true},
	} {
		_, hits, err := Check(tc.text, cfg)
		if err != nil || hasRule(hits, tc.rule) != tc.want {
			t.Errorf("Check(%s) = %+v, %v; want %s hit=%v", tc.text, hits, err, tc.rule, tc.want)
		}
	}
}
func TestProductionWriteRules(t *testing.T) {
	for _, tc := range []struct {
		text, rule string
		want       bool
	}{
		{`{"update":"orders","updates":[{"q":{"_id":1},"u":{"$set":{"upsert":true}}},{"q":{"_id":2},"u":{"$set":{"a":1}},"upsert":true}]}`, "MongoForbidUpsert", true},
		{`{"update":"orders","updates":[{"q":{"_id":1},"u":{"$set":{"upsert":true}}}]}`, "MongoForbidUpsert", false},
		{`{"findAndModify":"orders","query":{"_id":1},"update":{"$set":{"a":1}},"upsert":true}`, "MongoForbidUpsert", true},
		{`{"update":"orders","updates":[{"q":{"a":1},"u":{"$set":{"a":2}},"multi":true}]}`, "MongoForbidMultiWrite", true},
		{`{"delete":"orders","deletes":[{"q":{"a":1},"limit":{"$numberInt":"0"}}]}`, "MongoForbidMultiWrite", true},
		{`{"delete":"orders","deletes":[{"q":{"a":1},"limit":0.0}]}`, "MongoForbidMultiWrite", true},
		{`{"delete":"orders","deletes":[{"q":{"_id":1},"limit":1},{"q":{"_id":2},"limit":1}]}`, "MongoForbidMultiWrite", false},
	} {
		cfg := Config{Levels: map[string]string{tc.rule: "warn"}}
		if tc.rule == "MongoForbidUpsert" {
			cfg.ForbidUpsert = true
		} else {
			cfg.ForbidMultiWrite = true
		}
		defaults := cfg
		defaults.Levels = nil
		if _, hits, err := Check(tc.text, defaults); err != nil || (Blocked(hits) != nil) != tc.want {
			t.Errorf("默认级别 Check(%s) = %+v, %v; want blocked=%v", tc.text, hits, err, tc.want)
		}
		_, hits, err := Check(tc.text, cfg)
		if err != nil || hasRule(hits, tc.rule) != tc.want || Blocked(hits) != nil {
			t.Errorf("Check(%s) = %+v, %v; want %s hit=%v at warn", tc.text, hits, err, tc.rule, tc.want)
		}
		if _, hits, err := Check(tc.text, Config{}); err != nil || len(hits) > 0 {
			t.Errorf("默认关闭时 Check(%s) = %+v, %v", tc.text, hits, err)
		}
	}
}
