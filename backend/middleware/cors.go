package middleware

import (
	"log"
	"net/http"
	"strings"

	"simple-captcha/helper"
	"simple-captcha/services"

	"github.com/gin-gonic/gin"
)

const appIDContextKey = "app_id"

// AppOriginMiddleware 用户侧接口（出题、提交）的按应用跨域与来源校验，只挂在这两类路由上：
//   - 预检（OPTIONS）对任意 Origin 都返回 204 并回显来源。预检一旦失败浏览器根本不会发出真实请求，
//     SDK 只能看到网络错误、读不到错误码；真正的拦截放在真实请求上
//   - 真实请求先按查询参数 appId 校验来源，失败返回 403 与错误码（同样回显来源，SDK 才读得到），
//     通过后才进入出题或提交逻辑，被拒绝的请求不消耗出图 CPU
//
// 管理接口与 S2S 状态查询不挂本中间件，不返回任何跨域响应头
func AppOriginMiddleware(apps *services.AppService) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			addVaryHeader(c, "Origin")
		}

		if c.Request.Method == http.MethodOptions {
			c.Header("Access-Control-Allow-Methods", "POST, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type")
			c.Header("Access-Control-Max-Age", "86400")
			addVaryHeader(c, "Access-Control-Request-Method")
			addVaryHeader(c, "Access-Control-Request-Headers")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		appID := c.Query("appId")
		if err := apps.CheckOrigin(appID, origin, RequestOrigin(c)); err != nil {
			// appId 与 Origin 都是外部输入，用 %q 转义避免日志注入
			log.Printf("来源校验未通过: app=%q, origin=%q, ip=%s, code=%d", appID, origin, helper.GetRealClientIP(c), err.Code)
			helper.ErrorResponse(c, err)
			c.Abort()
			return
		}
		c.Set(appIDContextKey, appID)
		c.Next()
	}
}

// RequestOrigin 请求自身的 scheme://host，用于判定系统应用的同源请求。
// 只有直连来源命中 TRUSTED_PROXIES 时才采信 X-Forwarded-Host / X-Forwarded-Proto（取第一个值），
// 否则任何客户端都能伪造这两个头冒充同源
func RequestOrigin(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	host := c.Request.Host
	if helper.IsTrustedProxyRequest(c) {
		if v := firstHeaderValue(c.GetHeader("X-Forwarded-Proto")); v != "" {
			scheme = strings.ToLower(v)
		}
		if v := firstHeaderValue(c.GetHeader("X-Forwarded-Host")); v != "" {
			host = v
		}
	}
	return scheme + "://" + host
}

// GetRequestAppID 通过来源校验的应用 ID（查询参数 appId）；未经过校验时返回空串
func GetRequestAppID(c *gin.Context) string {
	if v, ok := c.Get(appIDContextKey); ok {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}

func firstHeaderValue(v string) string {
	first, _, _ := strings.Cut(v, ",")
	return strings.TrimSpace(first)
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
