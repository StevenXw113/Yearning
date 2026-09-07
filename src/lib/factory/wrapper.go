package factory

import (
	"Yearning-go/src/model"
	crand "crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"github.com/Jeffail/gabs/v2"
	"golang.org/x/crypto/pbkdf2"
	"strconv"
	"strings"
)

func ArrayRemove(source []byte, flag string) ([]byte, error) {
	p, err := gabs.ParseJSON(source)
	if err != nil {
		return nil, err
	}
	for i, c := range p.Children() {
		if v, ok := c.Data().(string); ok && v == flag {
			_ = p.ArrayRemove(i)
		}
	}
	return p.EncodeJSON(), nil
}

func MultiArrayRemove(source []byte, sep []string, flag string) ([]byte, error) {
	p, err := gabs.ParseJSON(source)
	if err != nil {
		return nil, err
	}
	// gabs 容器非并发安全，必须串行处理，否则会触发 concurrent map writes 导致进程退出
	for _, dl := range sep {
		var indicesToRemove []int
		for i, c := range p.S(dl).Children() {
			if v, ok := c.Data().(string); ok && v == flag {
				indicesToRemove = append(indicesToRemove, i)
			}
		}
		// 从后向前删除，避免索引错位
		for i := len(indicesToRemove) - 1; i >= 0; i-- {
			p.ArrayRemove(indicesToRemove[i], dl)
		}
	}
	return p.EncodeJSON(), nil
}

// GetRandom 生成密码学安全的随机盐值。
// 原先使用 math/rand + 时间种子，salt 可被预测，拖库后可预先构造彩虹表。
func GetRandom() []byte {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, 16)
	if _, err := crand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	out := make([]byte, len(b))
	for i, v := range b {
		out[i] = alphabet[int(v)%len(alphabet)]
	}
	return out
}

// defaultIterations 新建密码使用的 PBKDF2 迭代次数。
// 旧密码按其自身记录的迭代次数校验，因此调整此值不会使存量密码失效。
const defaultIterations = 600000

func DjangoEncrypt(password string, sl string) string {
	return djangoEncrypt(password, sl, defaultIterations)
}

func djangoEncrypt(password string, sl string, iterations int) string {
	pwd := []byte(password)
	salt := []byte(sl)
	dk := pbkdf2.Key(pwd, salt, iterations, 32, sha256.New)
	str := base64.StdEncoding.EncodeToString(dk)
	return "pbkdf2_sha256" + "$" + strconv.FormatInt(int64(iterations), 10) + "$" + string(salt) + "$" + str
}

func DjangoCheckPassword(account *model.CoreAccount, password string) bool {
	// 密码格式异常时直接判定失败，避免索引越界 panic
	parts := strings.Split(account.Password, "$")
	if len(parts) != 4 {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations <= 0 {
		return false
	}
	checkPasswordToken := djangoEncrypt(password, parts[2], iterations)
	// 恒定时间比较，避免通过响应耗时侧信道逐字节推断摘要
	return subtle.ConstantTimeCompare([]byte(account.Password), []byte(checkPasswordToken)) == 1
}
