package fetch

import (
	"Yearning-go/src/handler/common"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/model"
	"fmt"
	"net/http"

	"github.com/cookieY/yee"
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

func deny(c yee.Context) error {
	return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_USER_NO_PERMISSION)))
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
}

type ShowCreateTable struct {
	CreateTable string `gorm:"column:Create Table"`
}

func (a *advisorFrom) Go() (tables []string, err error) {
	var dataSource model.CoreDataSource
	model.DB().Model(model.CoreDataSource{}).Where("source_id =?", a.SourceID).First(&dataSource)
	db, err := dataSource.ConnectDB(a.Schema)
	if err != nil {
		return nil, err
	}
	defer model.Close(db)
	for _, i := range a.Tables {
		var result ShowCreateTable
		err = db.Raw(fmt.Sprintf("SHOW CREATE TABLE %s.%s", a.Schema, i)).Scan(&result).Error
		if err != nil {
			return nil, fmt.Errorf("failed to execute query: %v", err)
		}
		tables = append(tables, result.CreateTable)
	}
	return tables, nil
}

func (u *_FetchBind) FetchTableFieldsOrIndexes() error {
	var s model.CoreDataSource

	model.DB().Where("source_id =?", u.SourceId).First(&s)

	db, err := s.ConnectDB(u.DataBase)
	if err != nil {
		return err
	}

	defer model.Close(db)

	if err := db.Raw(fmt.Sprintf("SHOW FULL FIELDS FROM `%s`.`%s`", u.DataBase, u.Table)).Scan(&u.Rows).Error; err != nil {
		return err
	}

	if err := db.Raw(fmt.Sprintf("SHOW INDEX FROM `%s`.`%s`", u.DataBase, u.Table)).Scan(&u.Idx).Error; err != nil {
		return err
	}
	return nil
}
