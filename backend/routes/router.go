package routes

import (
	"log"
	"net/http"
	"runtime"
	"simple-captcha/config"
	"simple-captcha/console"
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

	// 客户端 IP 解析：代理头列表来自 REMOTE_IP_HEADERS（默认仅 X-Forwarded-For / X-Real-IP），
	// 且只有在直连来源命中 TrustedProxies 时才会被采信
	r.RemoteIPHeaders = cfg.Security.RemoteIPHeaders

	// 初始化服务
	serviceManager := services.GetServiceManager()
	riskControl := serviceManager.GetRiskControlService()

	// 添加全局中间件（按顺序）
	r.Use(middleware.RequestIDMiddleware())
	r.Use(middleware.LoggerMiddleware())
	r.Use(middleware.ErrorHandlerMiddleware())
	if cfg.Monitoring.Enabled {
		r.Use(middleware.MetricsMiddleware())
	}
	r.Use(middleware.SecurityMiddleware())
	r.Use(middleware.TrustedProxyMiddleware(r))
	r.Use(middleware.CompressionMiddleware())
	r.Use(middleware.CORSMiddleware(cfg.CORS))

	// 开发环境特殊中间件
	if cfg.Server.Environment == "development" {
		r.Use(middleware.RequestBodyLoggerMiddleware())
	}

	// 健康检查和监控路由（不需要限流）
	r.GET(cfg.Monitoring.HealthPath, healthCheckHandler)
	if cfg.Monitoring.Enabled {
		r.GET(cfg.Monitoring.MetricsPath, metricsHandler)
	}
	r.GET(cfg.Monitoring.StatsPath, statsHandler)

	// 验证码API路由组
	api := r.Group(cfg.Server.APIPrefix + "/captchas")
	api.Use(middleware.NoCacheMiddleware()) // 验证码接口（含错误响应）一律禁止缓存

	// 注册验证码相关路由
	registerCaptchaRoutes(api, serviceManager.GetAppService(), middleware.CaptchaIPPressureMiddleware(riskControl))

	// 管理后台路由：管理员登录 + 背景图与接入应用管理
	RegisterAdminRoutes(r.Group(cfg.Server.APIPrefix+"/admin"),
		serviceManager.GetAdminService(), serviceManager.GetAppService(), riskControl)

	// 管理后台页面：内嵌的前端构建产物，与 API 同源，无需跨域配置
	consoleHandler := console.NewHandler(console.Assets(), cfg.Server.APIPrefix)
	console.Register(r, consoleHandler)
	if consoleHandler.Built() {
		log.Printf("管理后台页面: %s/", console.BasePath)
	} else {
		log.Printf("管理后台前端未内嵌（未执行 pnpm build:console），%s/ 只显示构建提示", console.BasePath)
	}

	return r
}

// RegisterAdminRoutes 注册管理后台路由。
// 登录接口叠加 IP 限流并以行为验证作为前置；其余接口要求 Bearer 会话令牌，
// 且在修改初始密码之前只开放会话查询、注销与改密。
// 单独导出以便测试用自定义的服务实例（如临时素材与数据目录）构建路由。
func RegisterAdminRoutes(rg *gin.RouterGroup, admin *services.AdminService, apps *services.AppService, riskControl services.RiskControlService) {
	adminCtrl := controllers.NewAdminController(admin)
	appCtrl := controllers.NewAppController(apps)
	rg.Use(middleware.NoCacheMiddleware())

	rg.POST("/login", middleware.AdminRateLimitMiddleware(riskControl), adminCtrl.LoginHandler)

	authed := rg.Group("", middleware.AdminAuthMiddleware(admin))
	authed.GET("/session", adminCtrl.SessionHandler)
	authed.DELETE("/session", adminCtrl.LogoutHandler)
	authed.PUT("/password", middleware.AdminRateLimitMiddleware(riskControl), adminCtrl.ChangePasswordHandler)

	ready := authed.Group("", middleware.RequirePasswordChanged())
	ready.GET("/backgrounds", adminCtrl.ListBackgroundsHandler)
	ready.POST("/backgrounds", adminCtrl.UploadBackgroundHandler)
	ready.GET("/backgrounds/:name", adminCtrl.PreviewBackgroundHandler)
	ready.DELETE("/backgrounds/:name", adminCtrl.DeleteBackgroundHandler)

	ready.GET("/apps", appCtrl.ListHandler)
	ready.POST("/apps", appCtrl.CreateHandler)
	ready.PUT("/apps/:id", appCtrl.UpdateHandler)
	ready.POST("/apps/:id/secret", appCtrl.RotateSecretHandler)
	ready.DELETE("/apps/:id", appCtrl.DeleteHandler)
}

// registerCaptchaRoutes 注册验证码相关路由；ipPressure 只挂在用户侧的出题与提交接口上
func registerCaptchaRoutes(rg *gin.RouterGroup, apps *services.AppService, ipPressure gin.HandlerFunc) {
	// 获取验证码资源
	rg.GET("/:type",
		ipPressure,
		middleware.ValidateCaptchaRequest(),
		controllers.GetCaptchaHandler)

	// 提交一次验证答案（用户侧）
	rg.POST("/:type/:key/attempts",
		ipPressure,
		middleware.ValidateVerifyRequest(),
		controllers.VerifyCaptchaHandler)

	// 查询验证会话状态（S2S，需携带 Bearer <应用密钥>）。不按 IP 计数：调用方是业务后端，
	// 全站所有用户的二次校验都从它的固定 IP 发出，请求量反映的是全站登录量而不是某个出口的压力
	rg.GET("/states/:key",
		middleware.AppAuthMiddleware(apps),
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
				"server_port":            cfg.Server.Port,
				"redis_host":             cfg.Redis.Host,
				"captcha_cache_expire":   cfg.Captcha.CacheExpiration.String(),
				"ip_escalate_per_minute": cfg.Captcha.IPEscalatePerMin,
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

	// 添加中间件指标：关闭采集时请求数恒为 0，GetMetrics 算错误率会除以 0，得到的 NaN/Inf 无法序列化成 JSON
	if cfg.Monitoring.Enabled {
		stats["metrics"] = middleware.GetMetrics()
	}

	c.JSON(http.StatusOK, stats)
}
