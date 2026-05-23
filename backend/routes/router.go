package routes

import (
	"net/http"
	"runtime"
	"simple-captcha/config"
	"simple-captcha/controllers"
	"simple-captcha/middleware"
	"simple-captcha/services"
	"time"

	"github.com/gin-gonic/gin"
)

var startTime = time.Now()

func getUptime() time.Duration {
	return time.Since(startTime)
}

// SetupRouter 设置所有路由
func SetupRouter() *gin.Engine {
	cfg := config.GetConfig()

	// 根据环境设置Gin模式
	if cfg.Server.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()

	// 初始化服务
	serviceManager := services.GetServiceManager()
	riskControl := serviceManager.GetRiskControlService()

	// 添加全局中间件（按顺序）
	r.Use(middleware.RequestIDMiddleware())
	r.Use(middleware.LoggerMiddleware())
	r.Use(middleware.ErrorHandlerMiddleware())
	r.Use(middleware.MetricsMiddleware())
	r.Use(middleware.SecurityMiddleware())
	r.Use(middleware.TrustedProxyMiddleware(r))
	r.Use(middleware.CompressionMiddleware())

	// 开发环境特殊中间件
	if cfg.Server.Environment == "development" {
		r.Use(middleware.RequestBodyLoggerMiddleware())
	}

	// 健康检查和监控路由（不需要限流）
	r.GET(cfg.Monitoring.HealthPath, healthCheckHandler)
	r.GET(cfg.Monitoring.MetricsPath, metricsHandler)
	r.GET(cfg.Monitoring.StatsPath, statsHandler)

	// 配置跨域中间件 (全量生效或只对api组生效)
	r.Use(middleware.CORSMiddleware())

	// 验证码API路由组（需要限流保护）
	api := r.Group("/api/v1/captchas")
	api.Use(middleware.RateLimitMiddleware(riskControl))
	api.Use(middleware.CacheMiddleware(5 * time.Minute)) // 缓存5分钟

	// 注册验证码相关路由
	registerCaptchaRoutes(api)

	return r
}

// registerCaptchaRoutes 注册验证码相关路由
func registerCaptchaRoutes(rg *gin.RouterGroup) {
	// 获取验证码资源
	rg.GET("/:type",
		middleware.ValidateCaptchaRequest(),
		controllers.GetCaptchaHandler)

	// 提交一次验证答案（用户侧）
	rg.POST("/:type/:key/attempts",
		middleware.ValidateVerifyRequest(),
		controllers.VerifyCaptchaHandler)

	// 查询验证会话状态（S2S，需携带 Bearer Token）
	rg.GET("/states/:key",
		middleware.APIKeyMiddleware(),
		middleware.ValidateStateRequest(),
		controllers.ValidateCaptchaHandler)
}

// healthCheckHandler 增强健康检查处理器
func healthCheckHandler(c *gin.Context) {
	cfg := config.GetConfig()
	serviceManager := services.GetServiceManager()

	// 检查各个服务的健康状态
	health := map[string]interface{}{
		"status":      "ok",
		"service":     "simple-captcha",
		"version":     "1.0.0",
		"environment": cfg.Server.Environment,
		"timestamp":   time.Now().Format(time.RFC3339),
		"uptime":      getUptime().String(),
	}

	// 检查Redis连接
	cacheService := serviceManager.GetCacheService()
	if cacheService != nil {
		if err := cacheService.Ping(); err != nil {
			health["redis"] = map[string]interface{}{
				"status": "error",
				"error":  err.Error(),
			}
			c.JSON(http.StatusServiceUnavailable, health)
			return
		} else {
			health["redis"] = map[string]interface{}{
				"status": "ok",
			}
		}
	}

	// 检查系统资源
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	health["system"] = map[string]interface{}{
		"goroutines": runtime.NumGoroutine(),
		"memory_mb":  m.Alloc / 1024 / 1024,
		"gc_cycles":  m.NumGC,
	}

	c.JSON(http.StatusOK, health)
}

// metricsHandler 指标处理器
func metricsHandler(c *gin.Context) {
	metrics := middleware.GetMetrics()

	// 添加系统指标
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	metrics["system"] = map[string]interface{}{
		"goroutines":   runtime.NumGoroutine(),
		"memory_alloc": m.Alloc,
		"memory_sys":   m.Sys,
		"gc_cycles":    m.NumGC,
		"gc_pause_ns":  m.PauseNs[(m.NumGC+255)%256],
	}

	// 添加服务指标
	serviceManager := services.GetServiceManager()
	if stats := serviceManager.GetStats(); stats != nil {
		metrics["services"] = stats
	}

	c.JSON(http.StatusOK, gin.H{
		"timestamp": time.Now().Format(time.RFC3339),
		"metrics":   metrics,
	})
}

// statsHandler 统计信息处理器
func statsHandler(c *gin.Context) {
	cfg := config.GetConfig()
	serviceManager := services.GetServiceManager()

	stats := map[string]interface{}{
		"service": map[string]interface{}{
			"name":        "simple-captcha",
			"version":     "1.0.0",
			"environment": cfg.Server.Environment,
			"config": map[string]interface{}{
				"server_port":          cfg.Server.Port,
				"redis_host":           cfg.Redis.Host,
				"captcha_cache_expire": cfg.Captcha.CacheExpiration.String(),
				"rate_limit":           cfg.Captcha.IPRateLimit,
			},
		},
		"runtime": map[string]interface{}{
			"go_version": runtime.Version(),
			"goroutines": runtime.NumGoroutine(),
			"cpu_count":  runtime.NumCPU(),
		},
		"timestamp": time.Now().Format(time.RFC3339),
	}

	// 添加服务统计
	if serviceStats := serviceManager.GetStats(); serviceStats != nil {
		stats["services"] = serviceStats
	}

	// 添加中间件指标
	stats["metrics"] = middleware.GetMetrics()

	c.JSON(http.StatusOK, stats)
}
