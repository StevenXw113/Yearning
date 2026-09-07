package ad

import (
	"Yearning-go/src/model"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cookieY/yee/logger"
	"github.com/go-ldap/ldap/v3"
	"net"
)

type ALdap struct {
	model.Ldap
	ldapMap
}

type ldapMap struct {
	RealName   string `json:"real_name"`
	Email      string `json:"email"`
	Department string `json:"department"`
}

func (l *ALdap) LdapConnect(user string, pass string, isTest bool) (isOk bool, err error) {
	var ld *ldap.Conn
	if l.Ldaps {
		// 校验服务器证书与主机名：跳过校验会让域账号明文口令暴露给中间人
		host := l.Url
		if h, _, serr := net.SplitHostPort(l.Url); serr == nil {
			host = h
		}
		ld, err = ldap.DialTLS("tcp", l.Url, &tls.Config{ServerName: host})
	} else {
		ld, err = ldap.Dial("tcp", l.Url)
	}

	if err != nil {
		return false, err
	}

	defer ld.Close()

	if ld != nil {
		if err := ld.Bind(l.User, l.Password); err != nil {
			return false, err
		}
	}

	if isTest {
		user = l.TestUser
		pass = l.TestPassword
	}

	searchRequest := ldap.NewSearchRequest(
		l.Sc,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf(l.Type, user),
		[]string{},
		nil,
	)

	sr, err := ld.Search(searchRequest)

	if err != nil {
		return false, err
	}

	if len(sr.Entries) != 1 {
		return false, errors.New("user does not exist or too many entries returned")
	}

	userdn := sr.Entries[0].DN
	if err := ld.Bind(userdn, pass); err != nil {
		return false, err
	}
	var lmap ldapMap
	if err := json.Unmarshal([]byte(l.Map), &lmap); err != nil {
		logger.DefaultLogger.Error(err)
	} else {
		l.Email = sr.Entries[0].GetAttributeValue(lmap.Email)
		l.Department = sr.Entries[0].GetAttributeValue(lmap.Department)
		l.RealName = sr.Entries[0].GetAttributeValue(lmap.RealName)
	}

	return true, nil
}
