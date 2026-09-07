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

package login

import (
	"Yearning-go/src/handler/common"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/model"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cookieY/yee"
	"gorm.io/gorm"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	oidcStateTTL    = 5 * time.Minute
	oidcHTTPTimeout = 10 * time.Second
)

// OIDC 的 state 用于防止登录 CSRF：必须是随机值、一次性、短时效。
// 原先使用硬编码常量且回调从不校验，攻击者可诱导受害者完成登录。
var (
	stateMu    sync.Mutex
	oidcStates = make(map[string]time.Time)
)

func newOidcState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	s := hex.EncodeToString(b)
	stateMu.Lock()
	defer stateMu.Unlock()
	for k, v := range oidcStates {
		if time.Since(v) > oidcStateTTL {
			delete(oidcStates, k)
		}
	}
	oidcStates[s] = time.Now()
	return s, nil
}

func verifyOidcState(s string) bool {
	if s == "" {
		return false
	}
	stateMu.Lock()
	defer stateMu.Unlock()
	created, ok := oidcStates[s]
	if !ok {
		return false
	}
	delete(oidcStates, s)
	return time.Since(created) <= oidcStateTTL
}

func oidcAuthURL(state string) string {
	return fmt.Sprintf(
		"%s?response_type=code&client_id=%s&redirect_uri=%s&scope=%s&state=%s",
		model.C.Oidc.AuthUrl,
		model.C.Oidc.ClientId,
		url.QueryEscape(model.C.Oidc.RedirectUrL),
		url.QueryEscape(model.C.Oidc.Scope),
		url.QueryEscape(state))
}

func OidcState(c yee.Context) (err error) {
	if !model.C.Oidc.Enable {
		return c.JSON(http.StatusOK, common.SuccessPayload(map[string]interface{}{
			"enabled": false,
		}))
	}
	state, err := newOidcState()
	if err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	return c.JSON(http.StatusOK, common.SuccessPayload(map[string]interface{}{
		"authUrl": oidcAuthURL(state),
		"enabled": true,
	}))
}

func OidcLogin(c yee.Context) (err error) {
	if !model.C.Oidc.Enable {
		return c.HTML(400, i18n.DefaultLang.Load(i18n.INFO_OIDC_LOGIN_DISABLED))
	}

	code := c.FormValue("code")
	if code == "" {
		state, serr := newOidcState()
		if serr != nil {
			return c.HTML(500, serr.Error())
		}
		return c.Redirect(302, oidcAuthURL(state))
	}

	// state 不匹配时拒绝：否则任意 code 都能被兑换成一次登录
	if !verifyOidcState(c.FormValue("state")) {
		return c.HTML(403, i18n.DefaultLang.Load(i18n.ER_REQ_FAKE))
	}

	account, err := getAccount(code)
	if err != nil || account == nil {
		c.Logger().Error(err)
		return c.HTML(401, i18n.DefaultLang.Load(i18n.ER_LOGIN))
	}

	token, tokenErr := factory.JwtAuth(factory.Token{
		Username: account.Username,
		RealName: account.RealName,
		IsRecord: account.IsRecorder == 1,
	})
	if tokenErr != nil {
		c.Logger().Error(tokenErr.Error())
		return
	}
	// 令牌置于 URL 片段（# 之后）中，浏览器不会将其发送到服务端，
	// 因此不会出现在 Referer 与反向代理访问日志里。
	return c.Redirect(302, fmt.Sprintf(
		"/#/login?oidcLogin=1&token=%s&user=%s&real_name=%s&is_record=%d",
		token, url.QueryEscape(account.Username), url.QueryEscape(account.RealName), account.IsRecorder),
	)
}

func getAccount(code string) (ac *model.CoreAccount, err error) {
	oidcToken, err := getOidcToken(code)
	if err != nil {
		return nil, err
	}
	userMap, err := getOidcUser(oidcToken)
	if err != nil {
		return nil, err
	}
	// IdP 返回的字段可能缺失或不是字符串，不能做裸类型断言
	username, ok := userMap[model.C.Oidc.UserNameKey].(string)
	if !ok || username == "" {
		return nil, errors.New("oidc userinfo missing username")
	}
	realname, _ := userMap[model.C.Oidc.RealNameKey].(string)
	email, _ := userMap[model.C.Oidc.EmailKey].(string)

	account := new(model.CoreAccount)
	if err := model.DB().Where("username = ?", username).First(&account).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		coreAccount := model.CoreAccount{
			Username:   username,
			RealName:   realname,
			Password:   factory.DjangoEncrypt(factory.GenWorkId(), string(factory.GetRandom())),
			Department: "",
			Email:      email,
			IsRecorder: 2,
		}
		model.DB().Create(&coreAccount)
		ix, _ := json.Marshal([]string{})
		model.DB().Create(&model.CoreGrained{Username: username, Group: ix})
	}
	model.DB().Where("username = ?", username).First(&account)

	return account, nil
}

func getOidcUser(token *OidcToken) (userMap map[string]interface{}, err error) {
	bearer := "Bearer " + token.AccessToken
	request, err := http.NewRequest(http.MethodGet, model.C.Oidc.UserUrl, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Add("Authorization", bearer)

	client := &http.Client{Timeout: oidcHTTPTimeout}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(response.Body)

	userMap = make(map[string]interface{})
	if err := json.NewDecoder(response.Body).Decode(&userMap); err != nil {
		return nil, err
	}
	return userMap, nil
}

func getOidcToken(code string) (oidc_token *OidcToken, err error) {
	form := url.Values{
		"code":          {code},
		"client_id":     {model.C.Oidc.ClientId},
		"client_secret": {model.C.Oidc.ClientSecret},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {model.C.Oidc.RedirectUrL},
	}
	req, err := http.NewRequest(http.MethodPost, model.C.Oidc.TokenUrl, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: oidcHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	token := new(OidcToken)
	if err := json.NewDecoder(resp.Body).Decode(token); err != nil {
		return nil, err
	}
	return token, nil
}

type OidcToken struct {
	Scope       string `json:"scope"`
	AccessToken string `json:"access_token"`
}
