package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadFromEnv_Admin(t *testing.T) {
	t.Run("默认配置", func(t *testing.T) {
		cfg := loadFromEnv()

		assert.Equal(t, "./data", cfg.Admin.DataDir)
		assert.NoError(t, validateConfig(cfg))
	})

	t.Run("管理后台常驻开启，配置项始终校验", func(t *testing.T) {
		t.Setenv("ADMIN_SESSION_TTL", "-1s")

		assert.ErrorContains(t, validateConfig(loadFromEnv()), "ADMIN_SESSION_TTL")
	})
}

func TestLoadFromEnv_IPCounters(t *testing.T) {
	// 验证码的单 IP 加难阈值已移到策略配置（见 runtime_test.go），这里只剩管理员接口的限流
	t.Run("管理员限流可配置，0 表示关闭", func(t *testing.T) {
		t.Setenv("ADMIN_IP_LIMIT_PER_MINUTE", "12")
		cfg := loadFromEnv()
		assert.Equal(t, 12, cfg.Admin.IPLimitPerMin)
		assert.NoError(t, validateConfig(cfg))

		t.Setenv("ADMIN_IP_LIMIT_PER_MINUTE", "0")
		assert.NoError(t, validateConfig(loadFromEnv()))
	})

	t.Run("拒绝负数", func(t *testing.T) {
		t.Setenv("ADMIN_IP_LIMIT_PER_MINUTE", "-1")
		assert.ErrorContains(t, validateConfig(loadFromEnv()), "ADMIN_IP_LIMIT_PER_MINUTE")
	})
}
