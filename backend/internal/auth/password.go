// Package auth 提供登录密码哈希与校验能力。
package auth

import (
	"crypto/md5"
	"encoding/hex"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// IsBcrypt 判断存储的哈希是否为 bcrypt 格式。
func IsBcrypt(hash string) bool {
	return strings.HasPrefix(hash, "$2a$") || strings.HasPrefix(hash, "$2b$") || strings.HasPrefix(hash, "$2y$")
}

// HashPassword 用 bcrypt 生成密码哈希。
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckPassword 校验明文密码与存储哈希是否匹配。
// 返回 (是否匹配, 是否需要升级为 bcrypt 存储)。
func CheckPassword(stored, plain string) (bool, bool) {
	if IsBcrypt(stored) {
		err := bcrypt.CompareHashAndPassword([]byte(stored), []byte(plain))
		return err == nil, false
	}
	// 遗留 MD5 hex（老配置）
	h := md5.Sum([]byte(plain))
	if hex.EncodeToString(h[:]) == stored {
		return true, true
	}
	return false, false
}