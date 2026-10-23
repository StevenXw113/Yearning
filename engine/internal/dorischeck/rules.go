package dorischeck

import "strconv"

// ---- 建表必须有 DISTRIBUTED BY ----

const RuleRequireDistribution = "DorisRequireDistribution"

type requireDistribution struct{}

func init() { Register(requireDistribution{}) }

func (requireDistribution) Name() string { return RuleRequireDistribution }
func (requireDistribution) Desc() string {
	return "建表必须指定 DISTRIBUTED BY（否则默认随机分桶，不可控）"
}
func (requireDistribution) Category() Category { return CategorySecurity }
func (requireDistribution) DefaultLevel() Level {
	return LevelError
}
func (requireDistribution) Check(cfg Config, stmt *CreateTable) []Finding {
	if !cfg.RequireDistribution || stmt.HasDistribution {
		return nil
	}
	return []Finding{{
		Rule:    RuleRequireDistribution,
		Level:   cfg.Level(requireDistribution{}),
		Message: "建表 " + stmt.TableName + " 没有指定 DISTRIBUTED BY：Doris 会默认随机分桶，数据分布不可控，请显式写 DISTRIBUTED BY HASH(...) 或 RANDOM",
	}}
}

// ---- BUCKETS 数量上限 ----

const RuleBucketLimit = "DorisBucketLimit"

type bucketLimit struct{}

func init() { Register(bucketLimit{}) }

func (bucketLimit) Name() string       { return RuleBucketLimit }
func (bucketLimit) Desc() string       { return "建表 BUCKETS 数量超过上限（0 = 不限制）" }
func (bucketLimit) Category() Category { return CategoryStructure }
func (bucketLimit) DefaultLevel() Level {
	return LevelWarn
}
func (bucketLimit) Check(cfg Config, stmt *CreateTable) []Finding {
	if cfg.BucketLimit <= 0 || stmt.Buckets == 0 || stmt.Buckets <= cfg.BucketLimit {
		return nil
	}
	return []Finding{{
		Rule:    RuleBucketLimit,
		Level:   cfg.Level(bucketLimit{}),
		Message: "建表 " + stmt.TableName + " 的 BUCKETS 数量 " + strconv.Itoa(stmt.Buckets) + " 超过上限 " + strconv.Itoa(cfg.BucketLimit) + "：分桶过多会增加元数据与调度开销",
	}}
}

// ---- 副本数 ----

const RuleReplicationNum = "DorisReplicationNum"

type replicationNum struct{}

func init() { Register(replicationNum{}) }

func (replicationNum) Name() string       { return RuleReplicationNum }
func (replicationNum) Desc() string       { return "建表副本数与期望值不一致（0 = 不校验）" }
func (replicationNum) Category() Category { return CategoryStructure }
func (replicationNum) DefaultLevel() Level {
	return LevelWarn
}
func (replicationNum) Check(cfg Config, stmt *CreateTable) []Finding {
	if cfg.ReplicationNum <= 0 || stmt.ReplicationNum == 0 || stmt.ReplicationNum == cfg.ReplicationNum {
		return nil
	}
	return []Finding{{
		Rule:    RuleReplicationNum,
		Level:   cfg.Level(replicationNum{}),
		Message: "建表 " + stmt.TableName + " 的副本数 " + strconv.Itoa(stmt.ReplicationNum) + " 与期望值 " + strconv.Itoa(cfg.ReplicationNum) + " 不一致",
	}}
}

// ---- KEY 数据模型提示 ----

const RuleKeyModel = "DorisKeyModel"

type keyModel struct{}

func init() { Register(keyModel{}) }

func (keyModel) Name() string { return RuleKeyModel }
func (keyModel) Desc() string {
	return "提示 AGGREGATE KEY 聚合模型（写入有约束，聚合类型选错会导致结果错误）"
}
func (keyModel) Category() Category { return CategoryStructure }
func (keyModel) DefaultLevel() Level {
	return LevelObserve
}
func (keyModel) Check(cfg Config, stmt *CreateTable) []Finding {
	if !cfg.KeyModel || stmt.KeyModel != "aggregate" {
		return nil
	}
	return []Finding{{
		Rule:    RuleKeyModel,
		Level:   cfg.Level(keyModel{}),
		Message: "建表 " + stmt.TableName + " 使用 AGGREGATE KEY 聚合模型：非 key 列的聚合类型（SUM/REPLACE 等）选错会导致查询结果错误，写入也有约束，请确认字段聚合类型",
	}}
}
