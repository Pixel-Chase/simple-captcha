package middleware

import (
	"bytes"
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

const ipRequestsKey = "ip_requests"

// CaptchaIPPressureMiddleware 验证码出题与提交的单 IP 计数：记录该 IP 在窗口内的请求数供出题加难，不因请求量拒绝
func CaptchaIPPressureMiddleware(riskControl services.RiskControlService) gin.HandlerFunc {
	return ipGuard(func(c *gin.Context, ip string) bool {
		c.Set(ipRequestsKey, riskControl.TrackCaptchaIP(ip))
		return true
	})
}

// AdminRateLimitMiddleware 管理员登录与改密的单 IP 限流，每分钟超过 ADMIN_IP_LIMIT_PER_MINUTE 即拒绝
func AdminRateLimitMiddleware(riskControl services.RiskControlService) gin.HandlerFunc {
	return ipGuard(func(_ *gin.Context, ip string) bool {
		return riskControl.CheckAdminIP(ip)
	})
}

// GetIPRequestCount 返回 CaptchaIPPressureMiddleware 记录的该 IP 最近一分钟请求数；白名单或关闭加难时为 0
func GetIPRequestCount(c *gin.Context) int {
	return c.GetInt(ipRequestsKey)
}

// ipGuard 白名单直接放行、黑名单拒绝，其余交给 allow 判断
func ipGuard(allow func(c *gin.Context, ip string) bool) gin.HandlerFunc {
	cfg := config.GetConfig()

	return func(c *gin.Context) {
		realIP := helper.GetRealClientIP(c)

		if isIPInList(realIP, cfg.Security.IPWhitelist) {
			c.Next()
			return
		}

		if isIPInList(realIP, cfg.Security.IPBlacklist) {
			helper.SimpleErrorResponse(c, 403, "访问被拒绝")
			c.Abort()
			return
		}

		if !allow(c, realIP) {
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

		// 检查请求大小：ContentLength 只能拦住声明了长度的请求，chunked 编码时为 -1，
		// 因此再用 MaxBytesReader 在读取阶段硬性截断
		if c.Request.ContentLength > cfg.Security.MaxRequestSize {
			helper.SimpleErrorResponse(c, 413, "请求体过大")
			c.Abort()
			return
		}
		if c.Request.Body != nil && cfg.Security.MaxRequestSize > 0 {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, cfg.Security.MaxRequestSize)
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

// NoCacheMiddleware 禁止缓存中间件：验证码题目与错误响应都不允许被浏览器或 CDN 缓存
func NoCacheMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
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
