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

package enc

import (
	"Yearning-go/src/i18n"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"strings"

	"github.com/cookieY/yee/logger"
	"golang.org/x/crypto/hkdf"
)

// gcmPrefix 标识 AES-256-GCM 密文，格式为 v1$base64(nonce||ciphertext)。
// 不带该前缀的密文为历史 AES-CBC 格式，仍需可解密，以保证存量数据平滑迁移。
const gcmPrefix = "v1$"

const dbPasswordInfo = "yearning-db-password-v1"

// DeriveKey 使用 HKDF-SHA256 从主密钥派生出指定长度的子密钥。
// 不同用途应传入不同的 info，避免一处泄露导致全部用途失守。
func DeriveKey(master, info string, length int) []byte {
	k := make([]byte, length)
	r := hkdf.New(sha256.New, []byte(master), nil, []byte(info))
	if _, err := io.ReadFull(r, k); err != nil {
		return nil
	}
	return k
}

// Encrypt 使用主密钥派生的 32 字节密钥做 AES-256-GCM 加密。
// 每次加密使用随机 nonce，因此相同明文不会产生相同密文。
func Encrypt(s, p string) string {
	key := DeriveKey(s, dbPasswordInfo, 32)
	if key == nil {
		logger.DefaultLogger.Error(i18n.DefaultLang.Load(i18n.ER_KEY_DECRYPTION_FAILED))
		return ""
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		logger.DefaultLogger.Error(i18n.DefaultLang.Load(i18n.ER_KEY_DECRYPTION_FAILED))
		return ""
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		logger.DefaultLogger.Error(i18n.DefaultLang.Load(i18n.ER_KEY_DECRYPTION_FAILED))
		return ""
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		logger.DefaultLogger.Error(i18n.DefaultLang.Load(i18n.ER_KEY_DECRYPTION_FAILED))
		return ""
	}
	// Seal 会把 nonce 作为密文前缀写入，解密时按同样长度切分
	return gcmPrefix + base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(p), nil))
}

// Decrypt 自动识别密文格式：新版走 AES-256-GCM，旧版回退到 AES-CBC。
func Decrypt(s, cryted string) string {
	if cryted == "" {
		return ""
	}
	if strings.HasPrefix(cryted, gcmPrefix) {
		return gcmDecrypt(s, strings.TrimPrefix(cryted, gcmPrefix))
	}
	return cbcDecrypt(s, cryted)
}

func gcmDecrypt(s, cryted string) string {
	key := DeriveKey(s, dbPasswordInfo, 32)
	if key == nil {
		logger.DefaultLogger.Error(i18n.DefaultLang.Load(i18n.ER_KEY_DECRYPTION_FAILED))
		return ""
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		logger.DefaultLogger.Error(i18n.DefaultLang.Load(i18n.ER_KEY_DECRYPTION_FAILED))
		return ""
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		logger.DefaultLogger.Error(i18n.DefaultLang.Load(i18n.ER_KEY_DECRYPTION_FAILED))
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(cryted)
	if err != nil {
		logger.DefaultLogger.Error(i18n.DefaultLang.Load(i18n.ER_KEY_DECRYPTION_FAILED))
		return ""
	}
	if len(raw) < gcm.NonceSize() {
		logger.DefaultLogger.Error(i18n.DefaultLang.Load(i18n.ER_KEY_DECRYPTION_FAILED))
		return ""
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		logger.DefaultLogger.Error(i18n.DefaultLang.Load(i18n.ER_KEY_DECRYPTION_FAILED))
		return ""
	}
	return string(plain)
}

// cbcDecrypt 兼容历史数据：AES-CBC，IV 固定为密钥前 16 字节。
// 该模式不具备语义安全也不校验完整性，仅用于解密存量密文，不用于写入。
func cbcDecrypt(s, cryted string) string {
	crytedByte, err := base64.StdEncoding.DecodeString(cryted)
	if err != nil {
		logger.DefaultLogger.Error(i18n.DefaultLang.Load(i18n.ER_KEY_DECRYPTION_FAILED))
		return ""
	}
	k := []byte(s)
	if len(k) != 16 && len(k) != 24 && len(k) != 32 {
		logger.DefaultLogger.Error(i18n.DefaultLang.Load(i18n.ER_KEY_DECRYPTION_FAILED))
		return ""
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		logger.DefaultLogger.Error(i18n.DefaultLang.Load(i18n.ER_KEY_DECRYPTION_FAILED))
		return ""
	}
	blockSize := block.BlockSize()
	if len(crytedByte) == 0 || len(crytedByte)%blockSize != 0 {
		logger.DefaultLogger.Error(i18n.DefaultLang.Load(i18n.ER_KEY_DECRYPTION_FAILED))
		return ""
	}
	orig := make([]byte, len(crytedByte))
	cipher.NewCBCDecrypter(block, k[:blockSize]).CryptBlocks(orig, crytedByte)
	orig = PKCS7UnPadding(orig)
	if orig == nil {
		logger.DefaultLogger.Error(i18n.DefaultLang.Load(i18n.ER_KEY_DECRYPTION_FAILED))
		return ""
	}
	return string(orig)
}

// 补码
func PKCS7Padding(ciphertext []byte, blocksize int) []byte {
	padding := blocksize - len(ciphertext)%blocksize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}

// 去码
func PKCS7UnPadding(origData []byte) []byte {
	if origData == nil {
		return nil
	}
	if len(origData) > 0 {
		length := len(origData)
		unpadding := int(origData[length-1])
		if (length - unpadding) < 0 {
			return nil
		}
		return origData[:(length - unpadding)]
	}
	return nil
}
