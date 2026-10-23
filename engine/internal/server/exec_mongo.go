package server

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	enginev1 "engine/gen/engine/v1"
	"engine/internal/mongodb"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// mongoTimeout 建立连接与探活的超时。
const mongoTimeout = 15 * time.Second

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
// 口令是明文：主程序在发起 RPC 前已经把口令列解密好了，引擎侧不再持有主程序密钥。
func connectMongo(ds *enginev1.DataSource) (*mongo.Client, error) {
	if ds == nil || ds.Ip == "" {
		return nil, fmt.Errorf("数据源 IP 为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), mongoTimeout)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().
		ApplyURI(mongoURI(ds.Username, ds.Password, ds.Ip, ds.Port)).
		SetConnectTimeout(mongoTimeout).
		SetServerSelectionTimeout(mongoTimeout))
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

// execMongo 执行 MongoDB 变更工单：硬保底 → 连接 → 前镜像 → 执行 → 回滚。
//
// 与 SQL 引擎不同，Mongo 的执行由引擎独立完成（前镜像抓取与回滚都在 internal/mongodb 里），
// 结果映射回 ExecReply，与 SQL 同一形状——主程序侧的落库逻辑（Records/Rollback）
// 因此不需要知道数据源类型。
func (e *Engine) execMongo(ctx context.Context, req *enginev1.ExecRequest) (*enginev1.ExecReply, error) {
	var cmd bson.D
	if err := bson.UnmarshalExtJSON([]byte(req.Order.Sql), false, &cmd); err != nil {
		return &enginev1.ExecReply{Ok: false, Error: "工单内容不是合法的 MongoDB 命令 JSON：" + err.Error()}, nil
	}
	// 执行前再过一遍硬保底（与提交时同一套）：提交到审批之间工单内容可能被改过
	if _, err := mongodb.Validate(cmd); err != nil {
		return &enginev1.ExecReply{Ok: false, Error: err.Error()}, nil
	}
	maxAffectRows := 0
	if req.Rules != nil {
		maxAffectRows = int(req.Rules.MongoMaxAffectRows)
	}

	client, err := connectMongo(req.Source)
	if err != nil {
		return &enginev1.ExecReply{Ok: false, Error: "连接 MongoDB 失败: " + err.Error()}, nil
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	// 注意（回归点）：createIndexes 的实时构建进度此前由主程序写 osc_info，
	// 引擎没有主程序的元数据库，这里不再挂 WatchIndexBuild 进度回传；
	// 索引构建完成与否仍由执行结果体现。见 design.md 语义核对第 4 条。
	out, err := mongodb.Execute(ctx, client, req.Order.DataBase, cmd, maxAffectRows)
	if err != nil {
		return &enginev1.ExecReply{Ok: false, Error: err.Error(),
			Records: []*enginev1.Record{{Sql: req.Order.Sql, Status: "执行失败", Level: 1, Error: err.Error()}}}, nil
	}

	rec := &enginev1.Record{Sql: req.Order.Sql, Status: "已执行", AffectRows: uint32(out.Affected)}
	if out.Warning != "" {
		rec.Error = out.Warning
	}
	return &enginev1.ExecReply{Ok: true, Records: []*enginev1.Record{rec}, Rollback: out.Rollback}, nil
}
