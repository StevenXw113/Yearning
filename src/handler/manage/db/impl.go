package db

import (
	"Yearning-go/src/handler/common"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/enc"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/model"
	"encoding/json"
	"github.com/google/uuid"
	drive "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type CommonDBPost struct {
	Encrypt       bool                 `json:"encrypt"`
	Tp            string               `json:"tp"`
	DB            model.CoreDataSource `json:"db"`
	ExcludeDbList []string             `json:"exclude_db_list"`
	WordList      []string             `json:"word_list"`
}

func ConnTest(u *model.CoreDataSource) error {
	dsn, err := model.InitDSN(model.DSN{
		Username: u.Username,
		Password: u.Password,
		Host:     u.IP,
		Port:     u.Port,
		DBName:   "",
		CA:       u.CAFile,
		Cert:     u.Cert,
		Key:      u.KeyFile,
	})
	if err != nil {
		return err
	}
	db, err := gorm.Open(drive.New(drive.Config{
		DSN:                       dsn,
		DefaultStringSize:         256,   // string 类型字段的默认长度
		SkipInitializeWithVersion: false, // 根据当前 MySQL 版本自动配置
	}), &gorm.Config{})
	if err != nil {
		return err
	}
	d, err := db.DB()
	if err != nil {
		return err
	}
	return d.Close()
}

func SuperEditSource(source *model.CoreDataSource) common.Resp {
	// 只有当提交值无法解密（说明是明文新口令）时才重新加密写回，
	// 避免把已经是密文的值二次加密。加密失败必须中止，否则口令会被清空。
	if source.Password != "" && enc.Decrypt(model.C.General.SecretKey, source.Password) == "" {
		pwd := enc.Encrypt(model.C.General.SecretKey, source.Password)
		if pwd == "" {
			return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_KEY_DECRYPTION_FAILED))
		}
		model.DB().Model(&model.CoreDataSource{}).Where("source_id =?", source.SourceId).Updates(&model.CoreDataSource{Password: pwd})
	}
	model.DB().Model(&model.CoreDataSource{}).Where("source_id =?", source.SourceId).Updates(map[string]interface{}{
		"id_c":               source.IDC,
		"ip":                 source.IP,
		"port":               source.Port,
		"source":             source.Source,
		"username":           source.Username,
		"is_query":           source.IsQuery,
		"flow_id":            source.FlowID,
		"exclude_db_list":    source.ExcludeDbList,
		"principal":          source.Principal,
		"insulate_word_list": source.InsulateWordList,
		"ca_file":            source.CAFile,
		"cert":               source.Cert,
		"key_file":           source.KeyFile,
		"rule_id":            source.RuleId,
	})
	model.DB().Model(model.CoreQueryOrder{}).Where("status =? and source_id =?", 1, source.SourceId).Updates(&model.CoreQueryOrder{Assigned: source.Principal})
	var k []model.CoreRoleGroup
	model.DB().Find(&k)
	for i := range k {
		var p model.PermissionList
		if err := json.Unmarshal(k[i].Permissions, &p); err != nil {
			return common.ERR_COMMON_MESSAGE(err)
		}
		if source.IsQuery == 0 {
			p.QuerySource = factory.RemoveString(p.QuerySource, source.Source)
		}
		if source.IsQuery == 1 {
			p.DDLSource = factory.RemoveString(p.DDLSource, source.Source)
			p.DMLSource = factory.RemoveString(p.DMLSource, source.Source)
		}
		r, _ := json.Marshal(p)
		model.DB().Model(&model.CoreRoleGroup{}).Where("id =?", k[i].ID).Updates(model.CoreRoleGroup{Permissions: r})
	}
	return common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.DB_EDIT_SUCCESS))
}

func SuperCreateSource(source *model.CoreDataSource) common.Resp {
	pwd := enc.Encrypt(model.C.General.SecretKey, source.Password)
	if pwd == "" {
		return common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_KEY_DECRYPTION_FAILED))
	}
	source.Password = pwd
	source.SourceId = uuid.New().String()
	model.DB().Create(source)
	return common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.DB_SAVE_SUCCESS))
}

func SuperTestDBConnect(source *model.CoreDataSource) common.Resp {
	if err := ConnTest(source); err != nil {
		return common.ERR_COMMON_MESSAGE(err)
	}
	return common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.CONN_TEST_SUCCESS))
}
