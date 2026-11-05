package personal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"Yearning-go/src/handler/common"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/lib/rediscmd"
	"Yearning-go/src/model"

	"engine/api"

	"github.com/cookieY/yee"
	"github.com/redis/go-redis/v9"
	"github.com/vmihailenco/msgpack/v5"
	"golang.org/x/net/websocket"
)

// redisCommandTimeout 单条 Redis 命令的超时。
// 正常命令是毫秒级的，30 秒已相当宽裕；需要长时间执行的命令
// （KEYS 全量遍历之类）本来就被只读白名单与审核规则挡在外面。
const redisCommandTimeout = 30 * time.Second

// FetchRedisDatabaseInfo 列出 Redis 的逻辑库（db0..dbN，树的第一层）。
// 契约与 MySQL / Mongo 版一致：带上 db_type，前端据此决定右键生成什么语句。
func FetchRedisDatabaseInfo(u *model.CoreDataSource) ([]map[string]interface{}, error) {
	names, err := u.RedisDatabases()
	if err != nil {
		return nil, err
	}
	return treeNodes(names, "Schema", false, model.DBTypeRedis), nil
}

// FetchRedisKeyInfo 列出某个逻辑库里的键（树的第二层）。
//
// ⚠️ Redis 没有「表」这个概念：这一层就是**键名**，而且只取前 RedisScanLimit 条——
// 键空间可能上百万，SCAN 全量遍历既慢也没意义。前端把它当「表名」用
// （选中后生成 GET / TYPE / TTL 这类命令，工单的 table 字段存的也是键名）。
func FetchRedisKeyInfo(u *model.CoreDataSource, schema string) ([]map[string]interface{}, error) {
	db, err := model.RedisParseDB(schema)
	if err != nil {
		return nil, err
	}
	keys, err := u.RedisScanKeys(db, "", model.RedisScanLimit)
	if err != nil {
		return nil, err
	}
	return treeNodes(keys, "Table", true, model.DBTypeRedis), nil
}

// RedisKeyDetail 把「看这个键」翻译成一条按类型选择的只读命令（GET 对 hash 会 WRONGTYPE）。
func RedisKeyDetail(u *model.CoreDataSource, db int, key string) (map[string]interface{}, error) {
	var detail map[string]interface{}
	err := u.WithRedisClient(db, func(client model.RedisConn) error {
		ctx, cancel := context.WithTimeout(context.Background(), redisCommandTimeout)
		defer cancel()

		typ, err := client.Type(ctx, key).Result()
		if err != nil {
			return err
		}
		detail = map[string]interface{}{
			"key":  key,
			"type": typ,
			"sql":  redisPreviewSQL(typ, key),
		}
		// 下面三项是锦上添花：托管实例可能禁用 TTL 之外的对象命令，取不到就省略
		if ttl, terr := client.TTL(ctx, key).Result(); terr == nil {
			// go-redis 用负值表示「无过期(-1) / 键不存在(-2)」，直接除会变成 0
			sec := int64(ttl / time.Second)
			if ttl < 0 {
				sec = int64(ttl)
			}
			detail["ttl"] = sec
		}
		if size, serr := client.Do(ctx, "MEMORY", "USAGE", key).Int64(); serr == nil {
			detail["size"] = size
		}
		if enc, eerr := client.ObjectEncoding(ctx, key).Result(); eerr == nil {
			detail["encoding"] = enc
		}
		return nil
	})
	return detail, err
}

// redisPreviewSQL 按值类型选一条只读命令；none（键不存在）不生成命令。
// 集合类都带上限：查询页是同步等结果的，全量读取会拖垮连接，翻页交给用户自己写。
func redisPreviewSQL(typ, key string) string {
	// 键名必须转义：含空格会被切词拆成多个参数、含引号会让切词报错
	k := rediscmd.Quote(key)
	switch strings.ToLower(typ) {
	case "string":
		return "GET " + k
	case "hash":
		return "HGETALL " + k
	case "list":
		return "LRANGE " + k + " 0 99"
	case "set":
		return "SSCAN " + k + " 0 COUNT 100"
	case "zset":
		return "ZRANGE " + k + " 0 99 WITHSCORES"
	case "stream":
		return "XRANGE " + k + " - + COUNT 20"
	}
	return ""
}

