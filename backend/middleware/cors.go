package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"simple-captcha/config"

	"github.com/gin-gonic/gin"
)

// CORSMiddleware 跨域中间件
// 可选参数用于测试或自定义路由注入；不传时使用应用全局配置。
func CORSMiddleware(corsConfigs ...config.CORSConfig) gin.HandlerFunc {
	var cfg config.CORSConfig
	if len(corsConfigs) > 0 {
		cfg = corsConfigs[0]
	} else {
		appConfig := config.GetConfig()
		if appConfig == nil {
			appConfig = config.LoadConfig()
		}
		cfg = appConfig.CORS
	}

	return func(c *gin.Context) {
		if !cfg.Enabled {
			c.Next()
			return
		}

		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			c.Next()
			return
		}

		allowAllOrigins := contains(cfg.AllowOrigins, "*")
		if !allowAllOrigins || cfg.AllowCredentials {
			addVaryHeader(c, "Origin")
		}
		if !allowAllOrigins && !contains(cfg.AllowOrigins, origin) {
			if isPreflightRequest(c.Request) {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.Next()
			return
		}

		// 携带凭证时规范不允许返回通配符，因此需要回显已获准的来源。
		if allowAllOrigins && !cfg.AllowCredentials {
			c.Header("Access-Control-Allow-Origin", "*")
		} else {
			c.Header("Access-Control-Allow-Origin", origin)
		}

		if cfg.AllowCredentials {
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		if len(cfg.ExposeHeaders) > 0 {
			c.Header("Access-Control-Expose-Headers", strings.Join(cfg.ExposeHeaders, ", "))
		}

		if isPreflightRequest(c.Request) {
			requestMethod := c.GetHeader("Access-Control-Request-Method")
			if requestMethod != "" && !containsFold(cfg.AllowMethods, requestMethod) {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}

			c.Header("Access-Control-Allow-Methods", strings.Join(cfg.AllowMethods, ", "))
			if len(cfg.AllowHeaders) > 0 {
				c.Header("Access-Control-Allow-Headers", strings.Join(cfg.AllowHeaders, ", "))
			}
			if cfg.MaxAge > 0 {
				c.Header("Access-Control-Max-Age", strconv.FormatInt(int64(cfg.MaxAge.Seconds()), 10))
			}
			addVaryHeader(c, "Access-Control-Request-Method")
			addVaryHeader(c, "Access-Control-Request-Headers")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func isPreflightRequest(request *http.Request) bool {
	return request.Method == http.MethodOptions && request.Header.Get("Access-Control-Request-Method") != ""
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

func addVaryHeader(c *gin.Context, value string) {
	for _, current := range c.Writer.Header().Values("Vary") {
		for _, item := range strings.Split(current, ",") {
			if strings.EqualFold(strings.TrimSpace(item), value) {
				return
			}
		}
	}
	c.Writer.Header().Add("Vary", value)
}
