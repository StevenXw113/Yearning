// Package totp 实现 RFC 6238 TOTP（时间一次性密码，Google Authenticator 等
// 认证器 App 的标准协议）。只用标准库：HMAC-SHA1 + 动态截断 + Base32。
// 不引第三方依赖——算法本身 50 行，引库反而多一个供应链面。
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	// Step 时间步长（秒），与认证器 App 的默认周期一致
	Step = 30
	// Digits 校验码位数
	Digits = 6
	// verifyWindow 允许的时钟偏差窗口：±1 个步长（覆盖手机/服务器各慢快 30s）
	verifyWindow = 1
)

// GenerateSecret 生成 160bit 随机密钥。返回原始字节（用于 HMAC 计算）与
// Base32 无填充编码（用于展示/录入认证器）。
func GenerateSecret() (raw []byte, base32Secret string, err error) {
	raw = make([]byte, 20)
	if _, err = rand.Read(raw); err != nil {
		return nil, "", err
	}
	return raw, EncodeSecret(raw), nil
}

// EncodeSecret 原始密钥 → Base32 无填充编码（认证器通用格式）
func EncodeSecret(raw []byte) string {
	return strings.TrimRight(base32.StdEncoding.EncodeToString(raw), "=")
}

// DecodeSecret Base32 编码（可含填充/空格/小写）→ 原始密钥
func DecodeSecret(s string) ([]byte, error) {
	s = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	s = strings.TrimRight(s, "=")
	// 长度非 8 的倍数时补齐（部分工具省略填充）
	if m := len(s) % 8; m != 0 {
		s += strings.Repeat("=", 8-m)
	}
	return base32.StdEncoding.DecodeString(s)
}

// codeAt 计算指定计数器的 HOTP（RFC 4226 动态截断）
func codeAt(secret []byte, counter uint64, digits int) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, secret)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	bin := (uint32(sum[off])&0x7f)<<24 |
		uint32(sum[off+1])<<16 |
		uint32(sum[off+2])<<8 |
		uint32(sum[off+3])
	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, bin%mod)
}

// Code 返回 t 时刻的校验码（6 位）
func Code(secret []byte, t time.Time) string {
	return codeAt(secret, uint64(t.Unix()/Step), Digits)
}

// Verify 校验 code：当前时刻 ±1 个步长内匹配即通过（时钟偏差容忍）。
// lastStep 为该账号上次成功使用的计数器（防重放）：只接受严格更新的步长，
// 同一个码在有效期内不能被用第二次。返回命中的步长供调用方持久化。
// 比较用 subtle.ConstantTimeCompare，不泄漏匹配位置。
func Verify(secret []byte, code string, t time.Time, lastStep int64) (ok bool, step int64) {
	code = strings.TrimSpace(code)
	if len(code) != Digits {
		return false, 0
	}
	now := t.Unix() / Step
	for i := -verifyWindow; i <= verifyWindow; i++ {
		s := now + int64(i)
		if s <= lastStep {
			continue // 已用过的步长（含更早的窗口）一律拒绝，防重放
		}
		want := codeAt(secret, uint64(s), Digits)
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return true, s
		}
	}
	return false, 0
}

// URI 生成认证器扫码用的 otpauth:// 地址（Google Authenticator 等通用格式）
func URI(issuer, account, base32Secret string) string {
	label := url.PathEscape(issuer + ":" + account)
	return fmt.Sprintf("otpauth://totp/%s?secret=%s&issuer=%s&algorithm=SHA1&digits=%d&period=%d",
		label, base32Secret, url.QueryEscape(issuer), Digits, Step)
}

// ParseDigits 仅供测试：按指定位数计算（RFC 6238 附录向量是 8 位）
func CodeDigits(secret []byte, t time.Time, digits int) string {
	return codeAt(secret, uint64(t.Unix()/Step), digits)
}
