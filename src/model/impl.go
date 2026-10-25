package model

import (
	"Yearning-go/src/lib/enc"
	"errors"
	"gorm.io/gorm"
)

func (s *CoreDataSource) ConnectDB(schema string) (*gorm.DB, error) {
	// 与 fetch.go/exec 路径同理:空口令数据源(无认证 Mongo、默认 root 的 Doris)
	// 解密出的空串是合法明文;只有存储了密文却解不出才算失败。
	ps := ""
	if s.Password != "" {
		ps = enc.Decrypt(C.General.SecretKey, s.Password)
		if ps == "" {
			return nil, errors.New("连接失败,数据源口令解密失败！")
		}
	}

	return NewDBSub(DSN{
		Username: s.Username,
		Password: ps,
		Host:     s.IP,
		Port:     s.Port,
		DBName:   schema,
		CA:       s.CAFile,
		Cert:     s.Cert,
		Key:      s.KeyFile,
		DBType:   s.DBType,
	})
}
