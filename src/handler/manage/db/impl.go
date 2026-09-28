package db

import (
	"Yearning-go/src/handler/common"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/enc"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/model"
	"context"
	"encoding/json"
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
	// MongoDB 数据源用 mongo 驱动探活，不经过 gorm / mysql 方言。
	// 口令在这里已经是明文（调用方解过密文），别再走会解密的 ConnectMongo
	if u.DBType == model.DBTypeMongoDB {
		client, err := u.ConnectMongoWith(u.Password)
		if err != nil {
			return err
		}
		return client.Disconnect(context.Background())
	}
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
		// 权限列表里存的是 source_id（不是展示名），用 source.Source 会永不相等、删除变空操作
		if source.IsQuery == 0 {
			p.QuerySource = factory.RemoveString(p.QuerySource, source.SourceId)
		}
		if source.IsQuery == 1 {
			p.DDLSource = factory.RemoveString(p.DDLSource, source.SourceId)
			p.DMLSource = factory.RemoveString(p.DMLSource, source.SourceId)
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
	// 4 位短 ID（查重分配）：source_id 会在工单、权限配置里被人眼看，UUID 太长
	source.SourceId = factory.NextSourceId()
	model.DB().Create(source)
	// 新建数据源默认授权给所有权限组：数据源是团队资源，默认可见、再按需收紧，
	// 比「默认谁都看不到、要逐个组去勾」更符合直觉，也避免漏授权导致「看不到数据源」。
	// （曾出现过：新加的 MongoDB 源只有 admin 可见，因为超管走特判、其余组没勾。）
	grantSourceToAllGroups(source.SourceId, int(source.IsQuery))
	return common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.DB_SAVE_SUCCESS))
}

func SuperTestDBConnect(source *model.CoreDataSource) common.Resp {
	if err := ConnTest(source); err != nil {
		return common.ERR_COMMON_MESSAGE(err)
	}
	return common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.CONN_TEST_SUCCESS))
}

// grantSourceToAllGroups 把新数据源按读写属性追加进所有已有权限组。
// IsQuery 语义（与 SuperEditSource 的清理逻辑一致）：0 = 可变更（ddl/dml）、
// 1 = 只读（query）、2 = 读写（三者都给）。组里已存在则跳过，不重复添加。
func grantSourceToAllGroups(sourceId string, isQuery int) {
	if sourceId == "" {
		return
	}
	var groups []model.CoreRoleGroup
	model.DB().Find(&groups)
	for i := range groups {
		var p model.PermissionList
		if err := json.Unmarshal(groups[i].Permissions, &p); err != nil {
			// 单个组的权限 JSON 坏掉不该影响数据源创建，跳过并留痕
			model.DefaultLogger.Errorf("解析权限组 %s 失败，跳过授权: %v", groups[i].Name, err)
			continue
		}
		if isQuery != 1 {
			p.DDLSource = appendIfMissing(p.DDLSource, sourceId)
			p.DMLSource = appendIfMissing(p.DMLSource, sourceId)
		}
		if isQuery != 0 {
			p.QuerySource = appendIfMissing(p.QuerySource, sourceId)
		}
		r, _ := json.Marshal(p)
		model.DB().Model(&model.CoreRoleGroup{}).Where("id =?", groups[i].ID).Updates(model.CoreRoleGroup{Permissions: r})
	}
}

// appendIfMissing 追加且去重
func appendIfMissing(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}
