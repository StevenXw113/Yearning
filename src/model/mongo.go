package model

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"Yearning-go/src/lib/enc"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// DBTypeMongoDB 数据源类型：0 mysql（默认）、1 pg（未实现）、2 mongodb
const DBTypeMongoDB = 2

// MongoTimeout 建立连接与探活的超时
const MongoTimeout = 15 * time.Second

// MongoSystemDB 系统库，不对用户暴露
var MongoSystemDB = map[string]struct{}{"admin": {}, "config": {}, "local": {}}

// MongoURI 拼 MongoDB 连接串。用户名与口令走 url.UserPassword 做转义，
// 否则口令里的 @ : / % 会把连接串拆坏；认证库固定 admin（Mongo 账号通常建在 admin 库）。
func MongoURI(s *CoreDataSource, password string) string {
	u := &url.URL{
		Scheme:   "mongodb",
		User:     url.UserPassword(s.Username, password),
		Host:     net.JoinHostPort(s.IP, strconv.Itoa(s.Port)),
		Path:     "/",
		RawQuery: "authSource=admin",
	}
	return u.String()
}

// ConnectMongo 按数据源配置建立 MongoDB 客户端（口令列先解密再拼连接串）。
// 与 MySQL 不同，Mongo 连接不经过 gorm，调用方负责 Disconnect。
func (s *CoreDataSource) ConnectMongo() (*mongo.Client, error) {
	return s.ConnectMongoWith(enc.Decrypt(C.General.SecretKey, s.Password))
}

// ConnectMongoWith 用明文口令建连。数据源管理页的「检测」已经把口令解好
// （明文与密文都从界面来，见 ManageDBCreateOrEdit），这里再解一次会变成空口令。
func (s *CoreDataSource) ConnectMongoWith(password string) (*mongo.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), MongoTimeout)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().
		ApplyURI(MongoURI(s, password)).
		SetConnectTimeout(MongoTimeout).
		SetServerSelectionTimeout(MongoTimeout))
	if err != nil {
		return nil, err
	}
	// 连接是懒建立的，Ping 一次才能把"地址/口令不对"在入口处暴露出来
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	return client, nil
}

// MongoDatabases 列出业务库：过滤系统库与数据源上配置的 ExcludeDbList。
// （库名不做标识符校验——Mongo 的库名只是选择器，不像 SQL 那样要拼进语句。）
func (s *CoreDataSource) MongoDatabases() ([]string, error) {
	client, err := s.ConnectMongo()
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), MongoTimeout)
	defer cancel()
	names, err := client.ListDatabaseNames(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	exclude := map[string]struct{}{}
	for _, n := range strings.Split(s.ExcludeDbList, ",") {
		if n = strings.TrimSpace(n); n != "" {
			exclude[n] = struct{}{}
		}
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		if _, ok := MongoSystemDB[n]; ok {
			continue
		}
		if _, ok := exclude[n]; ok {
			continue
		}
		out = append(out, n)
	}
	return out, nil
}

// MongoCollections 列出指定库下的集合
func (s *CoreDataSource) MongoCollections(database string) ([]string, error) {
	if database == "" {
		return nil, nil
	}
	client, err := s.ConnectMongo()
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), MongoTimeout)
	defer cancel()
	return client.Database(database).ListCollectionNames(ctx, bson.M{})
}

// MongoSampleSize 每个集合取样推断字段的文档数。
// 文档模型没有固定 schema，取 1 条容易漏字段，多取几条把字段名汇总起来。
const MongoSampleSize = 50

// MongoMaxSampleCollections 未指定集合时，最多取样多少个集合
const MongoMaxSampleCollections = 10

// MongoCollectionSchema 对集合取样，汇总字段名，输出形如
// "users(_id, name, email, tags)" 的文本，供 AI 建议感知数据结构。
// 只取字段名、不取字段值：业务数据不出库，也不进 prompt。
func (s *CoreDataSource) MongoCollectionSchema(database string, collections []string) ([]string, error) {
	if database == "" || len(collections) == 0 {
		return nil, nil
	}
	client, err := s.ConnectMongo()
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), MongoTimeout*2)
	defer cancel()

	out := make([]string, 0, len(collections))
	for _, name := range collections {
		if name == "" {
			continue
		}
		coll := client.Database(database).Collection(name)
		// $sample 比 find().limit() 更能覆盖到不同形态的文档；
		// 下面只读字段名、不读字段值，业务数据不会进 prompt
		cur, err := coll.Aggregate(ctx, bson.A{
			bson.D{{Key: "$sample", Value: bson.D{{Key: "size", Value: MongoSampleSize}}}},
		})
		if err != nil {
			out = append(out, fmt.Sprintf("%s(取样失败: %v)", name, err))
			continue
		}
		var order []string
		seen := map[string]struct{}{}
		for cur.Next(ctx) {
			var doc bson.M
			if err := cur.Decode(&doc); err != nil {
				continue
			}
			for k := range doc {
				if _, ok := seen[k]; !ok {
					seen[k] = struct{}{}
					order = append(order, k)
				}
			}
		}
		_ = cur.Close(ctx)
		// 文档模型下字段出现顺序随机，排一下让同一集合每次输出一致
		sort.Strings(order)
		out = append(out, fmt.Sprintf("%s(%s)", name, strings.Join(order, ", ")))
	}
	return out, nil
}
