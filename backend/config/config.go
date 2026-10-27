package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"sort"
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
	Monitoring MonitoringConfig
	Security   SecurityConfig
	Risk       RiskConfig
	Admin      AdminConfig
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
	IPEscalatePerMin   int // 同一 IP 每分钟出题与提交超过该次数后续题目加 1 级、超过 2 倍加 2 级，0 关闭；验证码请求从不因 IP 被拒绝
	ImagePath          string
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
	EnvScoreCap       int           // 环境 / 请求侧信号对风险分的贡献上限 0-100，0 不限制；仅凭环境信号达不到拦截阈值
	BlockMinBehavior  int           // 触发拦截所需的最低行为分 0-100（命中 webdriver 等硬性标记时不要求）
	FPReplayThreshold int           // 同一设备指纹在时间窗内对应的不同客户端数超过该值即视为重放加难，0 关闭计数
	FPReplayWindow    time.Duration // 指纹重放计数的时间窗

	TLSFingerprintHeader   string // 可信代理注入的 TLS 指纹（JA3 / JA4）请求头名，空表示不读取
	HTTP2FingerprintHeader string // 可信代理注入的 HTTP/2 指纹请求头名，空表示不读取
	TLSFingerprintList     string // 指纹参考列表 JSON（browser 家族映射 + deny 列表）路径，空表示只记录不计分
}

// AdminConfig 管理控制台配置（管理员登录、背景图与接入应用管理），随服务常驻开启。
// 管理员密码不走配置：首次启动时随机生成并打印到控制台，凭据与接入应用持久化在 DataDir。
type AdminConfig struct {
	Username       string        // 管理员用户名
	DataDir        string        // 控制台数据目录：管理员凭据（admin.json）与接入应用（apps.json），固定为工作目录下的 data（镜像内 /app/data）
	SessionTTL     time.Duration // 登录会话有效期
	MaxUploadSize  int64         // 单张背景图上传体积上限（字节）
	MaxImagePixels int           // 单张背景图像素总数上限，防止解压炸弹
	IPLimitPerMin  int           // 登录与改密接口的单 IP 每分钟上限，0 不限；与验证码请求分开计数
}

// MonitoringConfig 监控配置
type MonitoringConfig struct {
	Enabled     bool // 是否采集请求指标并开放 MetricsPath；关闭后 StatsPath 也不再附带请求指标
	MetricsPath string
	HealthPath  string
	StatsPath   string
}

// SecurityConfig 安全配置
type SecurityConfig struct {
	EnableHTTPS       bool
	CertFile          string
	KeyFile           string
	TrustedProxies    []string
	RemoteIPHeaders   []string // 解析客户端 IP 时按顺序尝试的代理头；仅在直连来源命中 TrustedProxies 时采信
	MaxRequestSize    int64
	VerifyMaxBodySize int64 // 校验提交接口（attempts）的请求体上限；遥测数据体积有限，单独收紧
	IPWhitelist       []string
	IPBlacklist       []string
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
	for _, warning := range removedEnvWarnings() {
		log.Print(warning)
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
			IPEscalatePerMin:   getIntEnv("CAPTCHA_IP_ESCALATE_PER_MINUTE", 30),
			ImagePath:          getEnv("CAPTCHA_IMAGE_PATH", "./images"),
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
			EnvScoreCap:       getIntEnv("RISK_ENV_SCORE_CAP", 60),
			BlockMinBehavior:  getIntEnv("RISK_BLOCK_MIN_BEHAVIOR", 20),
			FPReplayThreshold: getIntEnv("RISK_FP_REPLAY_THRESHOLD", 5),
			FPReplayWindow:    getDurationEnv("RISK_FP_REPLAY_WINDOW", 10*time.Minute),

			TLSFingerprintHeader:   strings.TrimSpace(getEnv("RISK_TLS_FINGERPRINT_HEADER", "")),
			HTTP2FingerprintHeader: strings.TrimSpace(getEnv("RISK_HTTP2_FINGERPRINT_HEADER", "")),
			TLSFingerprintList:     strings.TrimSpace(getEnv("RISK_TLS_FINGERPRINT_LIST", "")),
		},
		Monitoring: MonitoringConfig{
			Enabled:     getBoolEnv("ENABLE_METRICS", true),
			MetricsPath: getEnv("METRICS_PATH", "/metrics"),
			HealthPath:  getEnv("HEALTH_PATH", "/health"),
			StatsPath:   getEnv("STATS_PATH", "/stats"),
		},
		Security: SecurityConfig{
			EnableHTTPS:    getBoolEnv("ENABLE_HTTPS", false),
			CertFile:       getEnv("CERT_FILE", ""),
			KeyFile:        getEnv("KEY_FILE", ""),
			TrustedProxies: getStringSliceEnv("TRUSTED_PROXIES", []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}),
			// 默认只采信常规反向代理头。CF-Connecting-IP / True-Client-IP 之类的 CDN 头只应在
			// 确认部署于对应 CDN 之后显式开启，否则前置代理原样透传时攻击者可借此伪造来源 IP。
			RemoteIPHeaders:   getStringSliceEnv("REMOTE_IP_HEADERS", []string{"X-Forwarded-For", "X-Real-IP"}),
			MaxRequestSize:    getInt64Env("MAX_REQUEST_SIZE", 32<<20),      // 32MB
			VerifyMaxBodySize: getInt64Env("VERIFY_MAX_BODY_SIZE", 256<<10), // 256KB：600 个轨迹点加环境信号远小于该值
			IPWhitelist:       getStringSliceEnv("IP_WHITELIST", []string{}),
			IPBlacklist:       getStringSliceEnv("IP_BLACKLIST", []string{}),
		},
		Admin: AdminConfig{
			Username:       getEnv("ADMIN_USERNAME", "admin"),
			DataDir:        "./data",
			SessionTTL:     getDurationEnv("ADMIN_SESSION_TTL", 2*time.Hour),
			MaxUploadSize:  getInt64Env("ADMIN_MAX_UPLOAD_SIZE", 5<<20), // 5MB
			MaxImagePixels: getIntEnv("ADMIN_MAX_IMAGE_PIXELS", 24_000_000),
			IPLimitPerMin:  getIntEnv("ADMIN_IP_LIMIT_PER_MINUTE", 30),
		},
	}
}

