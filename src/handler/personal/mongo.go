package personal

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"Yearning-go/src/engine"
	"Yearning-go/src/handler/common"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/calls"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/lib/mongodb"
	"Yearning-go/src/model"

	enginev1 "engine/gen/engine/v1"

	"github.com/cookieY/yee"
	"github.com/vmihailenco/msgpack/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/net/websocket"
	"io"
)

// mongoCommandTimeout 单条 Mongo 命令的超时（聚合可能较慢，比建连宽松）
const mongoCommandTimeout = 60 * time.Second

// FetchMongoDatabaseInfo 列出 Mongo 实例里的业务库（库表树第一层，契约与 MySQL 版一致）
func FetchMongoDatabaseInfo(u *model.CoreDataSource) ([]map[string]interface{}, error) {
	names, err := u.MongoDatabases()
	if err != nil {
		return nil, err
	}
	list := make([]map[string]interface{}, 0, len(names))
	for _, n := range names {
		// 带上 db_type：前端左树按根节点的类型决定右键生成 SQL 还是命令 JSON
		list = append(list, map[string]interface{}{
			"title": n, "key": n, "meta": "Schema", "isLeaf": false, "db_type": model.DBTypeMongoDB,
		})
	}
	return list, nil
}

// FetchMongoCollectionInfo 列出库下的集合（树第二层）。
// key 就是集合名（MySQL 版是 `db`.`table`）：前端按数据源类型生成对应的查询语句。
func FetchMongoCollectionInfo(u *model.CoreDataSource, schema string) ([]map[string]interface{}, error) {
	if !factory.IsValidIdentifier(schema) {
		return nil, errors.New(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE))
	}
	names, err := u.MongoCollections(schema)
	if err != nil {
		return nil, err
	}
	list := make([]map[string]interface{}, 0, len(names))
	for _, n := range names {
		list = append(list, map[string]interface{}{
			"title": n, "key": n, "meta": "Table", "isLeaf": true, "db_type": model.DBTypeMongoDB,
		})
	}
	return list, nil
}

// socketMongoResults 与 SocketQueryResults 同构的 Mongo 版本：
// 权限校验、申请校验、审计落库、消息格式都与 SQL 版保持一致，只有"执行"这步换成 Mongo 命令。
func socketMongoResults(c yee.Context, ws *websocket.Conn, u model.CoreDataSource, user string) {
	client, err := u.ConnectMongo()
	if err != nil {
		c.Logger().Error(err)
		_ = websocket.Message.Send(ws, factory.ToMsg(queryResults{Error: err.Error()}))
		return
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	var b []byte
	for {
		if err := websocket.Message.Receive(ws, &b); err != nil {
			if err != io.EOF {
				c.Logger().Error(err)
			}
			return
		}
		if string(b) == "ping" {
			_ = websocket.Message.Send(ws, factory.ToMsg(queryResults{HeartBeat: common.Pong, IsOnly: model.GloOther.Load().Query}))
			continue
		}
		msg := new(QueryDeal)
		if err := msgpack.Unmarshal(b, &msg.Ref); err != nil {
			c.Logger().Error(err)
			return
		}
		d, ok := permitQueryOrder(user)
		if !ok {
			_ = websocket.Message.Send(ws, factory.ToMsg(queryResults{Status: true}))
			continue
		}
		// 查询侧规则审核（引擎，可配置）：拦截级命中就不执行，提示级照常执行但留痕。
		audit, blocked := checkMongoQuery(&u, msg.Ref.Schema, msg.Ref.Sql)
		if blocked != nil {
			saveQueryRecord(d, msg.Ref.Sql, u.Source, msg.Ref.Schema, 0, blocked.Error())
			_ = websocket.Message.Send(ws, factory.ToMsg(queryResults{Error: blocked.Error()}))
			continue
		}
		clock := time.Now()
		result, err := runMongoCommand(client, msg.Ref.Schema, msg.Ref.Sql, u.InsulateWordList)
		if err != nil {
			_ = websocket.Message.Send(ws, factory.ToMsg(queryResults{Error: err.Error()}))
			continue
		}
		cost := int(time.Since(clock).Seconds() * 1000)
		saveQueryRecord(d, msg.Ref.Sql, u.Source, msg.Ref.Schema, cost, audit)
		if err := websocket.Message.Send(ws, factory.ToMsg(queryResults{
			Export: d.Export == 1, Results: []*Query{result}, QueryTime: cost,
		})); err != nil {
			c.Logger().Error(err)
			return
		}
	}
}

// mongoQueryCheckTimeout 查询侧规则审核的超时（与申请页检测一致）
const mongoQueryCheckTimeout = 30 * time.Second

// checkMongoQuery 把只读命令送引擎做查询侧规则审核（mode=query）。
//
// 返回要写进查询记录的命中说明，以及拦截级错误（非 nil 时不要执行）。
//
// 引擎不可用**不拦查询**：查询是只读的，本地已有只读白名单硬保底
// （mongodb.ValidateQuery），没必要因为引擎离线把整个查询功能拿掉；
// 这种情况会在审计记录里写清「未做规则审核」。
func checkMongoQuery(source *model.CoreDataSource, schema, command string) (string, error) {
	rule, rerr := factory.CheckDataSourceRule(source.RuleId)
	if rerr != nil {
		return "规则集加载失败，未做查询侧规则审核：" + rerr.Error(), nil
	}
	client, conn, err := calls.NewClient()
	if err != nil {
		return "引擎不可用，未做查询侧规则审核：" + err.Error(), nil
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), mongoQueryCheckTimeout)
	defer cancel()
	rep, err := client.Check(ctx, &enginev1.CheckRequest{
		Sql:    command,
		Schema: schema,
		Source: &enginev1.DataSource{Kind: calls.DataSourceKind(source.DBType)},
		Lang:   model.C.General.Lang,
		Rule:   engine.AuditRoleToProto(rule),
		Mode:   engine.CheckModeQuery,
	})
	if err != nil {
		return "引擎不可用，未做查询侧规则审核：" + err.Error(), nil
	}
	if rep == nil || !rep.Ok {
		msg := "查询侧审核不通过"
		if rep != nil && rep.GetError() != "" {
			msg = rep.GetError()
		}
		return "", errors.New(msg)
	}
	var notes []string
	for _, r := range rep.Records {
		if r.GetLevel() == 1 {
			return "", errors.New("查询侧规则拦截：" + r.GetError())
		}
		if r.GetError() != "" {
			notes = append(notes, r.GetError())
		}
	}
	return strings.Join(notes, "；"), nil
}

