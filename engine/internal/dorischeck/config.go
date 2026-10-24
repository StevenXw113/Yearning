package dorischeck

import (
	"reflect"
	"strings"

	"engine/api"
)

// Config 规则开关的取值快照，由 server 从 api.AuditRole 构造。
// 字段显式类型；AuditRole → Config 的映射用 ConfigFromAPI 反射完成（对齐 mongocheck）。
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

// ConfigFromAPI 从规则集构造 Config：AuditRole 里 doris_* 的字段按名字自动填进 Config。
func ConfigFromAPI(r *api.AuditRole) Config {
	var cfg Config
	if r == nil {
		return cfg
	}
	cfg.Levels = r.RuleLevels

	src := reflect.ValueOf(r).Elem()
	dst := reflect.ValueOf(&cfg).Elem()
	st := src.Type()
	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		if !strings.HasPrefix(strings.ToLower(f.Name), "doris") {
			continue
		}
		field := dst.FieldByNameFunc(func(name string) bool {
			return normSwitch(name) == protoFieldKey(f.Name)
		})
		if !field.IsValid() || !field.CanSet() {
			continue
		}
		switch field.Kind() {
		case reflect.Bool:
			field.SetBool(src.Field(i).Bool())
		case reflect.Int, reflect.Int32, reflect.Int64:
			field.SetInt(src.Field(i).Int())
		case reflect.String:
			field.SetString(src.Field(i).String())
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

// protoFieldKey 把 AuditRole 字段名归一化成 Config 字段名：
// DorisRequireDistribution → requiredistribution。
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
