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
