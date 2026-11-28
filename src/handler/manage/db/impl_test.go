package db

import (
	"encoding/json"
	"reflect"
	"strings"
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

func TestValidateMongoConnection(t *testing.T) {
	cases := []struct {
		name      string
		source    *model.CoreDataSource
		wantError bool
	}{
		{"other dialect unchanged", &model.CoreDataSource{DBType: 0}, false},
		{"invalid TLS CA", &model.CoreDataSource{DBType: model.DBTypeMongoDB, IP: "localhost", MongoTLS: true, CAFile: "invalid-ca"}, true},
		{"invalid auth mechanism", &model.CoreDataSource{DBType: model.DBTypeMongoDB, IP: "localhost", MongoAuthMechanism: "unsupported"}, true},
		{"anonymous Mongo", &model.CoreDataSource{DBType: model.DBTypeMongoDB, IP: "localhost"}, false},
		{"replica and special password", &model.CoreDataSource{DBType: model.DBTypeMongoDB, IP: "a:27017,b:27018", Username: "user", Password: "p@ss^word%", AuthDatabase: "admin", MongoReplicaSet: "rs-test"}, false},
		{"missing username", &model.CoreDataSource{DBType: model.DBTypeMongoDB, IP: "localhost", Password: "pw"}, true},
		{"malformed seeds", &model.CoreDataSource{DBType: model.DBTypeMongoDB, IP: "a,,b"}, true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateMongoConnection(tt.source); (err != nil) != tt.wantError {
				t.Fatalf("validation error = %v, want error=%v", err, tt.wantError)
			}
		})
	}
}

func TestMergeSourceEditFromList(t *testing.T) {
	stored := model.CoreDataSource{
		SourceId: "test", DBType: model.DBTypeMongoDB, Source: "original",
		IP: "localhost", Port: 27017, Username: "user", Password: "stored-secret",
		AuthDatabase: "accounts", MongoReplicaSet: "rs-test", MongoTLS: true,
		MongoAuthMechanism: "SCRAM-SHA-256", EnvironmentTag: "production",
		CAFile: "stored-ca", Cert: "stored-cert", KeyFile: "stored-key",
	}
	// Reproduce the list projection, including zero-valued secrets serialized by CoreDataSource.
	listed := model.CoreDataSource{}
	fields := strings.Split(dataSourceListFields, ",")
	storedValue, listValue := reflect.ValueOf(stored), reflect.ValueOf(&listed).Elem()
	for i := 0; i < storedValue.NumField(); i++ {
		column := strings.Split(storedValue.Type().Field(i).Tag.Get("json"), ",")[0]
		for _, selected := range fields {
			if selected == column {
				listValue.Field(i).Set(storedValue.Field(i))
			}
		}
	}
	if listed.Password != "" || listed.KeyFile != "" || listed.Cert != "" || listed.CAFile != "" {
		t.Fatal("list projection must not expose connection secrets")
	}
	listed.Source = "renamed"
	raw, err := json.Marshal(listed)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := mergeSourceEdit(raw, stored, false)
	if err != nil {
		t.Fatal(err)
	}
	stored.Source = "renamed"
	if !reflect.DeepEqual(merged, stored) {
		t.Fatal("editing a list record changed connection settings or secrets")
	}
}

func TestMergeSourceEditPresenceAndSecrets(t *testing.T) {
	stored := model.CoreDataSource{
		DBType: model.DBTypeMongoDB, IP: "localhost", Username: "user",
		Password: "saved", AuthDatabase: "accounts", MongoReplicaSet: "rs",
		MongoTLS: true, MongoAuthMechanism: "SCRAM-SHA-1", CAFile: "ca", Cert: "cert", KeyFile: "key",
	}
	cases := []struct {
		name         string
		raw          string
		clear        bool
		wantPassword string
		wantDefaults bool
		wantErr      bool
	}{
		{"omitted fields", "{}", false, "saved", false, false},
		{"blank password keeps saved", `{"password":""}`, false, "saved", false, false},
		{"replace password", `{"password":"replacement"}`, false, "replacement", false, false},
		{"explicit clear", `{"password":"","username":""}`, true, "", false, false},
		{"clear omitted password", "{}", true, "", false, false},
		{"clear and replace rejected", `{"password":"replacement"}`, true, "", false, true},
		{"reset connection options", `{"auth_database":"","mongo_replica_set":"","mongo_tls":false,"mongo_auth_mechanism":""}`, false, "saved", true, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mergeSourceEdit(json.RawMessage(tt.raw), stored, tt.clear)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.Password != tt.wantPassword || got.CAFile != stored.CAFile || got.Cert != stored.Cert || got.KeyFile != stored.KeyFile {
				t.Fatal("incorrect secret merge")
			}
			if tt.wantDefaults {
				if got.AuthDatabase != "" || got.MongoReplicaSet != "" || got.MongoTLS || got.MongoAuthMechanism != "" {
					t.Fatal("explicit defaults were ignored")
				}
			} else if got.AuthDatabase != stored.AuthDatabase || got.MongoReplicaSet != stored.MongoReplicaSet || got.MongoTLS != stored.MongoTLS || got.MongoAuthMechanism != stored.MongoAuthMechanism {
				t.Fatal("omitted options were reset")
			}
		})
	}
	stored.DBType = 0 // MySQL
	if _, err := mergeSourceEdit(json.RawMessage("{}"), stored, true); err == nil {
		t.Fatal("password clear must be MongoDB only")
	}
}
