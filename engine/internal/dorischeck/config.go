package dorischeck

import (
	"reflect"
	"strings"

	enginev1 "engine/gen/engine/v1"
)

// Config 规则开关的取值快照，由 server 从 enginev1.AuditRole 构造。
// 字段显式类型；proto → Config 的映射用 ConfigFromProto 反射完成（对齐 mongocheck）。
type Config struct {
	// 建表必须有 DISTRIBUTED BY（否则 Doris 默认随机分桶，不可控）
	RequireDistribution bool
	// BUCKETS 数量上限（0 = 不限制）
	BucketLimit int
	// 期望的副本数（0 = 不校验）
	ReplicationNum int
	// 是否提示 KEY 数据模型（聚合模型写入有约束）
	KeyModel bool

	// Levels 规则级别：key 为规则名，value 为 error / warn / observe，缺省取规则默认级别。
	Levels map[string]string
}

// ConfigFromProto 从规则集构造 Config：AuditRole 里 doris_* 的字段按名字自动填进 Config。
func ConfigFromProto(r *enginev1.AuditRole) Config {
	var cfg Config
	if r == nil {
		return cfg
	}
	cfg.Levels = r.GetRuleLevels()

	ref := r.ProtoReflect()
	fds := ref.Descriptor().Fields()
	dst := reflect.ValueOf(&cfg).Elem()
	for i := 0; i < fds.Len(); i++ {
		fd := fds.Get(i)
		if !strings.HasPrefix(strings.ToLower(string(fd.Name())), "doris") {
			continue
		}
		field := dst.FieldByNameFunc(func(name string) bool {
			return normSwitch(name) == protoFieldKey(string(fd.Name()))
		})
		if !field.IsValid() || !field.CanSet() {
			continue
		}
		val := ref.Get(fd)
		switch field.Kind() {
		case reflect.Bool:
			field.SetBool(val.Bool())
		case reflect.Int:
			field.SetInt(val.Int())
		case reflect.String:
			field.SetString(val.String())
		}
	}
	return cfg
}

// Has 配置里是否存在该开关字段（忽略大小写与下划线），供 archguard 核对。
func (Config) Has(name string) bool {
	return reflect.ValueOf(Config{}).FieldByNameFunc(func(n string) bool {
		return normSwitch(n) == protoFieldKey(name)
	}).IsValid()
}

// protoFieldKey 把 proto 字段名归一化成 Config 字段名：
// doris_require_distribution → requiredistribution。
func protoFieldKey(protoName string) string {
	return strings.TrimPrefix(normSwitch(protoName), "doris")
}

// Level 取规则的生效级别：RuleLevels 里配了且合法就用配置值，否则用规则默认级别。
func (c Config) Level(r Rule) Level {
	if l, ok := c.Levels[r.Name()]; ok {
		if lv := Level(strings.ToLower(strings.TrimSpace(l))); ValidLevel(lv) {
			return lv
		}
	}
	return r.DefaultLevel()
}

// normSwitch 归一化开关名后比较
func normSwitch(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), "_", ""))
}
