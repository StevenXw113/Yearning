// Copyright 2019 HenryYee.
//
// Licensed under the AGPL, Version 3.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    https://www.gnu.org/licenses/agpl-3.0.en.html
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package model

import (
	"Yearning-go/src/i18n"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/cookieY/yee/logger"
	mmsql "github.com/go-sql-driver/mysql"
	drive "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"os"
	"time"
)

var sqlDB *gorm.DB

type DSN struct {
	Username string
	Password string
	Host     string
	Port     int
	DBName   string
	CA       string
	Cert     string
	Key      string
}

// defaultSecretKeys 是模板中随包分发的示例密钥，使用它们等同于把签名密钥公开
var defaultSecretKeys = map[string]struct{}{
	"dbcjqheupqjsuwsm":                     {},
	"CHANGE_ME_PLEASE_USE_RANDOM_32_CHARS": {},
}

func initConfig(cPath string) {
	_, err := toml.DecodeFile(cPath, &C)
	if err != nil {
		logger.DefaultLogger.Error(err)
	}
	var jwt = os.Getenv("SECRET_KEY")
	var lang = os.Getenv("Y_LANG")
	if jwt != "" {
		C.General.SecretKey = jwt
	}
	if lang != "" {
		C.General.Lang = lang
	}
	// 主密钥同时用于签发令牌和加密数据源口令，强度不足或沿用示例值必须拒绝启动
	if _, ok := defaultSecretKeys[C.General.SecretKey]; ok || len(C.General.SecretKey) < 16 {
		logger.DefaultLogger.Error("SecretKey 强度不足或仍是模板示例值，请使用随机字符串（建议 32 字符以上）配置 [General].SecretKey 或环境变量 SECRET_KEY")
		os.Exit(1)
	}
	i18n.MakeBuild(C.General.Lang)
	DefaultLogger = logger.LogCreator(int(TransferLogLevel()))
}

func DBNew(cPath string) {
	initConfig(cPath)
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", os.Getenv("MYSQL_USER"), os.Getenv("MYSQL_PASSWORD"), os.Getenv("MYSQL_ADDR"), os.Getenv("MYSQL_DB"))
	if os.Getenv("MYSQL_USER") == "" {
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", C.Mysql.User, C.Mysql.Password, C.Mysql.Host, C.Mysql.Port, C.Mysql.Db)
	}
	db, err := gorm.Open(drive.New(drive.Config{
		DSN:                       dsn,
		DefaultStringSize:         256,   // string 类型字段的默认长度
		SkipInitializeWithVersion: false, // 根据当前 MySQL 版本自动配置
	}), &gorm.Config{AllowGlobalUpdate: false}) // 禁止无 WHERE 的全表更新/删除
	if err != nil {
		logger.DefaultLogger.Error(i18n.DefaultLang.Load(i18n.ER_MYSQL_CONNECTION_FAILED))
		os.Exit(1)
		return
	}
	sqlDB = db
	conf, err := db.DB()
	if err != nil {
		logger.DefaultLogger.Error(err)
		return
	}
	conf.SetConnMaxLifetime(time.Minute * 10)
	conf.SetMaxOpenConns(50)
	conf.SetMaxIdleConns(15)
}

func DB() *gorm.DB {
	return sqlDB
}

func NewDBSub(dsn DSN) (*gorm.DB, error) {
	d, err := InitDSN(dsn)
	if err != nil {
		return nil, err
	}
	db, err := gorm.Open(drive.New(drive.Config{
		DSN:                       d,
		DefaultStringSize:         256,   // string 类型字段的默认长度
		SkipInitializeWithVersion: false, // 根据当前 MySQL 版本自动配置
	}), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	return db, nil
}

func Close(db *gorm.DB) error {
	orm, err := db.DB()
	if err != nil {
		return err
	}
	return orm.Close()
}

func InitDSN(dsn DSN) (string, error) {
	var tlsName string
	if dsn.CA != "" && dsn.Cert != "" && dsn.Key != "" {
		certPool := x509.NewCertPool()
		if ok := certPool.AppendCertsFromPEM([]byte(dsn.CA)); !ok {
			return "", fmt.Errorf("failed to append ca certs")
		}
		certs, err := tls.X509KeyPair([]byte(dsn.Cert), []byte(dsn.Key))
		if err != nil {
			return "", err
		}
		// 不关闭证书校验：否则管理员配置的 CA 形同虚设，
		// 中间人可伪装成数据库骗取数据源明文口令与全部 SQL 内容。
		// 配置名按证书内容派生，避免多个数据源共用同一个名字互相覆盖。
		sum := sha256.Sum256([]byte(dsn.CA + dsn.Cert + dsn.Key))
		tlsName = "yearning-" + hex.EncodeToString(sum[:8])
		if err := mmsql.RegisterTLSConfig(tlsName, &tls.Config{
			RootCAs:      certPool,
			Certificates: []tls.Certificate{certs},
		}); err != nil {
			return "", err
		}
	}
	cfg := mmsql.Config{
		User:                 dsn.Username,
		Passwd:               dsn.Password,
		Addr:                 fmt.Sprintf("%s:%d", dsn.Host, dsn.Port), //IP:PORT
		Net:                  "tcp",
		DBName:               dsn.DBName,
		Loc:                  time.Local,
		AllowNativePasswords: true,
		ParseTime:            true,
	}
	if tlsName != "" {
		cfg.TLSConfig = tlsName
	}
	return cfg.FormatDSN(), nil
}
