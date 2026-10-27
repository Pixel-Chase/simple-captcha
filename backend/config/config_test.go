package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// 跨域改为按应用配置允许来源：旧部署仍设置 CORS_* 时只告警
func TestRemovedCORSEnvWarns(t *testing.T) {
	t.Setenv("CORS_ALLOW_ORIGINS", "https://app.example.com")
	t.Setenv("CORS_ENABLED", "true")
	assert.NoError(t, validateConfig(loadFromEnv()))
	warnings := strings.Join(removedEnvWarnings(), "\n")
	assert.Contains(t, warnings, "CORS_ALLOW_ORIGINS")
	assert.Contains(t, warnings, "CORS_ENABLED")
	assert.Contains(t, warnings, "允许来源")
}

// API 路径固定为 /api/v1：旧部署仍设置 API_PREFIX 时只告警
func TestRemovedAPIPrefixWarns(t *testing.T) {
	t.Setenv("API_PREFIX", "/captcha/api")
	assert.NoError(t, validateConfig(loadFromEnv()))
	assert.Contains(t, strings.Join(removedEnvWarnings(), "\n"), "API_PREFIX")
	assert.Equal(t, "/api/v1", APIPrefix)
}

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

// Redis 已移除：旧部署仍设置 REDIS_* 时只告警，不再读取也不影响启动
func TestRemovedRedisEnvWarns(t *testing.T) {
	t.Setenv("REDIS_HOST", "redis.internal")
	t.Setenv("REDIS_POOL_SIZE", "0")

	assert.NoError(t, validateConfig(loadFromEnv()))
	warnings := strings.Join(removedEnvWarnings(), "\n")
	assert.Contains(t, warnings, "REDIS_HOST")
	assert.Contains(t, warnings, "REDIS_POOL_SIZE")
	assert.Contains(t, warnings, "已不再使用 Redis")
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

	t.Run("仍设置已移除的变量时给出警告", func(t *testing.T) {
		for key := range removedEnvVars {
			t.Setenv(key, "")
		}
		assert.Empty(t, removedEnvWarnings())

		t.Setenv("ENABLE_RATE_LIMIT", "false")
		t.Setenv("CAPTCHA_IP_RATE_LIMIT", "30")
		warnings := removedEnvWarnings()
		if assert.Len(t, warnings, 2) {
			assert.Contains(t, warnings[0], "CAPTCHA_IP_RATE_LIMIT")
			assert.Contains(t, warnings[1], "ENABLE_RATE_LIMIT")
		}
	})
}
