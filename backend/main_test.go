package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"simple-captcha/storage"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var printedSetupCode = regexp.MustCompile(`初始化口令:\s+(\S+)`)

func TestRunResetAdmin(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	ctx := context.Background()

	reset := func() string {
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, runResetAdmin(dataDir, &stdout, &stderr), stderr.String())
		match := printedSetupCode.FindStringSubmatch(stdout.String())
		require.Len(t, match, 2, stdout.String())
		return match[1]
	}

	// 数据目录不存在时自动创建；库中没有管理员账号，只有口令哈希
	first := reset()
	assert.Regexp(t, `^[A-Z2-9]{4}-[A-Z2-9]{4}-[A-Z2-9]{4}$`, first)
	db, err := storage.Open(dataDir)
	require.NoError(t, err)
	var accounts, setups int
	var hash string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(1) FROM admin_account`).Scan(&accounts))
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(1), MAX(code_hash) FROM admin_setup`).Scan(&setups, &hash))
	require.NoError(t, db.Close())
	assert.Zero(t, accounts)
	assert.Equal(t, 1, setups)
	assert.NotContains(t, readFile(t, filepath.Join(dataDir, storage.FileName)), first)

	info, err := os.Stat(filepath.Join(dataDir, storage.FileName))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	// 再次重置：口令更换
	second := reset()
	assert.NotEqual(t, first, second)
}

func TestRunResetAdmin_UnwritableDir(t *testing.T) {
	// 数据目录路径被一个普通文件占用，无法创建目录
	blocker := filepath.Join(t.TempDir(), "data")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))

	var stdout, stderr bytes.Buffer
	code := runResetAdmin(blocker, &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "重置管理员账号失败")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}
