package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"simple-captcha/config"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func setupCORSRouter(corsConfig config.CORSConfig) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORSMiddleware(corsConfig))
	router.Any("/resource", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return router
}

func TestCORSMiddleware_AllowsConfiguredOrigin(t *testing.T) {
	router := setupCORSRouter(config.CORSConfig{
		Enabled:          true,
		AllowOrigins:     []string{"https://app.example.com"},
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		ExposeHeaders:    []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	})

	request := httptest.NewRequest(http.MethodGet, "/resource", nil)
	request.Header.Set("Origin", "https://app.example.com")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "https://app.example.com", response.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", response.Header().Get("Access-Control-Allow-Credentials"))
	assert.Equal(t, "X-Request-ID", response.Header().Get("Access-Control-Expose-Headers"))
	assert.Contains(t, response.Header().Values("Vary"), "Origin")
}

func TestCORSMiddleware_HandlesPreflightRequest(t *testing.T) {
	router := setupCORSRouter(config.CORSConfig{
		Enabled:      true,
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "OPTIONS"},
		AllowHeaders: []string{"Content-Type", "Authorization"},
		MaxAge:       24 * time.Hour,
	})

	request := httptest.NewRequest(http.MethodOptions, "/resource", nil)
	request.Header.Set("Origin", "https://app.example.com")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "Authorization")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNoContent, response.Code)
	assert.Equal(t, "*", response.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "GET, POST, OPTIONS", response.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Content-Type, Authorization", response.Header().Get("Access-Control-Allow-Headers"))
	assert.Equal(t, "86400", response.Header().Get("Access-Control-Max-Age"))
}

func TestCORSMiddleware_RejectsDisallowedPreflight(t *testing.T) {
	router := setupCORSRouter(config.CORSConfig{
		Enabled:      true,
		AllowOrigins: []string{"https://app.example.com"},
		AllowMethods: []string{"GET", "OPTIONS"},
	})

	request := httptest.NewRequest(http.MethodOptions, "/resource", nil)
	request.Header.Set("Origin", "https://evil.example.com")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusForbidden, response.Code)
	assert.Empty(t, response.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORSMiddleware_RejectsDisallowedPreflightMethod(t *testing.T) {
	router := setupCORSRouter(config.CORSConfig{
		Enabled:      true,
		AllowOrigins: []string{"https://app.example.com"},
		AllowMethods: []string{"GET", "OPTIONS"},
	})

	request := httptest.NewRequest(http.MethodOptions, "/resource", nil)
	request.Header.Set("Origin", "https://app.example.com")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusForbidden, response.Code)
	assert.Equal(t, "https://app.example.com", response.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORSMiddleware_CanBeDisabled(t *testing.T) {
	router := setupCORSRouter(config.CORSConfig{Enabled: false})

	request := httptest.NewRequest(http.MethodGet, "/resource", nil)
	request.Header.Set("Origin", "https://app.example.com")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Empty(t, response.Header().Get("Access-Control-Allow-Origin"))
}
