package personal

import (
	"Yearning-go/src/engine"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/calls"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/model"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/cookieY/sqlx"
	"github.com/cookieY/yee/logger"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unsafe"
	enginev1 "engine/gen/engine/v1"
)

const (
	BUF = 1<<20 - 1
)

func bytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

type QueryDeal struct {
	Ref struct {
		Type     int    `msgpack:"type"` //0 conn 1 close
		Sql      string `msgpack:"sql"`
		Schema   string `msgpack:"schema"`
		SourceId string `json:"source_id"`
	}
	MultiSQLRunner []MultiSQLRunner
}

type MultiSQLRunner struct {
	SQL              string
	InsulateWordList map[string]struct{}
}

type Query struct {
	Field []map[string]interface{} `msgpack:"field"`
	Data  []map[string]interface{} `msgpack:"data"`
}

// identifierRegexp 限定 MySQL 标识符允许的字符，用于阻断标识符注入
var identifierRegexp = regexp.MustCompile(`^[\w$\-]+$`)

// isValidIdentifier 校验库名/表名是否合法，长度遵循 MySQL 64 字符上限
func isValidIdentifier(s string) bool {
	return s != "" && len(s) <= 64 && identifierRegexp.MatchString(s)
}

// escapeIdentifier 在标识符进入反引号前把反引号双写，防止闭合反引号逃逸
func escapeIdentifier(s string) string {
	return strings.ReplaceAll(s, "`", "``")
}

func (q *QueryDeal) PreCheck(insulateWordList string) error {
	client, conn, err := calls.NewClient()
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rep, err := client.Query(ctx, &enginev1.QueryRequest{
		Sql:              q.Ref.Sql,
		Limit:            model.GloOther.Load().Limit,
		InsulateWordList: insulateWordList,
	})
	if rep == nil || !rep.Ok {
		return calls.CombineReplyErr(rep, err)
	}
	for _, i := range rep.Records {
		rec := engine.RecordFromProto(i)
		if rec.Error != "" {
			return errors.New(rec.Error)
		}
		q.MultiSQLRunner = append(q.MultiSQLRunner, MultiSQLRunner{SQL: rec.SQL, InsulateWordList: factory.MapOn(rec.InsulateWordList)})
	}
	return nil
}

func (m *MultiSQLRunner) Run(db *sqlx.DB, schema string) (*Query, error) {
	query := new(Query)
	if db == nil {
		return nil, errors.New(i18n.DefaultLang.Load(i18n.ER_DATABASE_CONNECTION_FAILED))
	}
	// schema 来自 websocket 客户端，必须先校验再拼进 SQL
	if !isValidIdentifier(schema) {
		return nil, errors.New(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE))
	}
	_, err := db.Exec(fmt.Sprintf("use `%s`", escapeIdentifier(schema)))
	if err != nil {
		logger.LogCreator().Error(err)
	}
	rows, err := db.Queryx(m.SQL)

	if err != nil {
		return nil, err
	}

	cols, err := rows.Columns()

	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// 结果集必须有本地上限，远端 LIMIT 缺失时不能把整表打进内存
	limit := model.GloOther.Load().Limit
	if limit == 0 {
		limit = 1000
	}
	for rows.Next() {
		if uint64(len(query.Data)) >= limit {
			break
		}
		results := make(map[string]interface{})
		_ = rows.MapScan(results)
		for key := range results {
			switch r := results[key].(type) {
			case []uint8:
				if len(r) > BUF {
					results[key] = i18n.DefaultLang.Load(i18n.ER_BLOB_FIELD_NOT_DISPLAYABLE)
				} else {
					switch hex.EncodeToString(r) {
					case "01":
						results[key] = "true"
					case "00":
						results[key] = "false"
					default:
						results[key] = bytesToString(r)
					}
					if m.excludeFieldContext(key) {
						results[key] = i18n.DefaultLang.Load(i18n.INFO_SENSITIVE_FIELD)
					}
				}
			case int64:
				if m.excludeFieldContext(key) {
					results[key] = i18n.DefaultLang.Load(i18n.INFO_SENSITIVE_FIELD)
				} else {
					results[key] = strconv.FormatInt(r, 10)
				}
			case uint64:
				if m.excludeFieldContext(key) {
					results[key] = i18n.DefaultLang.Load(i18n.INFO_SENSITIVE_FIELD)
				} else {
					results[key] = strconv.FormatUint(r, 10)
				}
			}
		}
		query.Data = append(query.Data, results)
	}

	ele := removeDuplicateElement(cols)

	for cv := range ele {
		query.Field = append(query.Field, map[string]interface{}{"title": ele[cv], "dataIndex": ele[cv], "width": 200, "resizable": true, "ellipsis": true})
	}
	// 无结果集时 cols 为空，直接索引会 panic
	if len(query.Field) > 0 {
		query.Field[0]["fixed"] = "left"
	}
	return query, nil
}

func (m *MultiSQLRunner) excludeFieldContext(field string) bool {
	_, ok := m.InsulateWordList[strings.ToLower(field)]
	return ok
}

func removeDuplicateElement(addrs []string) []string {
	result := make([]string, 0, len(addrs))
	temp := map[string]struct{}{}
	idx := 0
	for _, item := range addrs {
		if _, ok := temp[item]; !ok {
			temp[item] = struct{}{}
			result = append(result, item)
		} else {
			idx++
			item += fmt.Sprintf("(%v)", idx)
			result = append(result, item)
		}
	}
	return result
}
