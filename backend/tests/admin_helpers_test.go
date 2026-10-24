package tests

import "golang.org/x/crypto/bcrypt"

// bcryptHash 生成测试用的密码哈希
func bcryptHash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}
