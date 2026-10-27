package tests

import (
	"os"
	"simple-captcha/config"
	"testing"
)

// TestMain 把全局服务管理器使用的控制台数据目录指向临时目录，
// 避免测试在仓库里生成 data/captcha.db 并打印初始化口令
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "simple-captcha-data-*")
	if err != nil {
		panic(err)
	}
	config.LoadConfig().Admin.DataDir = dir
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
