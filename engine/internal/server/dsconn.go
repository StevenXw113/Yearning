package server

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	enginev1 "engine/gen/engine/v1"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// 命令式引擎（MongoDB / Redis）的连接辅助。
//
// 与 SQL 的 dsnOf 对齐：入参是 enginev1.DataSource，口令是**明文**——
// 主程序在发起 RPC 前已经把口令列解密好了（见 src/handler 里 Password: p 的写法），
// 引擎侧不再持有主程序的密钥、也不需要再解一次。

const (
	dsMongoTimeout = 15 * time.Second
	dsRedisTimeout = 15 * time.Second
)

// mongoURI 拼 MongoDB 连接串。用户名与口令走 url.UserPassword 转义，
// 否则口令里的 @ : / % 会把连接串拆坏；认证库固定 admin。
func mongoURI(username, password, ip string, port int32) string {
	u := &url.URL{
		Scheme:   "mongodb",
		User:     url.UserPassword(username, password),
		Host:     net.JoinHostPort(ip, strconv.Itoa(int(port))),
		Path:     "/",
		RawQuery: "authSource=admin",
	}
	return u.String()
}

// connectMongo 按 DataSource 建立 MongoDB 客户端并 Ping 探活。调用方负责 Disconnect。
func connectMongo(ds *enginev1.DataSource) (*mongo.Client, error) {
	if ds == nil || ds.Ip == "" {
		return nil, fmt.Errorf("数据源 IP 为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), dsMongoTimeout)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().
		ApplyURI(mongoURI(ds.Username, ds.Password, ds.Ip, ds.Port)).
		SetConnectTimeout(dsMongoTimeout).
		SetServerSelectionTimeout(dsMongoTimeout))
	if err != nil {
		return nil, err
	}
	// 连接是懒建立的，Ping 一次才能把「地址/口令不对」在入口处暴露出来
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	return client, nil
}

// redisOptions 按 DataSource 构造客户端选项。口令为空表示未开认证（Redis 允许无密码）。
func redisOptions(ds *enginev1.DataSource) *redis.Options {
	opt := &redis.Options{
		Addr:         net.JoinHostPort(ds.Ip, strconv.Itoa(int(ds.Port))),
		Password:     ds.Password,
		DB:           0,
		DialTimeout:  dsRedisTimeout,
		ReadTimeout:  dsRedisTimeout,
		WriteTimeout: dsRedisTimeout,
	}
	if ds.Username != "" {
		opt.Username = ds.Username
	}
	return opt
}

// connectRedisDB 建连并选中指定逻辑库（go-redis 在建连时按 Options.DB 发 SELECT）。
func connectRedisDB(ds *enginev1.DataSource, db int) (*redis.Client, error) {
	if ds == nil || ds.Ip == "" {
		return nil, fmt.Errorf("数据源 IP 为空")
	}
	opt := redisOptions(ds)
	if db > 0 {
		opt.DB = db
	}
	client := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), dsRedisTimeout)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

// redisParseDB 把查询页传来的库名（dbN）解析成库号，非法值返回 0。
func redisParseDB(name string) int {
	if len(name) < 3 || name[:2] != "db" {
		return 0
	}
	n, err := strconv.Atoi(name[2:])
	if err != nil || n < 0 {
		return 0
	}
	return n
}
