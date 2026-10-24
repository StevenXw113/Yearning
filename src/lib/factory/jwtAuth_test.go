package factory

import (
	"testing"
	"time"

	"Yearning-go/src/model"

	jwt "github.com/golang-jwt/jwt/v5"
)

// TestJwtAuthSignParseRoundTrip v5 签发 → WsTokenParse(v5) 回读：
// claims 内容、过期拒绝、篡改拒绝、非 HMAC 算法拒绝。
// yee 的 v3 中间件解析同格式令牌属于线上兼容点（JWT 是线上标准，HS256 产物跨版本一致），
// 此处只验自有闭环。
func TestJwtAuthSignParseRoundTrip(t *testing.T) {
	tok, err := JwtAuth(Token{Username: "alice", RealName: "A", IsRecord: true})
	if err != nil || tok == "" {
		t.Fatalf("签发失败: %v", err)
	}
	parsed, err := WsTokenParse(tok)
	if err != nil || parsed == nil || !parsed.Valid {
		t.Fatalf("解析失败: %v", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok || claims["name"] != "alice" || claims["is_record"] != true {
		t.Fatalf("claims 不符: %v", claims)
	}

	if _, err := WsTokenParse(tok + "x"); err == nil {
		t.Fatal("篡改令牌应被拒绝")
	}

	expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"name": "alice", "exp": time.Now().Add(-time.Hour).Unix(),
	}).SignedString(model.JwtSigningKey())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WsTokenParse(expired); err == nil {
		t.Fatal("过期令牌应被拒绝")
	}

	// alg 混淆：用 none 算法伪造（无签名段）
	noneAlg, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"name": "evil", "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WsTokenParse(noneAlg); err == nil {
		t.Fatal("alg=none 伪造应被拒绝")
	}
}