// socketRedisResults 与 SocketQueryResults 同构的 Redis 版本：
// 权限校验、申请校验、查询侧审核、审计落库、消息格式都与 SQL 版保持一致，
// 只有「执行」这步换成 Redis 命令，且按目标逻辑库建连。
//
// 客户端按库复用走进程级连接池（model.WithRedisClient）：go-redis 在建连时发 SELECT，
// 换库必须换客户端，但同一数据源×库在整个进程内只需一个客户端；
// 借出/归还由引用计数管理，编辑数据源不会打断在途命令。
func socketRedisResults(c yee.Context, ws *websocket.Conn, u model.CoreDataSource, user string) {
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
		audit, blocked := checkQuerySide(&u, msg.Ref.Schema, msg.Ref.Sql)
		if blocked != nil {
			saveQueryRecord(d, msg.Ref.Sql, u.Source, msg.Ref.Schema, 0, blocked.Error())
			_ = websocket.Message.Send(ws, factory.ToMsg(queryResults{Error: blocked.Error()}))
			continue
		}
		// 先校验（不建连）：写命令、越权诊断命令在借客户端之前就拒绝
		db, args, cerr := prepareRedisCommand(msg.Ref.Schema, msg.Ref.Sql, u.AllowDiagCommands)
		if cerr != nil {
			_ = websocket.Message.Send(ws, factory.ToMsg(queryResults{Error: cerr.Error()}))
			continue
		}
		clock := time.Now()
		var result *Query
		err := u.WithRedisClient(db, func(client model.RedisConn) error {
			var rerr error
			result, rerr = runRedisCommand(client, args, u.InsulateWordList)
			return rerr
		})
		if err != nil {
			_ = websocket.Message.Send(ws, factory.ToMsg(queryResults{Error: err.Error()}))
			continue
		}
		cost := int(time.Since(clock).Seconds() * 1000)
		saveQueryRecord(d, msg.Ref.Sql, u.Source, msg.Ref.Schema, cost, audit)
		if err := websocket.Message.Send(ws, factory.ToMsg(queryResults{
			Export: d.Export == 1, Results: []*Query{result}, QueryTime: cost, Audit: audit,
		})); err != nil {
			c.Logger().Error(err)
			return
		}
	}
}

// prepareRedisCommand 校验只读白名单与诊断授权，并把库名解析成库号。
// 与执行分开：非法命令必须在借客户端、建连之前就被拒绝。
//
// 命令是**一行文本**（`GET user:1`、`SCAN 0 MATCH user:* COUNT 100`），
// 与 Mongo 的 JSON 命令不同，所以先用 rediscmd 切词再交给驱动。
func prepareRedisCommand(schema, command string, allowDiag bool) (int, []string, error) {
	// 查询页硬保底：只读命令白名单。不走规则集——能在这里执行写命令，
	// 等于绕过整个工单审批链路。
	kind, args, err := rediscmd.Validate(command)
	if err != nil {
		return 0, nil, err
	}
	if kind != rediscmd.KindQuery {
		return 0, nil, errors.New("查询页只能执行只读命令（GET / SCAN / HGETALL / LRANGE 等），变更请到工单申请页提交")
	}
	// 执行授权：只读命令也分档。诊断类（SLOWLOG/LATENCY/DUMP/COMMAND/INFO 全量）
	// 能看到配置或其它业务信息，默认关闭，按数据源开关放行。
	if ok, reason := (api.RedisPolicy{AllowDiag: allowDiag}).QueryAllowed(args); !ok {
		return 0, nil, errors.New(reason)
	}
	db, err := model.RedisParseDB(schema)
	if err != nil {
		return 0, nil, err
	}
	return db, args, nil
}

// runRedisCommand 用借到的客户端执行一条只读命令并转成前端表格契约。
// args 由 prepareRedisCommand 校验并切好词。
func runRedisCommand(client model.RedisConn, args []string, insulateWordList string) (*Query, error) {
	ctx, cancel := context.WithTimeout(context.Background(), redisCommandTimeout)
	defer cancel()

	iface := make([]interface{}, 0, len(args))
	for _, a := range args {
		iface = append(iface, a)
	}
	res, err := client.Do(ctx, iface...).Result()
	// 键不存在不是错误：GET/TTL 之类的空结果是常态，按 (nil) 展示即可，
	// 弹一条红色 "redis: nil" 反而像是命令执行失败
	if errors.Is(err, redis.Nil) {
		res, err = nil, nil
	}
	if err != nil {
		return nil, err
	}
	return redisQuery(rediscmd.CommandName(args), args, res, insulateWordList), nil
}

