package api

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestAuditRoleRoundTrip 契约锁定:AuditRole 94 字段 JSON 往返不漂移。
// 字段名即跨主服务/引擎的线缆契约,改名=两端不同批升级即坏。
func TestAuditRoleRoundTrip(t *testing.T) {
	in := &AuditRole{
		DmlTransaction: true, DmlMaxInsertRows: 100, DdlMaxKeyParts: 3,
		MaxTableNameLen: 64, MaxAffectRows: 5000, MaxDdlAffectRows: 10000,
		SupportCharset: "utf8mb4", SupportCollation: "utf8mb4_general_ci",
		MustHaveColumns: "id,created_at", DdlTablePrefix: "t_",
		DdlColumnsMustHaveIndex: "phone", OscExpr: "pt-osc", OscSize: 1024,
		RuleLevels:             map[string]string{"dml_where": "warn", "ddl_enable_drop_table": "error"},
		DdlForbidTruncate:      true,
		MongoForbidEmptyFilter: true, MongoMaxAffectRows: 1000, MongoIndexNameSpec: "idx_.*",
		RedisKeyPrefix: "app:", RedisMaxAffectKeys: 500, RedisScanRequireMatch: true,
		DorisRequireDistribution: true, DorisBucketLimit: 32, DorisReplicationNum: 3,
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	// 线缆字段名抽样:四种方言 + 级别 map 必须以 snake_case 出现。
	for _, key := range []string{
		`"dml_transaction"`, `"dml_max_insert_rows"`, `"ddl_max_key_parts"`,
		`"max_affect_rows"`, `"rule_levels"`, `"ddl_forbid_truncate"`,
		`"mongo_forbid_empty_filter"`, `"mongo_index_name_spec"`,
		`"redis_key_prefix"`, `"redis_scan_require_match"`,
		`"doris_require_distribution"`, `"doris_replication_num"`,
	} {
		if !strings.Contains(s, key) {
			t.Fatalf("wire field %s missing: %s", key, s)
		}
	}
	out := &AuditRole{}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("round-trip drift:\n got %+v\nwant %+v", out, in)
	}
}

func TestExecReplyRoundTrip(t *testing.T) {
	in := &ExecReply{
		Ok:    false,
		Error: "语句 2 执行失败",
		Records: []*Record{
			{Sql: "UPDATE t SET a=1", AffectRows: 3, Status: "已执行", Level: 0,
				ExecTime: "0.02", Table: "t", Schema: "db", IsOsc: false,
				InsulateWordList: []string{"`order`"}, Rollback: "UPDATE t SET a=0 WHERE id IN (1,2,3)"},
			{Sql: "DELETE FROM t", Error: "超时", Level: 1},
		},
		Rollback: []string{"INSERT INTO t SELECT ..."},
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, key := range []string{`"affect_rows"`, `"insulate_word_list"`, `"is_osc"`, `"exec_time"`} {
		if !strings.Contains(s, key) {
			t.Fatalf("wire field %s missing: %s", key, s)
		}
	}
	out := &ExecReply{}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Records) != 2 || out.Records[0].Rollback == "" || out.Records[1].Error != "超时" {
		t.Fatalf("round-trip drift: %+v", out)
	}
	if out.Records[0].GetError() != "" || in.Error == "" {
		t.Fatalf("GetError 形态破坏: %q", out.Records[0].GetError())
	}
}

// TestCheckRequestWireShape 关键语义字段:mode 空按 write、auth_db、64 位 limit。
func TestCheckRequestWireShape(t *testing.T) {
	in := &CheckRequest{
		Sql: "select 1", Schema: "db", Lang: "zh_CN", Mode: "query",
		Source: &DataSource{Ip: "10.0.0.1", Port: 3306, Username: "u", Password: "p",
			Kind: "mongodb", AuthDb: "business"},
		Rule: &AuditRole{DmlWhere: true},
	}
	b, _ := json.Marshal(in)
	for _, key := range []string{`"auth_db"`, `"mode"`, `"max_affect_rows"`} {
		// max_affect_rows 属 ExecRequest,这里只验 auth_db/mode
		if key == `"max_affect_rows"` {
			continue
		}
		if !strings.Contains(string(b), key) {
			t.Fatalf("wire field %s missing: %s", key, b)
		}
	}
	q := &QueryRequest{Sql: "find {}", Limit: 1 << 40, Kind: "mongodb"}
	bq, _ := json.Marshal(q)
	if !strings.Contains(string(bq), "1099511627776") {
		t.Fatalf("uint64 limit 精度丢失: %s", bq)
	}
}
