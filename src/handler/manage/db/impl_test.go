package db

import (
	"testing"

	"Yearning-go/src/model"
)

func TestValidateRuleSetBinding(t *testing.T) {
	mongoSource := &model.CoreDataSource{DBType: model.DBTypeMongoDB, RuleId: 1}
	mysqlSource := &model.CoreDataSource{DBType: 0, RuleId: 1}

	cases := []struct {
		name    string
		source  *model.CoreDataSource
		rule    *model.CoreRules
		wantErr bool
	}{
		{"Mongo 拒绝非法 v2", mongoSource, &model.CoreRules{Dialect: "mongodb", Definition: []byte(`{}`)}, true},
		{
			"Mongo 绑定有效 v2",
			mongoSource,
			&model.CoreRules{Dialect: "mongodb", Definition: []byte(`{
				"schema_version": 2,
				"dialect": "mongodb",
				"dialects": ["mongodb"],
				"scope": "data_source",
				"settings": {},
				"rules": [
					{"id":"mongo.collection.drop","enabled":true,"level":"error"},
					{"id":"mongo.collection.rename","enabled":true,"level":"error"},
					{"id":"mongo.account.privilege","enabled":true,"level":"error"}
				]
			}`)},
			false,
		},
		{"Mongo 拒绝 MySQL", mongoSource, &model.CoreRules{Dialect: "mysql"}, true},
		{"Mongo v2 缺 definition", mongoSource, &model.CoreRules{Dialect: "mongodb"}, true},
		{"MySQL 兼容旧规则", mysqlSource, &model.CoreRules{}, false},
		{"MySQL 拒绝 Mongo", mysqlSource, &model.CoreRules{Dialect: "mongodb", Definition: []byte(`{}`)}, true},
		{"MySQL 拒绝 Redis 方言", mysqlSource, &model.CoreRules{Dialect: "redis"}, true},
		{"非 Mongo 不支持 v2 definition", mysqlSource, &model.CoreRules{Dialect: "mysql", Definition: []byte(`{}`)}, true},
		{"mysql rejects explicit dialect with mongo content", mysqlSource, &model.CoreRules{Dialect: "mysql", AuditRole: model.JSON(`{"MongoForbidMultiWrite":true}`)}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateRuleSetBinding(c.source, c.rule)
			if (err != nil) != c.wantErr {
				t.Fatalf("validateRuleSetBinding() error = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}

func TestValidateRuleSetBindingLegacyDialects(t *testing.T) {
	mongoSource := &model.CoreDataSource{DBType: model.DBTypeMongoDB, RuleId: 1}
	mysqlSource := &model.CoreDataSource{DBType: 0, RuleId: 1}
	postgresSource := &model.CoreDataSource{DBType: model.DBTypePostgreSQL, RuleId: 1}
	redisSource := &model.CoreDataSource{DBType: model.DBTypeRedis, RuleId: 1}
	dorisSource := &model.CoreDataSource{DBType: model.DBTypeDoris, RuleId: 1}

	cases := []struct {
		name    string
		source  *model.CoreDataSource
		rule    *model.CoreRules
		wantErr bool
	}{
		{"mysql legacy sql", mysqlSource, &model.CoreRules{AuditRole: model.JSON(`{"DDLCheckTableComment":true}`)}, false},
		{"postgres legacy sql", postgresSource, &model.CoreRules{AuditRole: model.JSON(`{"DMLWhere":true}`)}, false},
		{"mongo rejects legacy sql", mongoSource, &model.CoreRules{AuditRole: model.JSON(`{"DDLCheckTableComment":true}`)}, true},
		{"mongo rejects legacy redis", mongoSource, &model.CoreRules{AuditRole: model.JSON(`{"RedisDeleteKey":true}`)}, true},
		{"mysql rejects legacy mongo", mysqlSource, &model.CoreRules{AuditRole: model.JSON(`{"MongoForbidMultiWrite":true}`)}, true},
		{"redis rejects legacy mongo", redisSource, &model.CoreRules{AuditRole: model.JSON(`{"MongoForbidMultiWrite":true}`)}, true},
		{"redis legacy redis", redisSource, &model.CoreRules{AuditRole: model.JSON(`{"RedisDeleteKey":true}`)}, false},
		{"doris legacy doris", dorisSource, &model.CoreRules{AuditRole: model.JSON(`{"DorisRequireDistribution":true,"DDLCheckTableComment":true}`)}, false},
		{"mysql rejects legacy doris", mysqlSource, &model.CoreRules{AuditRole: model.JSON(`{"DorisRequireDistribution":true}`)}, true},
		{"redis rejects legacy sql", redisSource, &model.CoreRules{AuditRole: model.JSON(`{"DDLCheckTableComment":true}`)}, true},
		{"mixed legacy dialect", mysqlSource, &model.CoreRules{AuditRole: model.JSON(`{"MongoForbidMultiWrite":true,"RedisDeleteKey":true}`)}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateRuleSetBinding(c.source, c.rule)
			if (err != nil) != c.wantErr {
				t.Fatalf("validateRuleSetBinding() error = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}
