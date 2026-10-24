package middleware

import (
	"net/http"
	"simple-captcha/config"
	"strings"

	"github.com/gin-gonic/gin"
)

// BearerToken 从 Authorization 头中提取 Bearer 令牌，格式不符时返回空串
func BearerToken(c *gin.Context) string {
	const prefix = "Bearer "
	header := c.GetHeader("Authorization")
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

// APIKeyMiddleware S2S API鉴权中间件
func APIKeyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.GetConfig()
		expectedKey := cfg.Security.APIKey

		// 如果未配置 API Key，则直接放行（适用于完全内网隔离或者关闭鉴权的情况）
		if expectedKey == "" {
			c.Next()
			return
		}

		// 仅检查标准的 Authorization: Bearer <Token>
		apiKey := BearerToken(c)
		if apiKey == "" || apiKey != expectedKey {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized: Invalid or missing Bearer token in Authorization header",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
