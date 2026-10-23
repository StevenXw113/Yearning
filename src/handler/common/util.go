package common

import (
	"Yearning-go/src/lib/dbengine"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/model"
	"errors"
	"fmt"
	"github.com/cookieY/yee/logger"
	"strings"
)

func unifiedLabel(col []string) []any {
	var s []any
	for i := 0; i < len(col); i++ {
		var t string
		s = append(s, &t)
	}
	return s
}

func ScanDataRows(s model.CoreDataSource, database, sql, meta string, isQuery bool, isLeaf bool) (*_dbInfo, error) {
	res := new(_dbInfo)
	db, err := s.ConnectDB(database)
	if err != nil {
		return res, err
	}

	defer func() {
		_ = model.Close(db)
	}()
	rows, err := db.Raw(sql).Rows()
	if err != nil {
		return nil, err
	}
	// 不关闭 rows 会一直占用连接池中的连接
	defer rows.Close()
	col, _ := rows.Columns()
	_tmp := unifiedLabel(col)
	if len(_tmp) == 0 {
		return nil, errors.New("field is empty")
	}
	excludeDbList := factory.MapOn(strings.Split(s.ExcludeDbList, ","))
	for rows.Next() {
		if err = rows.Scan(_tmp...); err != nil {
			logger.DefaultLogger.Error(err)
		}
		j := *_tmp[0].(*string)
		if isQuery {
			if len(excludeDbList) > 0 {
				if _, ok := excludeDbList[j]; ok {
					continue
				}
			}
			// db_type 一并下发：前端按数据源类型生成右键的默认语句（Mongo 与 SQL 语法不同）
			res.QueryList = append(res.QueryList, map[string]interface{}{"title": j, "key": checkMeta(j, database, meta), "meta": meta, "isLeaf": isLeaf, "db_type": s.DBType})
		} else {
			res.Results = append(res.Results, j)
		}
	}
	return res, nil
}

func checkMeta(s, database, flag string) string {
	if flag == "Table" {
		return fmt.Sprintf("`%s`.`%s`", database, s)
	}
	return s
}

// Highlight 返回补全建议。各引擎的取法已收敛到 lib/dbengine。
func Highlight(s *model.CoreDataSource, isField string, dbName string) []map[string]string {
	return dbengine.For(s.DBType).Highlight(s, isField == "true", dbName)
}

func SuccessPayload(payload interface{}) Resp {
	return Resp{
		Payload: payload,
		Code:    1200,
	}
}

func SuccessPayLoadToMessage(text string) Resp {
	return Resp{
		Text: text,
		Code: 1200,
	}
}
