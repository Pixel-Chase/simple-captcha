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
