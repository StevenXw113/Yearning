package dorischeck

import (
	"strings"
	"testing"
)

func fullCfg() Config {
	return Config{
		RequireDistribution: true,
		BucketLimit:         3,
		ReplicationNum:      3,
		KeyModel:            true,
	}
}

func hasRule(findings []Finding, rule string) bool {
	for _, f := range findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}

func TestRequireDistribution(t *testing.T) {
	noDist := `CREATE TABLE t (id INT) DUPLICATE KEY(id) PROPERTIES("replication_num"="1")`
	if f := Check(noDist, fullCfg()); !hasRule(f, RuleRequireDistribution) {
		t.Errorf("无 DISTRIBUTED BY 应命中, got %+v", f)
	}
	withDist := `CREATE TABLE t (id INT) DUPLICATE KEY(id) DISTRIBUTED BY HASH(id) BUCKETS 2 PROPERTIES("replication_num"="1")`
	if f := Check(withDist, fullCfg()); hasRule(f, RuleRequireDistribution) {
		t.Errorf("有 DISTRIBUTED BY 不应命中, got %+v", f)
	}
}

func TestBucketLimit(t *testing.T) {
	big := `CREATE TABLE t (id INT) DISTRIBUTED BY HASH(id) BUCKETS 10`
	if f := Check(big, fullCfg()); !hasRule(f, RuleBucketLimit) {
		t.Errorf("BUCKETS 10 超上限 3 应命中, got %+v", f)
	}
	small := `CREATE TABLE t (id INT) DISTRIBUTED BY HASH(id) BUCKETS 2`
	if f := Check(small, fullCfg()); hasRule(f, RuleBucketLimit) {
		t.Errorf("BUCKETS 2 未超上限不应命中, got %+v", f)
	}
	noBuckets := `CREATE TABLE t (id INT) DISTRIBUTED BY HASH(id)`
	if f := Check(noBuckets, fullCfg()); hasRule(f, RuleBucketLimit) {
		t.Errorf("未写 BUCKETS 不应命中, got %+v", f)
	}
}

func TestReplicationNum(t *testing.T) {
	mismatch := `CREATE TABLE t (id INT) DISTRIBUTED BY HASH(id) BUCKETS 2 PROPERTIES("replication_num"="1")`
	if f := Check(mismatch, fullCfg()); !hasRule(f, RuleReplicationNum) {
		t.Errorf("副本数 1 vs 期望 3 应命中, got %+v", f)
	}
	match := `CREATE TABLE t (id INT) DISTRIBUTED BY HASH(id) BUCKETS 2 PROPERTIES("replication_num"="3")`
	if f := Check(match, fullCfg()); hasRule(f, RuleReplicationNum) {
		t.Errorf("副本数匹配不应命中, got %+v", f)
	}
}

func TestKeyModel(t *testing.T) {
	agg := `CREATE TABLE t (id INT) AGGREGATE KEY(id) DISTRIBUTED BY HASH(id) BUCKETS 2`
	if f := Check(agg, fullCfg()); !hasRule(f, RuleKeyModel) {
		t.Errorf("AGGREGATE KEY 应提示, got %+v", f)
	}
	dup := `CREATE TABLE t (id INT) DUPLICATE KEY(id) DISTRIBUTED BY HASH(id) BUCKETS 2`
	if f := Check(dup, fullCfg()); hasRule(f, RuleKeyModel) {
		t.Errorf("DUPLICATE KEY 不应提示, got %+v", f)
	}
}

func TestParseNotCreateTable(t *testing.T) {
	if Parse("SELECT * FROM t") != nil {
		t.Error("非建表语句应返回 nil")
	}
	if Parse(`INSERT INTO t VALUES (1)`) != nil {
		t.Error("非建表语句应返回 nil")
	}
	if strings.Contains(Parse(`CREATE TABLE t (id INT)`).TableName, "t") == false {
		t.Error("表名解析错误")
	}
}
