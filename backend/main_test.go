package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"simple-captcha/config"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

var printedPassword = regexp.MustCompile(`密  码: (\S+)`)

func TestRunResetAdminPassword(t *testing.T) {
	cfg := config.AdminConfig{Username: "ops", DataDir: filepath.Join(t.TempDir(), "data")}

	readCredential := func() map[string]interface{} {
		data, err := os.ReadFile(filepath.Join(cfg.DataDir, "admin.json"))
		require.NoError(t, err)
		var cred map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &cred))
		return cred
	}
	reset := func() string {
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, runResetAdminPassword(cfg, &stdout, &stderr), stderr.String())
		assert.Contains(t, stdout.String(), "用户名: ops")
		match := printedPassword.FindStringSubmatch(stdout.String())
		require.Len(t, match, 2, stdout.String())
		return match[1]
	}

	// 数据目录不存在时自动创建；落盘的只有哈希，且要求下次登录改密
	first := reset()
	assert.Len(t, first, 20)
	cred := readCredential()
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(cred["passwordHash"].(string)), []byte(first)))
	assert.Equal(t, true, cred["mustChangePassword"])
	assert.NotContains(t, readFile(t, filepath.Join(cfg.DataDir, "admin.json")), first)

	info, err := os.Stat(filepath.Join(cfg.DataDir, "admin.json"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	// 再次重置：密码与凭据代次都会更换，旧密码失效
	generation := cred["generation"]
	second := reset()
	assert.NotEqual(t, first, second)
	cred = readCredential()
	assert.NotEqual(t, generation, cred["generation"])
	assert.Error(t, bcrypt.CompareHashAndPassword([]byte(cred["passwordHash"].(string)), []byte(first)))
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(cred["passwordHash"].(string)), []byte(second)))
}

func TestRunResetAdminPassword_UnwritableDir(t *testing.T) {
	// 数据目录路径被一个普通文件占用，无法创建目录
	blocker := filepath.Join(t.TempDir(), "data")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))

	var stdout, stderr bytes.Buffer
	code := runResetAdminPassword(config.AdminConfig{Username: "admin", DataDir: blocker}, &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "重置管理员密码失败")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}
