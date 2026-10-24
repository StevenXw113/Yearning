package factory

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cookieY/yee"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

const testSigningKey = "dbcjqheupqjsuwsm"

func genTestToken(t *testing.T, method jwt.SigningMethod, ttl time.Duration) string {
	t.Helper()
	token := jwt.New(method)
	claims := token.Claims.(jwt.MapClaims)
	claims["name"] = "henry"
	claims["exp"] = time.Now().Add(ttl).Unix()
	s, err := token.SignedString([]byte(testSigningKey))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// runMiddleware 跑中间件链,返回响应码与业务侧读到的 name(验证 claims 注入)。
func runMiddleware(t *testing.T, header, value string) (int, string) {
	t.Helper()
	y := yee.New()
	var gotName string
	y.GET("/", JwtMiddleware([]byte(testSigningKey)), func(c yee.Context) error {
		user := c.Get(authContextKey).(*jwt.Token)
		claims := user.Claims.(jwt.MapClaims)
		gotName, _ = claims["name"].(string)
		return c.String(http.StatusOK, "ok")
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if header != "" {
		req.Header.Set(header, value)
	}
	rec := httptest.NewRecorder()
	y.ServeHTTP(rec, req)
	return rec.Code, gotName
}

func TestJwtMiddlewareEndToEnd(t *testing.T) {
	const authz = yee.HeaderAuthorization
	const wsProto = yee.HeaderSecWebSocketProtocol

	t.Run("有效令牌到达业务并注入 claims", func(t *testing.T) {
		code, name := runMiddleware(t, authz, "Bearer "+genTestToken(t, jwt.SigningMethodHS256, time.Minute))
		assert.Equal(t, http.StatusOK, code)
		assert.Equal(t, "henry", name)
	})

	t.Run("无令牌 400(与 yee 版分档一致)", func(t *testing.T) {
		code, _ := runMiddleware(t, "", "")
		assert.Equal(t, http.StatusBadRequest, code)
	})

	t.Run("垃圾令牌 401", func(t *testing.T) {
		code, _ := runMiddleware(t, authz, "Bearer not.a.jwt")
		assert.Equal(t, http.StatusUnauthorized, code)
	})

	t.Run("过期令牌 401", func(t *testing.T) {
		code, _ := runMiddleware(t, authz, "Bearer "+genTestToken(t, jwt.SigningMethodHS256, -time.Minute))
		assert.Equal(t, http.StatusUnauthorized, code)
	})

	t.Run("非 HS256 签名算法拒绝(防 alg 混淆)", func(t *testing.T) {
		code, _ := runMiddleware(t, authz, "Bearer "+genTestToken(t, jwt.SigningMethodHS512, time.Minute))
		assert.Equal(t, http.StatusUnauthorized, code)
	})

	t.Run("ws 子协议头携带裸令牌可用", func(t *testing.T) {
		code, _ := runMiddleware(t, wsProto, genTestToken(t, jwt.SigningMethodHS256, time.Minute))
		assert.Equal(t, http.StatusOK, code)
	})

	t.Run("Bearer 前缀区分大小写(与 yee 版一致,小写视为缺失)", func(t *testing.T) {
		code, _ := runMiddleware(t, authz, fmt.Sprintf("bearer %s", genTestToken(t, jwt.SigningMethodHS256, time.Minute)))
		assert.Equal(t, http.StatusBadRequest, code)
	})
}
