package factory

import (
	"Yearning-go/src/model"
	crand "crypto/rand"
	"crypto/pbkdf2"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
)

// ArrayRemove 从 JSON 字符串数组里删掉所有等于 flag 的元素（用于 core_grained.group 这类顶层数组）
func ArrayRemove(source []byte, flag string) ([]byte, error) {
	var items []string
	if err := json.Unmarshal(source, &items); err != nil {
		return nil, err
	}
	return json.Marshal(dropValue(items, flag))
}

// MultiArrayRemove 对 JSON 对象里的多个字符串数组分别删掉等于 flag 的元素
// （用于 core_role_groups.permissions 的 ddl_source / dml_source / query_source）
func MultiArrayRemove(source []byte, sep []string, flag string) ([]byte, error) {
	var doc map[string][]string
	if err := json.Unmarshal(source, &doc); err != nil {
		return nil, err
	}
	for _, key := range sep {
		doc[key] = dropValue(doc[key], flag)
	}
	return json.Marshal(doc)
}

// dropValue 过滤掉等于 flag 的元素；结果为空时返回空数组而非 nil，避免序列化成 null
func dropValue(items []string, flag string) []string {
	out := make([]string, 0, len(items))
	for _, v := range items {
		if v != flag {
			out = append(out, v)
		}
	}
	return out
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
	dk, err := pbkdf2.Key(sha256.New, password, []byte(sl), iterations, 32)
	if err != nil {
		// 迭代次数与密钥长度都由代码给定，出错只能是参数 bug——不能静默落一个空密码
		panic("pbkdf2: " + err.Error())
	}
	str := base64.StdEncoding.EncodeToString(dk)
	return "pbkdf2_sha256" + "$" + strconv.FormatInt(int64(iterations), 10) + "$" + sl + "$" + str
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
