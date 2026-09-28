package fetch

import (
	"Yearning-go/src/handler/common"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/model"
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/cookieY/yee"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	UNDO_EXPR = "username =? AND work_id =? AND `status` =? "
)

// checkSourcePerm 校验当前登录用户是否拥有该数据源的任意权限
func checkSourcePerm(c yee.Context, sourceId string) bool {
	user := new(factory.Token).JwtParse(c).Username
	return common.HasAnySourcePermission(user, sourceId)
}

// checkOrderPerm 校验当前登录用户是否与工单相关（提交人、审批人或历史审批人）
func checkOrderPerm(c yee.Context, workId string) bool {
	user := new(factory.Token).JwtParse(c).Username
	return common.IsOrderRelated(workId, user)
}

// deny 数据源权限不足。ER_USER_NO_PERMISSION 文案里有两个 %s，
// 必须把用户名和数据源补进去，否则前端直接看到字面的 "user:%s没有该数据源(%s)权限"
func deny(c yee.Context, sourceId string) error {
	user := new(factory.Token).JwtParse(c).Username
	return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(
		fmt.Sprintf(i18n.DefaultLang.Load(i18n.ER_USER_NO_PERMISSION), user, sourceId)))
}

// denyOrder 与工单无关：文案不能说成「没有该数据源权限」，会把人往错误方向引
func denyOrder(c yee.Context) error {
	return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_ORDER_NOT_RELATED)))
}

type userProfile struct {
	Department string `gorm:"type:varchar(50);" json:"department"`
	RealName   string `gorm:"type:varchar(50);" json:"real_name"`
	Username   string `gorm:"type:varchar(50);not null;index:user_idx" json:"username"`
	Email      string `gorm:"type:varchar(50);" json:"email"`
}

type referOrder struct {
	Data model.CoreSqlOrder `json:"data"`
	SQLs string             `json:"sqls"`
	Tp   int                `json:"tp"`
}