// redisQuery 把 Redis 的返回值转成前端表格契约。
//
// Redis 的返回形态比 SQL / Mongo 都杂：标量（string / int64 / nil）、
// 一维数组（LRANGE / SMEMBERS / MGET）、二维数组（SCAN 的 [游标, 键列表]）。
// 统一成几类：
//   - nil            → 一行 "(nil)"
//   - HGETALL        → field / value 两列（返回值本来就是 field,value 交替）
//   - 一维数组       → index / value 两列
//   - 二维数组(SCAN) → index / value 两列，内部数组用 JSON 展示
//   - 标量           → 一行一列 value
//
// 脱敏按**键名**判定（与 SQL / Mongo 按字段名的口径一致）：Redis 的值是
// 任意字节串或结构化数据，没有"字段名"可用，只能以键为粒度。
func redisQuery(name string, args []string, res interface{}, insulateWordList string) *Query {
	spec := MultiSQLRunner{InsulateWordList: factory.MapOn(lowerList(strings.Split(insulateWordList, ",")))}
	limit := model.GloOther.Load().Limit
	if limit == 0 {
		limit = 1000
	}
	// sensitive 判断键名是否命中脱敏词表。
	//
	// 与 SQL 侧的差别在命名习惯：SQL 按字段名精确匹配（excludeFieldContext），
	// 而 Redis 的键名常写成 `业务:标识`（user:email、order:phone），
	// 只做精确匹配的话配了词表也几乎不会生效。所以这里在精确匹配之外，
	// 再把键名按 `:` 分段逐段比对——两者是包含关系：键名本身或任一段命中即脱敏。
	// 匹配函数仍复用 SQL 侧那一套（大小写不敏感），没有另起一套规则。
	sensitive := func(key string) bool {
		if key == "" {
			return false
		}
		if spec.excludeFieldContext(key) {
			return true
		}
		for _, seg := range strings.Split(key, ":") {
			if seg != "" && spec.excludeFieldContext(seg) {
				return true
			}
		}
		return false
	}

	query := new(Query)
	column := func(title string, first bool) map[string]interface{} {
		col := map[string]interface{}{"title": title, "dataIndex": title, "width": 200, "resizable": true, "ellipsis": true}
		if first {
			col["fixed"] = "left"
		}
		return col
	}
	// 命令作用的键：第一个参数（GET user:1 → user:1）。
	// Redis 的值没有"字段名"，脱敏只能以键为粒度。
	key := ""
	if len(args) > 1 {
		key = args[1]
	}

	// 单键命令把键名单独成一列：只看到 value 不知道查的是哪个键
	showKey := scalarKeyCommands[strings.ToUpper(name)] && key != ""
	scalar := func(v interface{}) *Query {
		val := redisDisplay(v)
		if sensitive(key) {
			val = i18n.DefaultLang.Load(i18n.INFO_SENSITIVE_FIELD)
		}
		q := new(Query)
		if showKey {
			q.Field = []map[string]interface{}{column("key", true), column("value", false)}
			q.Data = []map[string]interface{}{{"key": key, "value": val}}
			return q
		}
		q.Field = []map[string]interface{}{column("value", true)}
		q.Data = []map[string]interface{}{{"value": val}}
		return q
	}

	// mapQuery 把 field/value 对转成两列表格（map 无序，按字段名排序保证展示稳定）
	mapQuery := func(pairs [][2]string) *Query {
		sort.Slice(pairs, func(i, j int) bool { return pairs[i][0] < pairs[j][0] })
		query.Field = []map[string]interface{}{column("field", true), column("value", false)}
		for _, p := range pairs {
			if uint64(len(query.Data)) >= limit {
				break
			}
			val := p[1]
			if sensitive(p[0]) {
				val = i18n.DefaultLang.Load(i18n.INFO_SENSITIVE_FIELD)
			}
			query.Data = append(query.Data, map[string]interface{}{"field": p[0], "value": val})
		}
		return query
	}

	if res == nil {
		return scalar("(nil)")
	}

	switch v := res.(type) {
	case string, int64, float64, bool:
		return scalar(v)
	case map[interface{}]interface{}:
		// HGETALL 等的真实返回形态：go-redis 的 Do（Cmd）对 map 型 reply 用的是
		// map[interface{}]interface{}，不是 map[string]string，也不是 field,value 交替数组
		// ——这点是实测出来的（见 redis_test.go 的用例）。
		pairs := make([][2]string, 0, len(v))
		for k, val := range v {
			pairs = append(pairs, [2]string{fmt.Sprintf("%v", k), fmt.Sprintf("%v", val)})
		}
		return mapQuery(pairs)
	case map[string]string:
		// 另一条路径上的 map（例如未来换成类型化 API）同样按两列展示
		pairs := make([][2]string, 0, len(v))
		for k, val := range v {
			pairs = append(pairs, [2]string{k, val})
		}
		return mapQuery(pairs)
	case []interface{}:
		// MGET：返回值与参数里的键一一对应，按 key/value 两列展示
		if strings.EqualFold(name, "MGET") {
			keys := args[1:]
			query.Field = []map[string]interface{}{column("key", true), column("value", false)}
			for i, item := range v {
				if uint64(len(query.Data)) >= limit {
					break
				}
				k := ""
				if i < len(keys) {
					k = keys[i]
				}
				val := redisDisplay(item)
				if sensitive(k) {
					val = i18n.DefaultLang.Load(i18n.INFO_SENSITIVE_FIELD)
				}
				query.Data = append(query.Data, map[string]interface{}{"key": k, "value": val})
			}
			return query
		}
		// SCAN 家族返回 [游标, 元素列表]：展开成「游标 + 元素」逐行，
		// 别把整段数组塞进一个格子里（原来显示成 index/value 两行，游标和键都看不懂）
		if isScanCommand(name) && len(v) == 2 {
			cursor := redisDisplay(v[0])
			items, _ := v[1].([]interface{})
			title := scanItemTitle(name)
			query.Field = []map[string]interface{}{column("cursor", true), column(title, false)}
			if len(items) == 0 {
				query.Data = []map[string]interface{}{{"cursor": cursor, title: "(nil)"}}
				return query
			}
			for _, item := range items {
				if uint64(len(query.Data)) >= limit {
					break
				}
				query.Data = append(query.Data, map[string]interface{}{"cursor": cursor, title: redisDisplay(item)})
			}
			return query
		}
		// HGETALL：返回值是 field, value 交替，转成两列更贴近使用习惯
		if strings.EqualFold(name, "HGETALL") && len(v)%2 == 0 {
			query.Field = []map[string]interface{}{column("field", true), column("value", false)}
			for i := 0; i+1 < len(v) && uint64(len(query.Data)) < limit; i += 2 {
				f := redisDisplay(v[i])
				val := redisDisplay(v[i+1])
				if sensitive(f) {
					val = i18n.DefaultLang.Load(i18n.INFO_SENSITIVE_FIELD)
				}
				query.Data = append(query.Data, map[string]interface{}{"field": f, "value": val})
			}
			return query
		}
		query.Field = []map[string]interface{}{column("index", true), column("value", false)}
		for i, item := range v {
			if uint64(len(query.Data)) >= limit {
				break
			}
			query.Data = append(query.Data, map[string]interface{}{
				"index": i,
				"value": redisDisplay(item),
			})
		}
		return query
	default:
		return scalar(fmt.Sprintf("%v", res))
	}
}

