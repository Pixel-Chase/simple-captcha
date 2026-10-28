package routes

import (
	"context"
	"log"
	"net/http"
	"runtime"
	"simple-captcha/cache"
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

	// 开发环境特殊中间件
	if cfg.Server.Environment == "development" {
		r.Use(middleware.RequestBodyLoggerMiddleware())
	}

	// 健康检查和监控路由（不需要限流）
	r.GET(cfg.Monitoring.HealthPath, healthCheckHandler)
	// 容器健康检查用 wget --spider，发的是 HEAD；Gin 不会把 HEAD 自动映射到 GET
	r.HEAD(cfg.Monitoring.HealthPath, healthCheckHandler)
	if cfg.Monitoring.Enabled {
		r.GET(cfg.Monitoring.MetricsPath, metricsHandler)
	}
	r.GET(cfg.Monitoring.StatsPath, statsHandler)

	// 验证码API路由组
	api := r.Group(config.APIPrefix + "/captchas")
	api.Use(middleware.NoCacheMiddleware()) // 验证码接口（含错误响应）一律禁止缓存

	// 注册验证码相关路由
	registerCaptchaRoutes(api, serviceManager.GetAppService(), middleware.CaptchaIPPressureMiddleware(riskControl))

	// 管理后台路由：管理员登录 + 背景图与接入应用管理
	RegisterAdminRoutes(r.Group(config.APIPrefix+"/admin"),
		serviceManager.GetAdminService(), serviceManager.GetAppService(), serviceManager.GetSettingsService(), riskControl)

	// 管理后台页面：内嵌的前端构建产物，与 API 同源，无需跨域配置
	consoleHandler := console.NewHandler(console.Assets())
	console.Register(r, consoleHandler)
	if consoleHandler.Built() {
		log.Printf("管理后台页面: %s/", console.BasePath)
	} else {
		log.Printf("管理后台前端未内嵌（未执行 pnpm build:console），%s/ 只显示构建提示", console.BasePath)
	}

	return r
}

// RegisterAdminRoutes 注册管理后台路由。
// 初始化与登录接口叠加 IP 限流：初始化以启动日志中的一次性口令为凭据，登录以行为验证作为前置；
// 其余接口要求 Bearer 会话令牌。
// 单独导出以便测试用自定义的服务实例（如临时素材与数据目录）构建路由。
func RegisterAdminRoutes(rg *gin.RouterGroup, admin *services.AdminService, apps *services.AppService,
	settings *services.SettingsService, riskControl services.RiskControlService) {
	adminCtrl := controllers.NewAdminController(admin)
	appCtrl := controllers.NewAppController(apps)
	settingsCtrl := controllers.NewSettingsController(settings)
	rg.Use(middleware.NoCacheMiddleware())

	rg.GET("/setup", adminCtrl.SetupStatusHandler)
	rg.POST("/setup", middleware.AdminRateLimitMiddleware(riskControl), adminCtrl.SetupHandler)
	rg.POST("/login", middleware.AdminRateLimitMiddleware(riskControl), adminCtrl.LoginHandler)

	authed := rg.Group("", middleware.AdminAuthMiddleware(admin))
	authed.GET("/session", adminCtrl.SessionHandler)
	authed.DELETE("/session", adminCtrl.LogoutHandler)
	authed.PUT("/password", middleware.AdminRateLimitMiddleware(riskControl), adminCtrl.ChangePasswordHandler)

	authed.GET("/backgrounds", adminCtrl.ListBackgroundsHandler)
	authed.POST("/backgrounds", adminCtrl.UploadBackgroundHandler)
	authed.GET("/backgrounds/:name", adminCtrl.PreviewBackgroundHandler)
	authed.DELETE("/backgrounds/:name", adminCtrl.DeleteBackgroundHandler)

	authed.GET("/risk/telemetry", adminCtrl.RiskTelemetryHandler)

	authed.GET("/apps", appCtrl.ListHandler)
	authed.POST("/apps", appCtrl.CreateHandler)
	authed.PUT("/apps/:id", appCtrl.UpdateHandler)
	authed.POST("/apps/:id/secret", appCtrl.RotateSecretHandler)
	authed.DELETE("/apps/:id", appCtrl.DeleteHandler)

	authed.GET("/settings", settingsCtrl.GetHandler)
	authed.PUT("/settings", settingsCtrl.UpdateHandler)
}

// registerCaptchaRoutes 注册验证码相关路由；ipPressure 只挂在用户侧的出题与提交接口上
func registerCaptchaRoutes(rg *gin.RouterGroup, apps *services.AppService, ipPressure gin.HandlerFunc) {
	// 用户侧接口按 appId 校验来源并返回跨域响应头；预检对任意来源放行，拦截在真实请求上，先于出题与计数
	appOrigin := middleware.AppOriginMiddleware(apps)

	// 获取验证码资源（POST：请求体携带环境信号，服务端出题前据此加难）
	rg.OPTIONS("/:type", appOrigin)
	rg.POST("/:type",
		appOrigin,
		ipPressure,
		middleware.ValidateIssueRequest(),
		controllers.IssueCaptchaHandler)

	// 提交一次验证答案（用户侧）
	rg.OPTIONS("/:type/:key/attempts", appOrigin)
	rg.POST("/:type/:key/attempts",
		appOrigin,
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

	// 检查持久数据存储
	if db := serviceManager.GetStorage(); db != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		err := db.PingContext(ctx)
		cancel()
		if err != nil {
			health["status"] = "error"
			health["storage"] = map[string]interface{}{
				"status": "error",
				"error":  err.Error(),
			}
			c.JSON(http.StatusServiceUnavailable, health)
			return
		}
		health["storage"] = map[string]interface{}{
			"status": "ok",
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
	if stats := serviceManager.GetServiceStatistics(); stats != nil {
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
				"captcha_cache_expire":   config.Runtime().Captcha.Expire().String(),
				"ip_escalate_per_minute": config.Runtime().Captcha.IPEscalatePerMinute,
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
	if serviceStats := serviceManager.GetServiceStatistics(); serviceStats != nil {
		stats["services"] = serviceStats
	}

	// 存储：持久数据（SQLite）与进程内存中的短期数据条目数
	storageStats := map[string]interface{}{"status": "ok"}
	if db := serviceManager.GetStorage(); db != nil {
		if err := db.PingContext(c.Request.Context()); err != nil {
			storageStats["status"] = "error"
		}
		storageStats["path"] = db.Path()
	}
	mem := cache.Shared()
	memory := map[string]interface{}{}
	for _, kind := range []cache.Kind{cache.KindChallenge, cache.KindRateLimit, cache.KindClient, cache.KindFingerprint} {
		memory[string(kind)] = map[string]interface{}{"entries": mem.Len(kind), "evicted": mem.Evicted(kind)}
	}
	stats["storage"] = map[string]interface{}{"sqlite": storageStats, "memory": memory}
	if telemetry := serviceManager.GetTelemetryStore(); telemetry != nil {
		stats["telemetry"] = map[string]interface{}{"dropped": telemetry.Dropped()}
	}

	// 添加中间件指标：关闭采集时请求数恒为 0，GetMetrics 算错误率会除以 0，得到的 NaN/Inf 无法序列化成 JSON
	if cfg.Monitoring.Enabled {
		stats["metrics"] = middleware.GetMetrics()
	}

	c.JSON(http.StatusOK, stats)
}
