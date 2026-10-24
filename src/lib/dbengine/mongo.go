package dbengine

import (
	"context"
	"errors"

	"Yearning-go/src/lib/mongodb"
	"Yearning-go/src/model"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func init() { register(model.DBTypeMongoDB, mongoEngine{}) }

type mongoEngine struct{}

func (mongoEngine) Kind() string { return "mongodb" }

func (mongoEngine) ListDatabases(_ context.Context, src *model.CoreDataSource) ([]string, error) {
	return src.MongoDatabases()
}

func (mongoEngine) ListCollections(_ context.Context, src *model.CoreDataSource, db string) ([]string, error) {
	return src.MongoCollections(db)
}

func (mongoEngine) DescribeTable(_ context.Context, src *model.CoreDataSource, db, name string) ([]Field, error) {
	// Mongo 文档没有固定 schema：用字段名汇总当「字段」，类型未知。
	names, err := src.MongoCollectionSchema(db, []string{name})
	if err != nil {
		return nil, err
	}
	out := make([]Field, 0, len(names))
	for _, n := range names {
		out = append(out, Field{Name: n})
	}
	return out, nil
}

func (mongoEngine) Highlight(src *model.CoreDataSource, isField bool, db string) []map[string]string {
	if !isField {
		dbs, err := src.MongoDatabases()
		if err != nil {
			return nil
		}
		out := make([]map[string]string, 0, len(dbs))
		for _, name := range dbs {
			out = append(out, map[string]string{"vl": name, "meta": "Schema"})
		}
		return out
	}
	if db == "" {
		return nil
	}
	colls, err := src.MongoCollections(db)
	if err != nil {
		return nil
	}
	out := make([]map[string]string, 0, len(colls))
	for _, name := range colls {
		out = append(out, map[string]string{"vl": name, "meta": "Table"})
	}
	limit := colls
	if len(limit) > 10 {
		limit = limit[:10]
	}
	fields, err := src.MongoCollectionSchema(db, limit)
	if err != nil {
		return out
	}
	for _, f := range fields {
		out = append(out, map[string]string{"vl": f, "meta": "Fields"})
	}
	return out
}

func (mongoEngine) ValidateChange(_ *model.CoreDataSource, sql string) error {
	var cmd bson.D
	if err := bson.UnmarshalExtJSON([]byte(sql), false, &cmd); err != nil {
		return errors.New("请填写 MongoDB 命令的 JSON，例如 {\"update\":\"users\",\"updates\":[...]}：" + err.Error())
	}
	_, err := mongodb.Validate(cmd)
	return err
}
