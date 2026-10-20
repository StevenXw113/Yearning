package model

import (
	"Yearning-go/src/lib/enc"
	"errors"
	"gorm.io/gorm"
)

func (s *CoreDataSource) ConnectDB(schema string) (*gorm.DB, error) {
	ps := enc.Decrypt(C.General.SecretKey, s.Password)
	if ps == "" {
		return nil, errors.New("连接失败,密码解析错误！")
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
