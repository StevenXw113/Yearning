package fetch

import (
	"Yearning-go/src/handler/common"
	"Yearning-go/src/lib/factory"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/cookieY/yee"
)

// 一次性、短时效的 AI 聊天会话交换码。
// 目的：把已登录会话安全地交给（可能跨源的）内嵌聊天页，而无需把长期 JWT 放进 iframe URL
// （URL 会落入浏览器历史 / 访问日志 / Referer）。交换码随机、单次、短时效，泄露也无法重放成长期会话。
const chatCodeTTL = 60 * time.Second

var (
	chatCodeMu   sync.Mutex
	chatCodePool = make(map[string]*chatSession)
)

type chatSession struct {
	username string
	realName string
	isRecord bool
	expire   time.Time
}

func newChatCode(s *chatSession) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	code := hex.EncodeToString(b)
	chatCodeMu.Lock()
	defer chatCodeMu.Unlock()
	// 顺带清理过期项，防止 map 无限增长
	for k, v := range chatCodePool {
		if time.Until(v.expire) <= 0 {
			delete(chatCodePool, k)
		}
	}
	chatCodePool[code] = s
	return code, nil
}

func redeemChatCode(code string) *chatSession {
	if code == "" {
		return nil
	}
	chatCodeMu.Lock()
	defer chatCodeMu.Unlock()
	s, ok := chatCodePool[code]
	if !ok {
		return nil
	}
	delete(chatCodePool, code) // 单次使用
	if time.Until(s.expire) <= 0 {
		return nil
	}
	return s
}

// ChatSessionIssue 由已登录的主应用调用，签发一个单次/短时效交换码（绝不把长期 JWT 交给前端 URL）。
func ChatSessionIssue(c yee.Context) error {
	u := new(factory.Token).JwtParse(c)
	if u.Username == "" {
		return c.JSON(http.StatusUnauthorized, common.ERR_COMMON_TEXT_MESSAGE("unauthorized"))
	}
	code, err := newChatCode(&chatSession{
		username: u.Username,
		realName: u.RealName,
		isRecord: u.IsRecord,
		expire:   time.Now().Add(chatCodeTTL),
	})
	if err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	return c.JSON(http.StatusOK, common.SuccessPayload(map[string]interface{}{"code": code}))
}

// ChatSessionExchange 由聊天页启动时调用，用一次性交换码换一个全新的短期 JWT。
// 兑换后的 token 应仅存于聊天页内存中，用于对 /api/v2/chat 的鉴权请求头，不得再进入 URL。
func ChatSessionExchange(c yee.Context) error {
	s := redeemChatCode(c.FormValue("code"))
	if s == nil {
		return c.JSON(http.StatusUnauthorized, common.ERR_COMMON_TEXT_MESSAGE("invalid or expired code"))
	}
	token, err := factory.JwtAuth(factory.Token{Username: s.username, RealName: s.realName, IsRecord: s.isRecord})
	if err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	return c.JSON(http.StatusOK, common.SuccessPayload(map[string]interface{}{"token": token}))
}
