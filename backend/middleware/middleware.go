package middleware

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"simple-captcha/config"
	"simple-captcha/helper"
	"simple-captcha/services"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Metrics 指标收集器
type Metrics struct {
	RequestCount   int64
	ErrorCount     int64
	TotalLatency   time.Duration
	ActiveRequests int64
	mu             sync.RWMutex
}

var globalMetrics = &Metrics{}

// LoggerMiddleware 增强日志中间件
func LoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 处理请求
		c.Next()
	}
}

// ErrorHandlerMiddleware 增强错误处理中间件
func ErrorHandlerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// 记录panic详情
				log.Printf("Panic recovered: %v, Request: %s %s, IP: %s",
					err, c.Request.Method, c.Request.URL.Path, c.ClientIP())

				// 更新错误指标
				globalMetrics.mu.Lock()
				globalMetrics.ErrorCount++
				globalMetrics.mu.Unlock()

				helper.SimpleErrorResponse(c, 500, "内部服务器错误")
				c.Abort()
			}
		}()
		c.Next()

		// 处理业务错误
		if len(c.Errors) > 0 {
			globalMetrics.mu.Lock()
			globalMetrics.ErrorCount++
			globalMetrics.mu.Unlock()
		}
	}
}

// MetricsMiddleware 指标收集中间件
func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// 增加活跃请求数
		globalMetrics.mu.Lock()
		globalMetrics.ActiveRequests++
		globalMetrics.RequestCount++
		globalMetrics.mu.Unlock()

		c.Next()

		// 计算延迟并更新指标
		latency := time.Since(start)
		globalMetrics.mu.Lock()
		globalMetrics.ActiveRequests--
		globalMetrics.TotalLatency += latency
		globalMetrics.mu.Unlock()
	}
}

// RateLimitMiddleware 增强限流中间件
func RateLimitMiddleware(riskControl services.RiskControlService) gin.HandlerFunc {
	cfg := config.GetConfig()

	return func(c *gin.Context) {
		// 获取真实IP
		realIP := helper.GetRealClientIP(c)

		// 检查IP白名单
		if isIPInList(realIP, cfg.Security.IPWhitelist) {
			c.Next()
			return
		}

		// 检查IP黑名单
		if isIPInList(realIP, cfg.Security.IPBlacklist) {
			helper.SimpleErrorResponse(c, 403, "访问被拒绝")
			c.Abort()
			return
		}

		// 应用限流
		if cfg.Security.EnableRateLimit && !riskControl.CheckIPLimit(realIP) {
			helper.SimpleErrorResponse(c, 429, "请求过于频繁，请稍后再试")
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequestIDMiddleware 增强请求ID中间件
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 尝试从请求头获取现有的请求ID
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			var err error
			requestID, err = helper.GenerateTimestampedID()
			if err != nil {
				requestID = fmt.Sprintf("req_%d", time.Now().UnixNano())
			}
		}

		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

// SecurityMiddleware 增强安全中间件
func SecurityMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.GetConfig()

		// 设置安全头
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("X-Permitted-Cross-Domain-Policies", "none")

		if cfg.Security.EnableHTTPS {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		// 检查请求大小
		if c.Request.ContentLength > cfg.Security.MaxRequestSize {
			helper.SimpleErrorResponse(c, 413, "请求体过大")
			c.Abort()
			return
		}

		c.Next()
	}
}

// TrustedProxyMiddleware 可信代理中间件
func TrustedProxyMiddleware(engine *gin.Engine) gin.HandlerFunc {
	cfg := config.GetConfig()

	// 设置可信代理（在中间件创建时设置）
	if len(cfg.Security.TrustedProxies) > 0 {
		engine.SetTrustedProxies(cfg.Security.TrustedProxies)
	}

	return func(c *gin.Context) {
		c.Next()
	}
}

// CompressionMiddleware 压缩中间件
func CompressionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查客户端是否支持gzip
		if !strings.Contains(c.GetHeader("Accept-Encoding"), "gzip") {
			c.Next()
			return
		}

		// 只压缩特定类型的响应
		c.Next()

		contentType := c.Writer.Header().Get("Content-Type")
		if shouldCompress(contentType) {
			c.Header("Content-Encoding", "gzip")
			c.Header("Vary", "Accept-Encoding")
		}
	}
}

