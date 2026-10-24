package config

import (
	"fmt"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultAPIPrefix = "/api/v1"

// Config 应用配置结构
type Config struct {
	Server     ServerConfig
	CORS       CORSConfig
	Redis      RedisConfig
	Captcha    CaptchaConfig
	Logging    LoggingConfig
	Monitoring MonitoringConfig
	Security   SecurityConfig
	Risk       RiskConfig
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Port            string
	Host            string
	APIPrefix       string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	Environment     string
	GracefulTimeout time.Duration
	MaxHeaderBytes  int
}

// CORSConfig 跨域资源共享配置
type CORSConfig struct {
	Enabled          bool
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           time.Duration
}

// RedisConfig Redis配置
type RedisConfig struct {
	Host         string
	Port         string
	Password     string
	DB           int
	MaxRetries   int
	PoolSize     int
	MinIdleConns int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	PoolTimeout  time.Duration
	IdleTimeout  time.Duration
	MaxConnAge   time.Duration
}

// CaptchaConfig 验证码配置
type CaptchaConfig struct {
	CacheExpiration    time.Duration
	SecondVerifyExpire time.Duration
	MaxAttempts        int
	IPRateLimit        int
	RateLimitWindow    time.Duration
	ImagePath          string
	FontPath           string
	DefaultDifficulty  int           // 基础难度等级 1-5，风控状态会在此基础上升级
	MinSolveTime       time.Duration // 出题到提交的最短间隔，低于该值判定为自动化提交
	BindClient         bool          // 是否将验证码会话绑定到客户端（IP + UA 哈希）
	ImageQuality       int           // 主图 JPEG 质量 55-100
	MaxScale           int           // 允许的最大出图倍率（对应前端 devicePixelRatio）1-3
	PowEnabled         bool          // 是否下发工作量证明（PoW）挑战
	PowRequired        bool          // 是否强制要求提交 PoW（false 时旧版 SDK 仍可通过，但会计入风险分）
	PowDifficulty      int           // PoW 基础难度：SHA-256 前导零位数
	PowMaxDifficulty   int           // PoW 难度上限
}

// RiskConfig 风控评分配置
type RiskConfig struct {
	Enabled           bool          // 是否启用行为/环境风控评分
	Mode              string        // enforce：按阈值拦截；log：仅记录分数不拦截（灰度观察）
	EscalateThreshold int           // 达到该分数后提升该客户端后续题目的难度与 PoW 位数
	BlockThreshold    int           // 达到该分数直接拒绝本次校验（enforce 模式）
	StateTTL          time.Duration // 客户端风控状态在 Redis 中的保留时间
	TelemetryLogSize  int           // Redis 中保留的最近遥测记录条数，0 表示不落 Redis
}

// LoggingConfig 日志配置
type LoggingConfig struct {
	Level      string
	Format     string
	Output     string
	MaxSize    int
	MaxBackups int
	MaxAge     int
	Compress   bool
}

// MonitoringConfig 监控配置
type MonitoringConfig struct {
	Enabled         bool
	MetricsPath     string
	HealthPath      string
	StatsPath       string
	CollectInterval time.Duration
}

// SecurityConfig 安全配置
type SecurityConfig struct {
	EnableHTTPS      bool
	CertFile         string
	KeyFile          string
	TrustedProxies   []string
	EnableCSRF       bool
	CSRFSecret       string
	JWTSecret        string
	SessionSecret    string
	MaxRequestSize   int64
	EnableRateLimit  bool
	GlobalRateLimit  int
	IPWhitelist      []string
	IPBlacklist      []string
	APIKey           string
	RateLimitBackend string // redis：多实例共享的滑动窗口限流；memory：单实例内存限流
}

var (
	AppConfig *Config
	configMu  sync.RWMutex
)

// LoadConfig 从环境变量加载配置
func LoadConfig() *Config {
	configMu.RLock()
	if AppConfig != nil {
		defer configMu.RUnlock()
		return AppConfig
	}
	configMu.RUnlock()

	configMu.Lock()
	defer configMu.Unlock()

	// 双重检查
	if AppConfig != nil {
		return AppConfig
	}

	// 从环境变量加载配置
	AppConfig = loadFromEnv()

	// 验证配置
	if err := validateConfig(AppConfig); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	log.Printf("Configuration loaded successfully (Environment: %s)", AppConfig.Server.Environment)
	return AppConfig
}

// GetConfig 获取当前配置（线程安全）
func GetConfig() *Config {
	configMu.RLock()
	defer configMu.RUnlock()
	return AppConfig
}

// loadFromEnv 从环境变量加载配置
func loadFromEnv() *Config {
	return &Config{
		Server: ServerConfig{
			Port:            getEnv("PORT", "8080"),
			Host:            getEnv("HOST", "0.0.0.0"),
			APIPrefix:       normalizeAPIPrefix(getEnv("API_PREFIX", defaultAPIPrefix)),
			ReadTimeout:     getDurationEnv("SERVER_READ_TIMEOUT", 30*time.Second),
			WriteTimeout:    getDurationEnv("SERVER_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:     getDurationEnv("SERVER_IDLE_TIMEOUT", 60*time.Second),
			Environment:     getEnv("GIN_MODE", "release"),
			GracefulTimeout: getDurationEnv("SERVER_GRACEFUL_TIMEOUT", 30*time.Second),
			MaxHeaderBytes:  getIntEnv("SERVER_MAX_HEADER_BYTES", 1<<20),
		},
		CORS: CORSConfig{
			Enabled:          getBoolEnv("CORS_ENABLED", true),
			AllowOrigins:     getStringSliceEnv("CORS_ALLOW_ORIGINS", []string{"*"}),
			AllowMethods:     getStringSliceEnv("CORS_ALLOW_METHODS", []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
			AllowHeaders:     getStringSliceEnv("CORS_ALLOW_HEADERS", []string{"Content-Type", "Content-Length", "Accept-Encoding", "X-CSRF-Token", "Authorization", "Accept", "Origin", "Cache-Control", "X-Requested-With"}),
			ExposeHeaders:    getStringSliceEnv("CORS_EXPOSE_HEADERS", []string{}),
			AllowCredentials: getBoolEnv("CORS_ALLOW_CREDENTIALS", false),
			MaxAge:           getDurationEnv("CORS_MAX_AGE", 24*time.Hour),
		},
		Redis: RedisConfig{
			Host:         getEnv("REDIS_HOST", "localhost"),
			Port:         getEnv("REDIS_PORT", "6379"),
			Password:     getEnv("REDIS_PASSWORD", ""),
			DB:           getIntEnv("REDIS_DB", 0),
			MaxRetries:   getIntEnv("REDIS_MAX_RETRIES", 3),
			PoolSize:     getIntEnv("REDIS_POOL_SIZE", 10),
			MinIdleConns: getIntEnv("REDIS_MIN_IDLE", 5),
			DialTimeout:  getDurationEnv("REDIS_DIAL_TIMEOUT", 5*time.Second),
			ReadTimeout:  getDurationEnv("REDIS_READ_TIMEOUT", 3*time.Second),
			WriteTimeout: getDurationEnv("REDIS_WRITE_TIMEOUT", 3*time.Second),
			PoolTimeout:  getDurationEnv("REDIS_POOL_TIMEOUT", 4*time.Second),
			IdleTimeout:  getDurationEnv("REDIS_IDLE_TIMEOUT", 5*time.Minute),
			MaxConnAge:   getDurationEnv("REDIS_MAX_CONN_AGE", 30*time.Minute),
		},
		Captcha: CaptchaConfig{
			CacheExpiration:    getDurationEnv("CAPTCHA_EXPIRE_TIME", 5*time.Minute),
			SecondVerifyExpire: getDurationEnv("CAPTCHA_SECOND_VERIFY_EXPIRE", 2*time.Minute),
			MaxAttempts:        getIntEnv("CAPTCHA_MAX_ATTEMPTS", 5),
			IPRateLimit:        getIntEnv("CAPTCHA_IP_RATE_LIMIT", 30),
			RateLimitWindow:    getDurationEnv("CAPTCHA_RATE_LIMIT_WINDOW", 1*time.Minute),
			ImagePath:          getEnv("CAPTCHA_IMAGE_PATH", "./images"),
			FontPath:           getEnv("CAPTCHA_FONT_PATH", "./fonts"),
			DefaultDifficulty:  getIntEnv("CAPTCHA_DEFAULT_DIFFICULTY", 3),
			MinSolveTime:       getDurationEnv("CAPTCHA_MIN_SOLVE_TIME", 500*time.Millisecond),
			BindClient:         getBoolEnv("CAPTCHA_BIND_CLIENT", true),
			ImageQuality:       getIntEnv("CAPTCHA_IMAGE_QUALITY", 80),
			MaxScale:           getIntEnv("CAPTCHA_MAX_SCALE", 3),
			PowEnabled:         getBoolEnv("CAPTCHA_POW_ENABLED", true),
			PowRequired:        getBoolEnv("CAPTCHA_POW_REQUIRED", false),
			PowDifficulty:      getIntEnv("CAPTCHA_POW_DIFFICULTY", 14),
			PowMaxDifficulty:   getIntEnv("CAPTCHA_POW_MAX_DIFFICULTY", 20),
		},
		Risk: RiskConfig{
			Enabled:           getBoolEnv("RISK_ENABLED", true),
			Mode:              strings.ToLower(strings.TrimSpace(getEnv("RISK_MODE", "enforce"))),
			EscalateThreshold: getIntEnv("RISK_ESCALATE_THRESHOLD", 55),
			BlockThreshold:    getIntEnv("RISK_BLOCK_THRESHOLD", 85),
			StateTTL:          getDurationEnv("RISK_STATE_TTL", 30*time.Minute),
			TelemetryLogSize:  getIntEnv("RISK_TELEMETRY_LOG_SIZE", 1000),
		},
		Logging: LoggingConfig{
			Level:      getEnv("LOG_LEVEL", "info"),
			Format:     getEnv("LOG_FORMAT", "json"),
			Output:     getEnv("LOG_OUTPUT", "stdout"),
			MaxSize:    getIntEnv("LOG_MAX_SIZE", 100),
			MaxBackups: getIntEnv("LOG_MAX_BACKUPS", 3),
			MaxAge:     getIntEnv("LOG_MAX_AGE", 28),
			Compress:   getBoolEnv("LOG_COMPRESS", true),
		},
		Monitoring: MonitoringConfig{
			Enabled:         getBoolEnv("ENABLE_METRICS", true),
			MetricsPath:     getEnv("METRICS_PATH", "/metrics"),
			HealthPath:      getEnv("HEALTH_PATH", "/health"),
			StatsPath:       getEnv("STATS_PATH", "/stats"),
			CollectInterval: getDurationEnv("METRICS_COLLECT_INTERVAL", 30*time.Second),
		},
		Security: SecurityConfig{
			EnableHTTPS:      getBoolEnv("ENABLE_HTTPS", false),
			CertFile:         getEnv("CERT_FILE", ""),
			KeyFile:          getEnv("KEY_FILE", ""),
			TrustedProxies:   getStringSliceEnv("TRUSTED_PROXIES", []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}),
			EnableCSRF:       getBoolEnv("ENABLE_CSRF", false),
			CSRFSecret:       getEnv("CSRF_SECRET", ""),
			JWTSecret:        getEnv("JWT_SECRET", ""),
			SessionSecret:    getEnv("SESSION_SECRET", ""),
			MaxRequestSize:   getInt64Env("MAX_REQUEST_SIZE", 32<<20), // 32MB
			EnableRateLimit:  getBoolEnv("ENABLE_RATE_LIMIT", true),
			GlobalRateLimit:  getIntEnv("RATE_LIMIT_REQUESTS", 100),
			IPWhitelist:      getStringSliceEnv("IP_WHITELIST", []string{}),
			IPBlacklist:      getStringSliceEnv("IP_BLACKLIST", []string{}),
			APIKey:           getEnv("API_KEY", ""),
			RateLimitBackend: strings.ToLower(strings.TrimSpace(getEnv("RATE_LIMIT_BACKEND", "redis"))),
		},
	}
}

// normalizeAPIPrefix 统一 API 路径前缀格式，根路径使用空字符串表示。
func normalizeAPIPrefix(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultAPIPrefix
	}

	prefix := path.Clean("/" + strings.TrimLeft(value, "/"))
	if prefix == "/" {
		return ""
	}

	return prefix
}

// validateConfig 验证配置
func validateConfig(config *Config) error {
	if config.Server.Port == "" {
		return fmt.Errorf("server port cannot be empty")
	}
	if config.Redis.Host == "" {
		return fmt.Errorf("redis host cannot be empty")
	}
	if config.Captcha.ImagePath == "" {
		return fmt.Errorf("captcha image path cannot be empty")
	}
	if config.CORS.Enabled {
		if len(config.CORS.AllowOrigins) == 0 {
			return fmt.Errorf("cors allow origins cannot be empty when CORS is enabled")
		}
		if len(config.CORS.AllowMethods) == 0 {
			return fmt.Errorf("cors allow methods cannot be empty when CORS is enabled")
		}
		if config.CORS.MaxAge < 0 {
			return fmt.Errorf("cors max age cannot be negative")
		}
	}
	if config.Captcha.DefaultDifficulty < 1 || config.Captcha.DefaultDifficulty > 5 {
		return fmt.Errorf("CAPTCHA_DEFAULT_DIFFICULTY must be between 1 and 5")
	}
	if config.Captcha.ImageQuality < 55 || config.Captcha.ImageQuality > 100 {
		return fmt.Errorf("CAPTCHA_IMAGE_QUALITY must be between 55 and 100")
	}
	if config.Captcha.MaxScale < 1 || config.Captcha.MaxScale > 3 {
		return fmt.Errorf("CAPTCHA_MAX_SCALE must be between 1 and 3")
	}
	if config.Captcha.MinSolveTime < 0 {
		return fmt.Errorf("CAPTCHA_MIN_SOLVE_TIME cannot be negative")
	}
	if config.Captcha.PowDifficulty < 1 || config.Captcha.PowDifficulty > 26 {
		return fmt.Errorf("CAPTCHA_POW_DIFFICULTY must be between 1 and 26")
	}
	if config.Captcha.PowMaxDifficulty < config.Captcha.PowDifficulty || config.Captcha.PowMaxDifficulty > 26 {
		return fmt.Errorf("CAPTCHA_POW_MAX_DIFFICULTY must be between CAPTCHA_POW_DIFFICULTY and 26")
	}
	if config.Risk.Mode != "enforce" && config.Risk.Mode != "log" {
		return fmt.Errorf("RISK_MODE must be enforce or log")
	}
	if config.Risk.EscalateThreshold < 0 || config.Risk.EscalateThreshold > 100 ||
		config.Risk.BlockThreshold < 0 || config.Risk.BlockThreshold > 100 ||
		config.Risk.EscalateThreshold > config.Risk.BlockThreshold {
		return fmt.Errorf("risk thresholds must satisfy 0 <= RISK_ESCALATE_THRESHOLD <= RISK_BLOCK_THRESHOLD <= 100")
	}
	if config.Security.RateLimitBackend != "redis" && config.Security.RateLimitBackend != "memory" {
		return fmt.Errorf("RATE_LIMIT_BACKEND must be redis or memory")
	}
	return nil
}

// 环境变量辅助函数
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getInt64Env(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getBoolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getStringSliceEnv(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		// 简单的逗号分隔解析
		result := []string{}
		for _, item := range splitString(value, ",") {
			if trimmed := trimSpace(item); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return defaultValue
}

// 简单的字符串处理函数
func splitString(s, sep string) []string {
	if s == "" {
		return []string{}
	}
	// 简单实现，避免引入strings包
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if i < len(s)-len(sep)+1 && s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	result = append(result, s[start:])
	return result
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
