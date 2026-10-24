package login

import (
	"Yearning-go/src/handler/common"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/enc"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/lib/totp"
	"Yearning-go/src/model"
	"errors"
	"fmt"
	"github.com/cookieY/yee"
	"github.com/golang-jwt/jwt"
	"gorm.io/gorm"
	"net/http"
	"time"
)

// MFA（TOTP）二级认证。
//
// 登录两步流：密码正确且 MFA 启用时，第一步不签发正式 JWT，而是返回一张
// 5 分钟有效的 mfa ticket（claims 里只有 mfa_user，没有 name/is_record，
// 即使被拿去调 /api/v2 也过不了任何需要身份的接口）；
// 第二步带 ticket + 动态码调 /login/mfa 换取正式 JWT。
//
// OIDC 登录不走 MFA：二级认证由 IdP 侧负责（有 SSO 的部署不会同时用本地密码）。

const mfaTicketTTL = 5 * time.Minute

type mfaLoginForm struct {
	Ticket string `json:"ticket"`
	Code   string `json:"code"`
}

// mfaTicket 签发 MFA 中间票据：仅供第二步换票，不含任何权限声明
func mfaTicket(username string) (string, error) {
	t := jwt.New(jwt.SigningMethodHS256)
	claims := t.Claims.(jwt.MapClaims)
	claims["mfa_user"] = username
	claims["iat"] = time.Now().Unix()
	claims["exp"] = time.Now().Add(mfaTicketTTL).Unix()
	return t.SignedString(model.JwtSigningKey())
}

// mfaTicketParse 校验并解出 ticket 中的用户名。签名/过期/缺 claim 一律拒绝。
func mfaTicketParse(ticket string) (string, error) {
	token, err := jwt.Parse(ticket, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return model.JwtSigningKey(), nil
	})
	if err != nil || !token.Valid {
		return "", errors.New("mfa ticket 无效或已过期，请重新登录")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("mfa ticket 无效")
	}
	user, _ := claims["mfa_user"].(string)
	if user == "" {
		return "", errors.New("mfa ticket 无效")
	}
	return user, nil
}

// MfaLogin 登录第二步：ticket + 动态码 → 正式 JWT。与密码登录共用同一套
// IP+用户名维度的失败锁定（ratelimit.go），防止对 6 位码在线爆破。
func MfaLogin(c yee.Context) (err error) {
	u := new(mfaLoginForm)
	if err = c.Bind(u); err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	username, terr := mfaTicketParse(u.Ticket)
	if terr != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(terr.Error()))
	}
	key := clientKey(c, username)
	if isLocked(key) {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_LOGIN_TOO_MANY_ATTEMPTS)))
	}
	var account model.CoreAccount
	if errors.Is(model.DB().Where("username =?", username).First(&account).Error, gorm.ErrRecordNotFound) || account.MfaEnabled != 1 {
		// ticket 对应的账号已关闭 MFA（管理员重置等），直接拒绝，让其重新走第一步
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_MFA_NOT_ENABLED)))
	}
	secret := enc.Decrypt(model.C.General.SecretKey, account.MfaSecret)
	if secret == "" {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_KEY_DECRYPTION_FAILED)))
	}
	raw, derr := totp.DecodeSecret(secret)
	if derr != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_MFA_SECRET_INVALID)))
	}
	ok, step := totp.Verify(raw, u.Code, time.Now(), account.MfaLastStep)
	if !ok {
		recordFailure(key)
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_MFA_CODE_INVALID)))
	}
	resetAttempts(key)
	// 记录已用步长：同一个码在有效期内不能再次登录（防重放）
	model.DB().Model(model.CoreAccount{}).Where("username =?", username).
		Update("mfa_last_step", step)
	token, tokenErr := factory.JwtAuth(factory.Token{
		Username: account.Username,
		RealName: account.RealName,
		IsRecord: account.IsRecorder == 1,
	})
	if tokenErr != nil {
		c.Logger().Error(tokenErr.Error())
		return tokenErr
	}
	return c.JSON(http.StatusOK, common.SuccessPayload(map[string]interface{}{
		"token":     token,
		"real_name": account.RealName,
		"user":      account.Username,
		"is_record": account.IsRecorder,
	}))
}

// ---- 以下为已登录用户自助管理（/api/v2/mfa/:tp，JWT 保护）----

type mfaCodeForm struct {
	Code string `json:"code"`
}

