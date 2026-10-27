package services

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWarnLegacyDataFiles(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	dir := t.TempDir()
	warnLegacyDataFiles(dir)
	assert.Empty(t, buf.String(), "没有旧文件时不提示")

	for _, name := range legacyDataFiles {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o600))
	}
	warnLegacyDataFiles(dir)
	assert.Equal(t, 2, strings.Count(buf.String(), "已不再使用"))

	// 只提示，不读取也不删除
	for _, name := range legacyDataFiles {
		_, err := os.Stat(filepath.Join(dir, name))
		assert.NoError(t, err)
	}
}
