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
	t.Run("默认数据目录", func(t *testing.T) {
		t.Setenv("ADMIN_DATA_DIR", "")

		cfg := loadFromEnv()

		assert.Equal(t, "./data", cfg.Admin.DataDir)
		assert.Equal(t, "admin", cfg.Admin.Username)
		assert.NoError(t, validateConfig(cfg))
	})

	t.Run("管理后台常驻开启，配置项始终校验", func(t *testing.T) {
		t.Setenv("ADMIN_DATA_DIR", "/var/lib/captcha")
		t.Setenv("ADMIN_SESSION_TTL", "-1s")

		cfg := loadFromEnv()

		assert.Equal(t, "/var/lib/captcha", cfg.Admin.DataDir)
		assert.ErrorContains(t, validateConfig(cfg), "ADMIN_SESSION_TTL")
	})
}

func TestLoadFromEnv_ConsolePath(t *testing.T) {
	t.Run("默认挂在 /console，并规范化写法", func(t *testing.T) {
		t.Setenv("CONSOLE_PATH", "")
		assert.Equal(t, "/console", loadFromEnv().Admin.ConsolePath)

		t.Setenv("CONSOLE_PATH", " captcha/console/ ")
		cfg := loadFromEnv()
		assert.Equal(t, "/captcha/console", cfg.Admin.ConsolePath)
		assert.NoError(t, validateConfig(cfg))
	})

	t.Run("拒绝根路径与路由重叠", func(t *testing.T) {
		cases := []struct{ apiPrefix, consolePath string }{
			{"/api/v1", "/"},
			{"/api/v1", "/api/v1"},
			{"/api/v1", "/api"},
			{"/api/v1", "/api/v1/console"},
			{"/api/v1", "/health"},
			{"/", "/admin"},
			{"/", "/captchas/console"},
		}
		for _, tc := range cases {
			t.Setenv("API_PREFIX", tc.apiPrefix)
			t.Setenv("CONSOLE_PATH", tc.consolePath)
			assert.ErrorContains(t, validateConfig(loadFromEnv()), "CONSOLE_PATH", "%+v", tc)
		}

		t.Setenv("API_PREFIX", "/")
		t.Setenv("CONSOLE_PATH", "/console")
		assert.NoError(t, validateConfig(loadFromEnv()))
	})
}