// MfaSetup 生成（或重新生成）待绑定的密钥：写入 MfaSecret 但不启用，
// 返回 Base32 密钥与 otpauth:// URI 供前端渲染二维码/手动录入
func MfaSetup(c yee.Context) (err error) {
	user := new(factory.Token).JwtParse(c).Username
	if user == "" {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE)))
	}
	var account model.CoreAccount
	model.DB().Select("mfa_enabled").Where("username =?", user).First(&account)
	if account.MfaEnabled == 1 {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_MFA_ALREADY_ENABLED)))
	}
	_, b32, gerr := totp.GenerateSecret()
	if gerr != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(gerr))
	}
	model.DB().Model(model.CoreAccount{}).Where("username =?", user).Updates(map[string]interface{}{
		"mfa_secret":    enc.Encrypt(model.C.General.SecretKey, b32),
		"mfa_last_step": 0,
	})
	return c.JSON(http.StatusOK, common.SuccessPayload(map[string]interface{}{
		"secret": b32,
		"uri":    totp.URI("Yearning", user, b32),
	}))
}

// MfaEnable 用认证器返回的动态码确认绑定：验证通过才置 MfaEnabled=1
func MfaEnable(c yee.Context) (err error) {
	return mfaConfirm(c, true)
}

// MfaDisable 解绑：需提供当前有效动态码（防止拿到会话的人直接关掉二级认证）
func MfaDisable(c yee.Context) (err error) {
	return mfaConfirm(c, false)
}

func mfaConfirm(c yee.Context, enable bool) (err error) {
	user := new(factory.Token).JwtParse(c).Username
	if user == "" {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE)))
	}
	u := new(mfaCodeForm)
	if err = c.Bind(u); err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	var account model.CoreAccount
	model.DB().Select("mfa_secret, mfa_enabled, mfa_last_step").Where("username =?", user).First(&account)
	if account.MfaSecret == "" {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_MFA_SECRET_INVALID)))
	}
	if enable && account.MfaEnabled == 1 {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_MFA_ALREADY_ENABLED)))
	}
	if !enable && account.MfaEnabled != 1 {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_MFA_NOT_ENABLED)))
	}
	b32 := enc.Decrypt(model.C.General.SecretKey, account.MfaSecret)
	raw, derr := totp.DecodeSecret(b32)
	if derr != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_MFA_SECRET_INVALID)))
	}
	ok, step := totp.Verify(raw, u.Code, time.Now(), account.MfaLastStep)
	if !ok {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_MFA_CODE_INVALID)))
	}
	if enable {
		model.DB().Model(model.CoreAccount{}).Where("username =?", user).
			Updates(map[string]interface{}{"mfa_enabled": 1, "mfa_last_step": step})
		return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.INFO_MFA_ENABLED)))
	}
	model.DB().Model(model.CoreAccount{}).Where("username =?", user).
		Updates(map[string]interface{}{"mfa_enabled": 0, "mfa_secret": "", "mfa_last_step": 0})
	return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.INFO_MFA_DISABLED)))
}

// MfaStatus 当前登录账号的 MFA 状态（个人中心展示）
func MfaStatus(c yee.Context) (err error) {
	user := new(factory.Token).JwtParse(c).Username
	if user == "" {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE)))
	}
	var account model.CoreAccount
	model.DB().Select("mfa_enabled").Where("username =?", user).First(&account)
	return c.JSON(http.StatusOK, common.SuccessPayload(map[string]interface{}{"enabled": account.MfaEnabled == 1}))
}

func MfaGetApis(c yee.Context) (err error) {
	switch c.Params("tp") {
	case "setup":
		return MfaSetup(c)
	case "status":
		return MfaStatus(c)
	default:
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE)))
	}
}

func MfaPostApis(c yee.Context) (err error) {
	switch c.Params("tp") {
	case "enable":
		return MfaEnable(c)
	case "disable":
		return MfaDisable(c)
	default:
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE)))
	}
}

// SuperResetMFA 管理员重置用户 MFA（用户认证器丢失时的解锁出口）。
// 直接清空绑定，用户下次登录只需密码，之后可重新绑定。挂在 /manage 下，
// 由 SuperManageGroup 中间件保证仅超管可达。
func SuperResetMFA(c yee.Context) (err error) {
	u := new(struct {
		Username string `json:"username"`
	})
	if err = c.Bind(u); err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	if u.Username == "" {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE)))
	}
	res := model.DB().Model(model.CoreAccount{}).Where("username =?", u.Username).
		Updates(map[string]interface{}{"mfa_enabled": 0, "mfa_secret": "", "mfa_last_step": 0})
	if res.Error != nil || res.RowsAffected != 1 {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(fmt.Sprintf("用户 %s 不存在", u.Username)))
	}
	return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(fmt.Sprintf(i18n.DefaultLang.Load(i18n.INFO_MFA_RESET), u.Username)))
}

// MfaRestFulAPis 已登录用户的 MFA 自助管理（挂在 /api/v2/mfa/:tp，JWT 保护）
func MfaRestFulAPis() yee.RestfulAPI {
	return yee.RestfulAPI{
		Get:  MfaGetApis,
		Post: MfaPostApis,
	}
}