// scalarKeyCommands 只作用于单个键、返回标量的命令：展示时把键名单独成一列
var scalarKeyCommands = map[string]bool{
	// 只登记查询页白名单里的命令：写命令（GETDEL/GETSET/GETEX）永远到不了这里
	"GET": true, "STRLEN": true,
	"GETRANGE": true, "TYPE": true, "TTL": true, "PTTL": true,
	"HGET": true, "HLEN": true, "HEXISTS": true,
	"LLEN": true, "LINDEX": true, "LPOS": true,
	"SCARD": true, "SISMEMBER": true,
	"ZCARD": true, "ZSCORE": true, "ZRANK": true, "ZREVRANK": true,
	"XLEN": true, "GETBIT": true, "BITCOUNT": true, "BITPOS": true,
}

// isScanCommand 游标遍历类命令：返回 [游标, 元素列表]
func isScanCommand(name string) bool {
	switch strings.ToUpper(name) {
	case "SCAN", "HSCAN", "SSCAN", "ZSCAN":
		return true
	}
	return false
}

// scanItemTitle SCAN 家族里「元素」的列名
func scanItemTitle(name string) string {
	switch strings.ToUpper(name) {
	case "SCAN":
		return "key"
	case "HSCAN":
		return "field"
	}
	return "member"
}

// redisDisplay 把 Redis 返回的单个元素转成可展示的字符串。
// 嵌套数组（SCAN 的键列表）用 JSON 展示，其余直接字符串化。
//
// 超长值不往前端发：与 SQL 侧对 BLOB 的处理一致（BUF 上限），
// 否则一个几十兆的 value 会把 WS 消息与浏览器一起拖死。
func redisDisplay(v interface{}) string {
	s := redisDisplayRaw(v)
	if len(s) > BUF {
		return i18n.DefaultLang.Load(i18n.ER_BLOB_FIELD_NOT_DISPLAYABLE)
	}
	return s
}

func redisDisplayRaw(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return "(nil)"
	case string:
		return t
	case []byte:
		return string(t)
	case []interface{}, []string, map[string]interface{}:
		if b, err := json.Marshal(t); err == nil {
			return string(b)
		}
		return fmt.Sprintf("%v", t)
	default:
		return fmt.Sprintf("%v", t)
	}
}
