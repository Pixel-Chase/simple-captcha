package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestRunHashPassword(t *testing.T) {
	t.Run("从标准输入读取", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runHashPassword(nil, strings.NewReader("s3cret-demo\n"), &stdout, &stderr)
		require.Equal(t, 0, code, stderr.String())

		hash := strings.TrimSpace(stdout.String())
		assert.True(t, strings.HasPrefix(hash, "$2a$"))
		assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(hash), []byte("s3cret-demo")))
		assert.Error(t, bcrypt.CompareHashAndPassword([]byte(hash), []byte("wrong")))
		assert.Contains(t, stderr.String(), "单引号")
	})

	t.Run("从参数读取，末尾无换行的标准输入也可用", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runHashPassword([]string{"from-arg"}, strings.NewReader(""), &stdout, &stderr)
		require.Equal(t, 0, code)
		assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(strings.TrimSpace(stdout.String())), []byte("from-arg")))

		stdout.Reset()
		code = runHashPassword(nil, strings.NewReader("no-newline"), &stdout, &stderr)
		require.Equal(t, 0, code)
		assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(strings.TrimSpace(stdout.String())), []byte("no-newline")))
	})

	t.Run("空密码被拒绝", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		assert.Equal(t, 1, runHashPassword(nil, strings.NewReader("\n"), &stdout, &stderr))
		assert.Equal(t, 1, runHashPassword([]string{""}, strings.NewReader(""), &stdout, &stderr))
		assert.Empty(t, stdout.String())
	})
}