// removedEnvVars 已移除且曾经生效的环境变量及替代方式。部署环境里还设着它们时启动打印警告，避免旧配置被静默忽略
var removedEnvVars = map[string]string{
	"CAPTCHA_IP_RATE_LIMIT":     "验证码改用 CAPTCHA_IP_ESCALATE_PER_MINUTE（只加难不拒绝），管理员登录与改密改用 ADMIN_IP_LIMIT_PER_MINUTE",
	"CAPTCHA_RATE_LIMIT_WINDOW": "单 IP 计数窗口固定为 1 分钟，阈值按每分钟配置",
	"ENABLE_RATE_LIMIT":         "把 CAPTCHA_IP_ESCALATE_PER_MINUTE 或 ADMIN_IP_LIMIT_PER_MINUTE 设为 0 即可关闭对应功能",
	"RATE_LIMIT_BACKEND":        "单 IP 计数固定存放在 Redis，多实例共享",
	"ADMIN_DATA_DIR":            "数据目录固定为工作目录下的 data（镜像内 /app/data）；原先指向别处时把卷挂到该路径或迁移 admin.json 与 apps.json，否则会重新生成管理员密码，全部应用密钥失效",
	"CONSOLE_PATH":              "管理后台固定挂在 /console/",
}

// removedEnvWarnings 返回当前环境中仍在设置的已移除变量的警告，按变量名排序
func removedEnvWarnings() []string {
	var warnings []string
	for key, hint := range removedEnvVars {
		if os.Getenv(key) != "" {
			warnings = append(warnings, fmt.Sprintf("环境变量 %s 已移除，当前设置被忽略：%s", key, hint))
		}
	}
	sort.Strings(warnings)
	return warnings
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
	// 连接池参数直接交给 go-redis：池大小为 0 会被替换成 10*GOMAXPROCS，负数则在建池时 panic
	if config.Redis.PoolSize < 1 {
		return fmt.Errorf("REDIS_POOL_SIZE must be positive")
	}
	if config.Redis.MinIdleConns < 0 || config.Redis.MinIdleConns > config.Redis.PoolSize {
		return fmt.Errorf("REDIS_MIN_IDLE must be between 0 and REDIS_POOL_SIZE")
	}
	if config.Redis.MaxRetries < 0 {
		return fmt.Errorf("REDIS_MAX_RETRIES cannot be negative")
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
	if config.Risk.EnvScoreCap < 0 || config.Risk.EnvScoreCap > 100 {
		return fmt.Errorf("RISK_ENV_SCORE_CAP must be between 0 and 100")
	}
	if config.Risk.BlockMinBehavior < 0 || config.Risk.BlockMinBehavior > 100 {
		return fmt.Errorf("RISK_BLOCK_MIN_BEHAVIOR must be between 0 and 100")
	}
	if config.Risk.FPReplayThreshold < 0 {
		return fmt.Errorf("RISK_FP_REPLAY_THRESHOLD cannot be negative")
	}
	if config.Risk.FPReplayThreshold > 0 && config.Risk.FPReplayWindow <= 0 {
		return fmt.Errorf("RISK_FP_REPLAY_WINDOW must be positive when fingerprint replay counting is enabled")
	}
	if path := config.Risk.TLSFingerprintList; path != "" {
		if config.Risk.TLSFingerprintHeader == "" && config.Risk.HTTP2FingerprintHeader == "" {
			return fmt.Errorf("RISK_TLS_FINGERPRINT_LIST requires RISK_TLS_FINGERPRINT_HEADER or RISK_HTTP2_FINGERPRINT_HEADER")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("RISK_TLS_FINGERPRINT_LIST cannot be read: %w", err)
		}
		var list struct {
			Browser map[string]string `json:"browser"`
			Deny    []string          `json:"deny"`
		}
		if err := json.Unmarshal(data, &list); err != nil {
			return fmt.Errorf("RISK_TLS_FINGERPRINT_LIST is not valid JSON: %w", err)
		}
	}
	if strings.TrimSpace(config.Admin.Username) == "" {
		return fmt.Errorf("ADMIN_USERNAME cannot be empty")
	}
	if config.Admin.SessionTTL <= 0 {
		return fmt.Errorf("ADMIN_SESSION_TTL must be positive")
	}
	if config.Admin.MaxUploadSize <= 0 || config.Admin.MaxUploadSize > config.Security.MaxRequestSize {
		return fmt.Errorf("ADMIN_MAX_UPLOAD_SIZE must be between 1 and MAX_REQUEST_SIZE (%d)", config.Security.MaxRequestSize)
	}
	if config.Admin.MaxImagePixels <= 0 {
		return fmt.Errorf("ADMIN_MAX_IMAGE_PIXELS must be positive")
	}
	if config.Captcha.IPEscalatePerMin < 0 {
		return fmt.Errorf("CAPTCHA_IP_ESCALATE_PER_MINUTE cannot be negative")
	}
	if config.Admin.IPLimitPerMin < 0 {
		return fmt.Errorf("ADMIN_IP_LIMIT_PER_MINUTE cannot be negative")
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
