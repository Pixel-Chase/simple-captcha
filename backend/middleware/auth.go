package middleware

import (
	"simple-captcha/helper"
	"simple-captcha/services"
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

const appContextKey = "s2s_app"

// AppAuthMiddleware S2S 接口鉴权：校验 Authorization: Bearer <应用密钥>。
// 应用在管理后台创建；密钥缺失、无效或应用已停用一律 401，没有任何应用时同样全部拒绝
func AppAuthMiddleware(apps *services.AppService) gin.HandlerFunc {
	return func(c *gin.Context) {
		app, ok := apps.Authenticate(BearerToken(c))
		if !ok {
			c.Header("WWW-Authenticate", "Bearer")
			helper.ErrorResponse(c, helper.ErrAppUnauthorized)
			c.Abort()
			return
		}
		c.Set(appContextKey, app)
		c.Next()
	}
}

// GetAppID 返回通过 S2S 鉴权的应用 ID，用于日志追溯；未经过鉴权时返回 "-"
func GetAppID(c *gin.Context) string {
	if app, ok := c.Get(appContextKey); ok {
		if info, ok := app.(*services.AppInfo); ok {
			return info.ID
		}
	}
	return "-"
}