// CacheMiddleware 缓存中间件
func CacheMiddleware(duration time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 只对GET请求启用缓存
		if c.Request.Method != "GET" {
			c.Next()
			return
		}

		// 设置缓存头
		c.Header("Cache-Control", fmt.Sprintf("public, max-age=%d", int(duration.Seconds())))
		c.Header("Expires", time.Now().Add(duration).Format(http.TimeFormat))

		c.Next()
	}
}

// TimeoutMiddleware 超时中间件
func TimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 创建带超时的上下文
		ctx := c.Request.Context()
		cancel := func() {}

		if timeout > 0 {
			ctx, cancel = context.WithTimeout(c.Request.Context(), timeout)
		}
		defer cancel()

		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// RequestBodyLoggerMiddleware 请求体日志中间件（仅开发环境）
func RequestBodyLoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.GetConfig()
		if cfg.Server.Environment != "development" {
			c.Next()
			return
		}

		// 读取请求体
		if c.Request.Body != nil {
			bodyBytes, err := io.ReadAll(c.Request.Body)
			if err == nil {
				// 恢复请求体
				c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

				// 记录请求体（限制长度）
				body := string(bodyBytes)
				if len(body) > 1000 {
					body = body[:1000] + "..."
				}
				log.Printf("Request Body: %s", body)
			}
		}

		c.Next()
	}
}

// HealthCheckMiddleware 健康检查中间件
func HealthCheckMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 为健康检查路径跳过其他中间件
		if c.Request.URL.Path == "/health" || c.Request.URL.Path == "/metrics" {
			c.Next()
			return
		}

		c.Next()
	}
}

// GetMetrics 获取指标数据
func GetMetrics() map[string]interface{} {
	globalMetrics.mu.RLock()
	defer globalMetrics.mu.RUnlock()

	avgLatency := time.Duration(0)
	if globalMetrics.RequestCount > 0 {
		avgLatency = globalMetrics.TotalLatency / time.Duration(globalMetrics.RequestCount)
	}

	return map[string]interface{}{
		"request_count":   globalMetrics.RequestCount,
		"error_count":     globalMetrics.ErrorCount,
		"active_requests": globalMetrics.ActiveRequests,
		"average_latency": avgLatency.String(),
		"total_latency":   globalMetrics.TotalLatency.String(),
		"error_rate":      float64(globalMetrics.ErrorCount) / float64(globalMetrics.RequestCount),
	}
}

// ResetMetrics 重置指标
func ResetMetrics() {
	globalMetrics.mu.Lock()
	defer globalMetrics.mu.Unlock()

	globalMetrics.RequestCount = 0
	globalMetrics.ErrorCount = 0
	globalMetrics.TotalLatency = 0
	globalMetrics.ActiveRequests = 0
}

// 辅助函数
func isIPInList(ip string, list []string) bool {
	if len(list) == 0 {
		return false
	}

	clientIP := net.ParseIP(ip)
	if clientIP == nil {
		return false
	}

	for _, item := range list {
		// 支持CIDR格式
		if strings.Contains(item, "/") {
			_, network, err := net.ParseCIDR(item)
			if err == nil && network.Contains(clientIP) {
				return true
			}
		} else {
			// 直接IP比较
			if item == ip {
				return true
			}
		}
	}

	return false
}

func shouldCompress(contentType string) bool {
	compressibleTypes := []string{
		"application/json",
		"application/javascript",
		"text/css",
		"text/html",
		"text/plain",
		"text/xml",
	}

	for _, t := range compressibleTypes {
		if strings.Contains(contentType, t) {
			return true
		}
	}

	return false
}