// runMongoCommand 执行一条 MongoDB 命令并转成前端表格契约。
// 命令就是 db.runCommand 的 JSON 体（如 {"find":"users","filter":{},"limit":10}）：
// 透传命令而非自造查询语法，find / aggregate / count / distinct 等原生能力不需要额外解析器。
func runMongoCommand(client *mongo.Client, schema, command, insulateWordList string) (*Query, error) {
	if !factory.IsValidIdentifier(schema) {
		return nil, errors.New(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE))
	}
	var cmd bson.D
	// relaxed 模式：允许直接写普通 JSON 数字，不必包成 {"$numberInt":"20"}
	if err := bson.UnmarshalExtJSON([]byte(command), false, &cmd); err != nil {
		return nil, errors.New(`请填写 MongoDB 命令的 JSON，例如 {"find":"users","filter":{},"limit":10}：` + err.Error())
	}
	// 查询页的硬保底：只读命令白名单（含 aggregate 的 $out / $merge 写库阶段）。
	// 不走规则集——能在这里执行写命令，等于绕过整个工单审批链路。
	if err := mongodb.ValidateQuery(cmd); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), mongoCommandTimeout)
	defer cancel()
	var raw bson.M
	if err := client.Database(schema).RunCommand(ctx, cmd).Decode(&raw); err != nil {
		return nil, err
	}

	// 脱敏沿用 SQL 版那一套：按字段名比对（大小写不敏感）
	spec := MultiSQLRunner{InsulateWordList: factory.MapOn(lowerList(strings.Split(insulateWordList, ",")))}
	limit := model.GloOther.Load().Limit
	if limit == 0 {
		limit = 1000
	}

	query := new(Query)
	fields := make(map[string]struct{})
	for _, row := range mongoRows(raw) {
		if uint64(len(query.Data)) >= limit {
			break
		}
		out := make(map[string]interface{}, len(row))
		for k, v := range row {
			if _, ok := fields[k]; !ok {
				fields[k] = struct{}{}
				query.Field = append(query.Field, map[string]interface{}{
					"title": k, "dataIndex": k, "width": 200, "resizable": true, "ellipsis": true,
				})
			}
			if spec.excludeFieldContext(k) {
				out[k] = i18n.DefaultLang.Load(i18n.INFO_SENSITIVE_FIELD)
				continue
			}
			out[k] = mongoDisplay(v)
		}
		query.Data = append(query.Data, out)
	}
	// Mongo 文档没有固定 schema，map 迭代顺序随机；按字段名排序让每次列序稳定
	sort.Slice(query.Field, func(i, j int) bool {
		return query.Field[i]["title"].(string) < query.Field[j]["title"].(string)
	})
	if len(query.Field) > 0 {
		query.Field[0]["fixed"] = "left"
	}
	return query, nil
}

// mongoRows 取命令响应里的行集：cursor.firstBatch 是 find / aggregate 的结果，
// 其余命令（count、distinct、listCollections 等）把整个响应当作一行展示。
func mongoRows(raw bson.M) []bson.M {
	if cur, ok := asMap(raw["cursor"]); ok {
		if batch, ok := asSlice(cur["firstBatch"]); ok {
			rows := make([]bson.M, 0, len(batch))
			for _, item := range batch {
				if m, ok := asMap(item); ok {
					rows = append(rows, m)
				}
			}
			return rows
		}
	}
	return []bson.M{raw}
}

// asMap 兼容驱动可能给出的几种文档表示（bson.M / map / bson.D）
func asMap(v interface{}) (bson.M, bool) {
	switch x := v.(type) {
	case bson.M:
		return x, true
	case map[string]interface{}:
		return bson.M(x), true
	case bson.D:
		return x.Map(), true
	}
	return nil, false
}

func asSlice(v interface{}) ([]interface{}, bool) {
	switch x := v.(type) {
	case bson.A:
		return x, true
	case []interface{}:
		return x, true
	}
	return nil, false
}

// mongoDisplay 把 bson 值转成前端表格能直接渲染的形式：
// 标量原样返回，ObjectID / 时间给出可读字符串，嵌套文档与数组转成扩展 JSON 文本。
func mongoDisplay(v interface{}) interface{} {
	switch x := v.(type) {
	case nil, string, bool, int32, int64, float64:
		return x
	case primitive.ObjectID:
		return x.Hex()
	case primitive.DateTime:
		return x.Time().Format("2006-01-02 15:04:05")
	case time.Time:
		return x.Format("2006-01-02 15:04:05")
	case []byte:
		return string(x)
	default:
		b, err := bson.MarshalExtJSON(v, false, false)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(b)
	}
}
