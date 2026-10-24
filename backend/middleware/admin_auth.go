package middleware

import (
	"simple-captcha/helper"
	"simple-captcha/services"

	"github.com/gin-gonic/gin"
)

const adminSessionContextKey = "admin_session"

// AdminAuthMiddleware 管理接口鉴权：校验 Bearer 会话令牌。
// 与 APIKeyMiddleware 不同，这里没有"未配置则放行"的分支，令牌缺失或无效一律 401
func AdminAuthMiddleware(admin *services.AdminService) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, ok := admin.Authenticate(BearerToken(c))
		if !ok {
			c.Header("WWW-Authenticate", "Bearer")
			helper.ErrorResponse(c, helper.ErrUnauthorized)
			c.Abort()
			return
		}

		c.Set(adminSessionContextKey, session)
		c.Next()
	}
}

// GetAdminSession 读取鉴权中间件写入的会话；未经过鉴权时返回 nil
func GetAdminSession(c *gin.Context) *services.AdminSession {
	value, ok := c.Get(adminSessionContextKey)
	if !ok {
		return nil
	}
	session, _ := value.(*services.AdminSession)
	return session
}