type PageSizeRef struct {
	WorkId   string `json:"work_id"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

type _FetchBind struct {
	IDC      string             `json:"idc"`
	Tp       string             `json:"tp"`
	Source   string             `json:"source"`
	SourceId string             `json:"source_id"`
	DataBase string             `json:"data_base"`
	Table    string             `json:"table"`
	Rows     []common.FieldInfo `json:"rows"`
	Idx      []common.IndexInfo `json:"idx"`
	Hide     bool               `json:"hide"`
}

type advisorFrom struct {
	SourceID string   `json:"source_id"`
	Schema   string   `json:"data_base"`
	Tables   []string `json:"tables"`
	SQL      string   `json:"sql"`
	Desc     string   `json:"desc"`
	// Mongo 标记目标数据源是 MongoDB，用于挑选合适的提示词模板（不参与请求绑定）
	Mongo bool `json:"-"`
}

type ShowCreateTable struct {
	CreateTable string `gorm:"column:Create Table"`
}

func (a *advisorFrom) Go() (tables []string, err error) {
	var dataSource model.CoreDataSource
	model.DB().Model(model.CoreDataSource{}).Where("source_id =?", a.SourceID).First(&dataSource)

	// MongoDB 没有 SHOW CREATE TABLE 可读：改为对集合取样、汇总字段名
	// （只取字段名不取字段值，业务数据不进 prompt）
	if dataSource.DBType == model.DBTypeMongoDB {
		a.Mongo = true
		names := a.Tables
		if len(names) == 0 {
			// 没选集合时按库内集合取样，限制个数避免 prompt 过长
			all, listErr := dataSource.MongoCollections(a.Schema)
			if listErr != nil {
				return nil, listErr
			}
			if len(all) > model.MongoMaxSampleCollections {
				all = all[:model.MongoMaxSampleCollections]
			}
			names = all
		}
		return dataSource.MongoCollectionSchema(a.Schema, names)
	}

	db, err := dataSource.ConnectDB(a.Schema)
	if err != nil {
		return nil, err
	}
	defer model.Close(db)
	// 库名/表名来自客户端且无法占位符参数化，拼进 SQL 前必须先校验标识符再转义，防止注入
	if !factory.IsValidIdentifier(a.Schema) {
		return nil, errors.New("invalid database name")
	}
	for _, i := range a.Tables {
		if !factory.IsValidIdentifier(i) {
			return nil, errors.New("invalid table name")
		}
		var result ShowCreateTable
		err = db.Raw(fmt.Sprintf("SHOW CREATE TABLE `%s`.`%s`",
			factory.EscapeIdentifier(a.Schema), factory.EscapeIdentifier(i))).Scan(&result).Error
		if err != nil {
			return nil, fmt.Errorf("failed to execute query: %v", err)
		}
		tables = append(tables, result.CreateTable)
	}
	return tables, nil
}

// FetchTableFieldsOrIndexes 取「表结构（字段）」与「索引清单」。
// 所有数据源方言都填同一份契约（common.FieldInfo / common.IndexInfo）：
// 前端表格按返回对象的键动态生成列头，键一致展示格式就一致。
// 新增数据源类型时在这里加一个分支即可，前端无需改动。
func (u *_FetchBind) FetchTableFieldsOrIndexes() error {
	var s model.CoreDataSource

	// 读不到数据源必须直接报错：s 保持零值会让 DBType 变成 0（MySQL），
	// 拿着空配置去连库，最后抛出「密码解析错误」这种与真实原因无关的提示
	if err := model.DB().Where("source_id =?", u.SourceId).First(&s).Error; err != nil {
		return err
	}

	if s.DBType == model.DBTypeMongoDB {
		return u.fetchMongoFieldsOrIndexes(&s)
	}
	return u.fetchSQLFieldsOrIndexes(&s)
}

// fetchSQLFieldsOrIndexes MySQL：SHOW FULL FIELDS / SHOW INDEX
func (u *_FetchBind) fetchSQLFieldsOrIndexes(s *model.CoreDataSource) error {
	db, err := s.ConnectDB(u.DataBase)
	if err != nil {
		return err
	}
	defer model.Close(db)

	// 库名/表名来自客户端且无法占位符参数化，拼进 SQL 前必须先校验标识符再转义，防止注入
	if !factory.IsValidIdentifier(u.DataBase) || !factory.IsValidIdentifier(u.Table) {
		return errors.New("invalid database or table name")
	}
	dbName := factory.EscapeIdentifier(u.DataBase)
	tbName := factory.EscapeIdentifier(u.Table)

	if err := db.Raw(fmt.Sprintf("SHOW FULL FIELDS FROM `%s`.`%s`", dbName, tbName)).Scan(&u.Rows).Error; err != nil {
		return err
	}

	if err := db.Raw(fmt.Sprintf("SHOW INDEX FROM `%s`.`%s`", dbName, tbName)).Scan(&u.Idx).Error; err != nil {
		return err
	}
	return nil
}

// fetchMongoFieldsOrIndexes MongoDB：集合没有固定 schema，表结构由抽样文档汇总出
// 「字段 → 出现过的 BSON 类型」；索引取 listIndexes，复合索引按键顺序摊成多行
// （与 MySQL SHOW INDEX 一个索引多行的形态对齐）。
func (u *_FetchBind) fetchMongoFieldsOrIndexes(s *model.CoreDataSource) error {
	if u.DataBase == "" || u.Table == "" {
		return errors.New(i18n.DefaultLang.Load(i18n.INFO_LIBRARY_NAME_TABLE_NAME))
	}
	client, err := s.ConnectMongo()
	if err != nil {
		return err
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), model.MongoTimeout*4)
	defer cancel()
	coll := client.Database(u.DataBase).Collection(u.Table)

	// 空集合也要回空数组而不是 null，前端 rows?.length 才不会炸
	u.Rows = []common.FieldInfo{}
	u.Idx = []common.IndexInfo{}

	// 用 $sample 而不是 find().limit()：文档模型字段不固定，抽样才能覆盖到不同形态的文档
	cur, err := coll.Aggregate(ctx, bson.A{
		bson.D{{Key: "$sample", Value: bson.D{{Key: "size", Value: model.MongoSampleSize}}}},
		bson.D{{Key: "$project", Value: bson.D{{Key: "f", Value: bson.D{{Key: "$objectToArray", Value: "$$ROOT"}}}}}},
		bson.D{{Key: "$unwind", Value: "$f"}},
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$f.k"},
			{Key: "types", Value: bson.D{{Key: "$addToSet", Value: bson.D{{Key: "$type", Value: "$f.v"}}}}},
		}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	})
	if err != nil {
		return err
	}
	defer func() { _ = cur.Close(ctx) }()
	for cur.Next(ctx) {
		var row struct {
			Key   string   `bson:"_id"`
			Types []string `bson:"types"`
		}
		if err := cur.Decode(&row); err != nil {
			continue
		}
		// 同一字段在不同文档里类型可能不同，排序后再拼，保证每次输出的顺序稳定
		sort.Strings(row.Types)
		f := common.FieldInfo{Field: row.Key, Type: strings.Join(row.Types, " | "), Null: "YES"}
		if row.Key == "_id" {
			f.Key = "PRI"
			f.Null = "NO"
		}
		u.Rows = append(u.Rows, f)
	}
	if err := cur.Err(); err != nil {
		return err
	}

	icur, err := coll.Indexes().List(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = icur.Close(ctx) }()
	for icur.Next(ctx) {
		var doc struct {
			Name   string `bson:"name"`
			Unique bool   `bson:"unique"`
			Key    bson.D `bson:"key"`
		}
		if err := icur.Decode(&doc); err != nil {
			continue
		}
		u.Idx = append(u.Idx, mongoIndexRows(u.Table, doc.Name, doc.Unique, doc.Key)...)
	}
	return icur.Err()
}

// mongoIndexRows 把一个 listIndexes 文档摊平成按 Seq 编号的多行（复合索引一个键一行）
func mongoIndexRows(collection, name string, unique bool, key bson.D) []common.IndexInfo {
	// _id 索引不写 unique 字段，但它天然唯一
	nonUnique := 1
	if unique || name == "_id_" {
		nonUnique = 0
	}
	rows := make([]common.IndexInfo, 0, len(key))
	for i, kv := range key {
		rows = append(rows, common.IndexInfo{
			Table:      collection,
			NonUnique:  nonUnique,
			IndexName:  name,
			Seq:        i + 1,
			ColumnName: kv.Key,
			IndexType:  mongoIndexType(kv.Value),
		})
	}
	return rows
}

// mongoIndexType 把索引键的方向/类型值翻成 MySQL 风格的索引类型名，便于与 MySQL 工单对照
func mongoIndexType(v interface{}) string {
	switch x := v.(type) {
	case int32, int64, float64:
		return "BTREE"
	case string:
		switch x {
		case "text":
			return "TEXT"
		case "2dsphere", "2d":
			return "GEO"
		case "hashed":
			return "HASHED"
		default:
			return strings.ToUpper(x)
		}
	}
	return fmt.Sprint(v)
}
