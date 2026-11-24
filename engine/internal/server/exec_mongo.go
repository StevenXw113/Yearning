package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"engine/api"
	"engine/internal/mongodb"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// mongoTimeout 建立连接与探活的超时。
const mongoTimeout = 15 * time.Second

// mongoURI 拼 MongoDB 连接串。用户名与口令走 url.UserPassword 转义，
// 否则口令里的 @ : / % 会把连接串拆坏；认证库（authSource）来自数据源配置，
// 未配置时用 MongoDB 默认的 admin——账号建在业务库上的部署必须显式指定，
// 否则认证必失败（此前硬编码 admin，这类部署连不上）。
func mongoURI(username, password, ip string, port int32, authDB string) string {
	if authDB == "" {
		authDB = "admin"
	}
	u := &url.URL{
		Scheme:   "mongodb",
		User:     url.UserPassword(username, password),
		Host:     net.JoinHostPort(ip, strconv.Itoa(int(port))),
		Path:     "/",
		RawQuery: "authSource=" + url.QueryEscape(authDB),
	}
	return u.String()
}

// connectMongo 按 DataSource 建立 MongoDB 客户端并 Ping 探活。调用方负责 Disconnect。
// 口令是明文：主程序在发起 RPC 前已经把口令列解密好了，引擎侧不再持有主程序密钥。
func connectMongo(ds *api.DataSource) (*mongo.Client, error) {
	if ds == nil || ds.Ip == "" {
		return nil, fmt.Errorf("数据源 IP 为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), mongoTimeout)
	defer cancel()
	// v2 起 Connect 不再收 ctx:连接建立的时限由客户端选项控制(下方已设)
	client, err := mongo.Connect(options.Client().
		ApplyURI(mongoURI(ds.Username, ds.Password, ds.Ip, ds.Port, ds.AuthDb)).
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
func (e *Engine) execMongo(ctx context.Context, req *api.ExecRequest) (*api.ExecReply, error) {
	var cmd bson.D
	if err := bson.UnmarshalExtJSON([]byte(req.Order.Sql), false, &cmd); err != nil {
		return &api.ExecReply{Ok: false, Error: "工单内容不是合法的 MongoDB 命令 JSON：" + err.Error()}, nil
	}
	// 执行前再过一遍硬保底（与提交时同一套）：提交到审批之间工单内容可能被改过
	if _, err := mongodb.Validate(cmd); err != nil {
		return &api.ExecReply{Ok: false, Error: err.Error()}, nil
	}
	// 执行前复检规则集：审批到执行之间规则可能被收紧，只跑硬保底会把新规则跳过。
	// error 级中止执行，warn 级随执行明细一起记下（与申请页检测同一入口、同一规则集）。
	warns, recheckErr := recheckMongoRules(req)
	if recheckErr != nil {
		return &api.ExecReply{Ok: false, Error: recheckErr.Error()}, nil
	}
	maxAffectRows := 0
	if req.Rules != nil {
		maxAffectRows = int(req.Rules.MongoMaxAffectRows)
	}

	client, err := connectMongo(req.Source)
	if err != nil {
		return &api.ExecReply{Ok: false, Error: "连接 MongoDB 失败: " + err.Error()}, nil
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	// 注意（回归点）：createIndexes 的实时构建进度此前由主程序写 osc_info，
	// 引擎没有主程序的元数据库，这里不再挂 WatchIndexBuild 进度回传；
	// 索引构建完成与否仍由执行结果体现。见 design.md 语义核对第 4 条。
	out, err := mongodb.Execute(ctx, client, req.Order.DataBase, cmd, maxAffectRows)
	if err != nil {
		return &api.ExecReply{Ok: false, Error: err.Error(),
			Records: []*api.Record{{Sql: req.Order.Sql, Status: "执行失败", Level: 1, Error: err.Error()}}}, nil
	}

	rec := &api.Record{Sql: req.Order.Sql, Status: "已执行", AffectRows: uint32(out.Affected)}
	notes := append(warns, out.Warning)
	rec.Error = strings.Join(nonEmpty(notes), "；")
	return &api.ExecReply{Ok: true, Records: []*api.Record{rec}, Rollback: out.Rollback}, nil
}

// recheckMongoRules 执行前按当前规则集复检；返回 warning 级说明，error 级直接报错。
func recheckMongoRules(req *api.ExecRequest) ([]string, error) {
	rep := checkMongo(&api.CheckRequest{
		Sql:    req.Order.Sql,
		Schema: req.Order.DataBase,
		Source: req.Source,
		Rule:   req.Rules,
		Mode:   "write",
	})
	if rep == nil || !rep.Ok {
		msg := "引擎未返回复核结果"
		if rep != nil {
			msg = rep.Error
		}
		return nil, errors.New("执行前复核失败: " + msg)
	}
	var warns []string
	for _, r := range rep.Records {
		if r.Level == 1 {
			return nil, errors.New("执行前复核发现阻断级问题: " + r.Error)
		}
		if r.Error != "" {
			warns = append(warns, r.Error)
		}
	}
	return warns, nil
}

func nonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}
