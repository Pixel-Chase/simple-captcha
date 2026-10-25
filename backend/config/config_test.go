package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoadFromEnv_CORS(t *testing.T) {
	t.Setenv("CORS_ENABLED", "true")
	t.Setenv("CORS_ALLOW_ORIGINS", "https://app.example.com, https://admin.example.com")
	t.Setenv("CORS_ALLOW_METHODS", "GET,POST,OPTIONS")
	t.Setenv("CORS_ALLOW_HEADERS", "Content-Type,Authorization")
	t.Setenv("CORS_EXPOSE_HEADERS", "X-Request-ID")
	t.Setenv("CORS_ALLOW_CREDENTIALS", "true")
	t.Setenv("CORS_MAX_AGE", "30m")

	cfg := loadFromEnv()

	assert.True(t, cfg.CORS.Enabled)
	assert.Equal(t, []string{"https://app.example.com", "https://admin.example.com"}, cfg.CORS.AllowOrigins)
	assert.Equal(t, []string{"GET", "POST", "OPTIONS"}, cfg.CORS.AllowMethods)
	assert.Equal(t, []string{"Content-Type", "Authorization"}, cfg.CORS.AllowHeaders)
	assert.Equal(t, []string{"X-Request-ID"}, cfg.CORS.ExposeHeaders)
	assert.True(t, cfg.CORS.AllowCredentials)
	assert.Equal(t, 30*time.Minute, cfg.CORS.MaxAge)
}

func TestLoadFromEnv_APIPrefix(t *testing.T) {
	t.Run("使用默认前缀", func(t *testing.T) {
		t.Setenv("API_PREFIX", "")

		cfg := loadFromEnv()

		assert.Equal(t, "/api/v1", cfg.Server.APIPrefix)
	})

	t.Run("规范化自定义前缀", func(t *testing.T) {
		t.Setenv("API_PREFIX", " captcha/api/ ")

		cfg := loadFromEnv()

		assert.Equal(t, "/captcha/api", cfg.Server.APIPrefix)
	})

	t.Run("允许挂载到根路径", func(t *testing.T) {
		t.Setenv("API_PREFIX", "/")

		cfg := loadFromEnv()

		assert.Empty(t, cfg.Server.APIPrefix)
	})
}

func TestLoadFromEnv_Admin(t *testing.T) {
	t.Run("默认配置", func(t *testing.T) {
		cfg := loadFromEnv()

		assert.Equal(t, "./data", cfg.Admin.DataDir)
		assert.Equal(t, "admin", cfg.Admin.Username)
		assert.NoError(t, validateConfig(cfg))
	})

	t.Run("管理后台常驻开启，配置项始终校验", func(t *testing.T) {
		t.Setenv("ADMIN_SESSION_TTL", "-1s")

		assert.ErrorContains(t, validateConfig(loadFromEnv()), "ADMIN_SESSION_TTL")
	})
}

func TestLoadFromEnv_Redis(t *testing.T) {
	t.Run("读取连接池与超时参数", func(t *testing.T) {
		t.Setenv("REDIS_POOL_SIZE", "20")
		t.Setenv("REDIS_MIN_IDLE", "2")
		t.Setenv("REDIS_MAX_RETRIES", "0")
		t.Setenv("REDIS_POOL_TIMEOUT", "6s")
		t.Setenv("REDIS_MAX_CONN_AGE", "0s")

		cfg := loadFromEnv()

		assert.Equal(t, 20, cfg.Redis.PoolSize)
		assert.Equal(t, 2, cfg.Redis.MinIdleConns)
		assert.Equal(t, 0, cfg.Redis.MaxRetries)
		assert.Equal(t, 6*time.Second, cfg.Redis.PoolTimeout)
		assert.Equal(t, time.Duration(0), cfg.Redis.MaxConnAge)
		assert.NoError(t, validateConfig(cfg))
	})

	t.Run("拒绝 go-redis 无法正确处理的取值", func(t *testing.T) {
		t.Setenv("REDIS_POOL_SIZE", "10")
		cases := []struct {
			key, value, message string
		}{
			{"REDIS_POOL_SIZE", "0", "REDIS_POOL_SIZE"},
			{"REDIS_POOL_SIZE", "-1", "REDIS_POOL_SIZE"},
			{"REDIS_MIN_IDLE", "-1", "REDIS_MIN_IDLE"},
			{"REDIS_MIN_IDLE", "11", "REDIS_MIN_IDLE"},
			{"REDIS_MAX_RETRIES", "-1", "REDIS_MAX_RETRIES"},
		}
		for _, tc := range cases {
			t.Run(tc.key+"="+tc.value, func(t *testing.T) {
				t.Setenv(tc.key, tc.value)
				assert.ErrorContains(t, validateConfig(loadFromEnv()), tc.message)
			})
		}
	})
}

func TestLoadFromEnv_IPCounters(t *testing.T) {
	t.Run("验证码加难与管理员限流分别配置，0 表示关闭", func(t *testing.T) {
		t.Setenv("CAPTCHA_IP_ESCALATE_PER_MINUTE", "0")
		t.Setenv("ADMIN_IP_LIMIT_PER_MINUTE", "12")

		cfg := loadFromEnv()

		assert.Equal(t, 0, cfg.Captcha.IPEscalatePerMin)
		assert.Equal(t, 12, cfg.Admin.IPLimitPerMin)
		assert.NoError(t, validateConfig(cfg))
	})

	t.Run("拒绝负数", func(t *testing.T) {
		for _, key := range []string{"CAPTCHA_IP_ESCALATE_PER_MINUTE", "ADMIN_IP_LIMIT_PER_MINUTE"} {
			t.Run(key, func(t *testing.T) {
				t.Setenv(key, "-1")
				assert.ErrorContains(t, validateConfig(loadFromEnv()), key)
			})
		}
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
