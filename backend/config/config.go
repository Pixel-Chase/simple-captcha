package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// APIPrefix API 路径固定为 /api/v1，不再支持自定义前缀：验证码服务应部署在独立域名下
const APIPrefix = "/api/v1"

// Config 应用配置结构
type Config struct {
	Server     ServerConfig
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
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	Environment     string
	GracefulTimeout time.Duration
	MaxHeaderBytes  int
}

// CaptchaConfig 验证码部署参数；出题与校验策略见 RuntimeSettings（管理后台「策略配置」）
type CaptchaConfig struct {
	ImagePath string
}

// RiskConfig 风控部署参数（可信代理注入的 TLS / HTTP2 指纹）；评分策略见 RuntimeSettings
type RiskConfig struct {
	TLSFingerprintHeader   string // 可信代理注入的 TLS 指纹（JA3 / JA4）请求头名，空表示不读取
	HTTP2FingerprintHeader string // 可信代理注入的 HTTP/2 指纹请求头名，空表示不读取
	TLSFingerprintList     string // 指纹参考列表 JSON（browser 家族映射 + deny 列表）路径，空表示只记录不计分
}

// AdminConfig 管理控制台配置（管理员登录、背景图与接入应用管理），随服务常驻开启。
// 管理员用户名与密码不走配置：首次启动后在控制台初始化页面设置，账号与接入应用存于 DataDir 下的 captcha.db。
type AdminConfig struct {
	DataDir        string        // 数据目录：captcha.db（管理员账号、会话、接入应用等持久数据），固定为工作目录下的 data（镜像内 /app/data）
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
			ReadTimeout:     getDurationEnv("SERVER_READ_TIMEOUT", 30*time.Second),
			WriteTimeout:    getDurationEnv("SERVER_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:     getDurationEnv("SERVER_IDLE_TIMEOUT", 60*time.Second),
			Environment:     getEnv("GIN_MODE", "release"),
			GracefulTimeout: getDurationEnv("SERVER_GRACEFUL_TIMEOUT", 30*time.Second),
			MaxHeaderBytes:  getIntEnv("SERVER_MAX_HEADER_BYTES", 1<<20),
		},
		Captcha: CaptchaConfig{
			ImagePath: getEnv("CAPTCHA_IMAGE_PATH", "./images"),
		},
		Risk: RiskConfig{
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
			DataDir:        "./data",
			SessionTTL:     getDurationEnv("ADMIN_SESSION_TTL", 2*time.Hour),
			MaxUploadSize:  getInt64Env("ADMIN_MAX_UPLOAD_SIZE", 5<<20), // 5MB
			MaxImagePixels: getIntEnv("ADMIN_MAX_IMAGE_PIXELS", 24_000_000),
			IPLimitPerMin:  getIntEnv("ADMIN_IP_LIMIT_PER_MINUTE", 30),
		},
	}
}

// validateConfig 验证配置
func validateConfig(config *Config) error {
	if config.Server.Port == "" {
		return fmt.Errorf("server port cannot be empty")
	}
	if config.Captcha.ImagePath == "" {
		return fmt.Errorf("captcha image path cannot be empty")
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
	if config.Admin.SessionTTL <= 0 {
		return fmt.Errorf("ADMIN_SESSION_TTL must be positive")
	}
	if config.Admin.MaxUploadSize <= 0 || config.Admin.MaxUploadSize > config.Security.MaxRequestSize {
		return fmt.Errorf("ADMIN_MAX_UPLOAD_SIZE must be between 1 and MAX_REQUEST_SIZE (%d)", config.Security.MaxRequestSize)
	}
	if config.Admin.MaxImagePixels <= 0 {
		return fmt.Errorf("ADMIN_MAX_IMAGE_PIXELS must be positive")
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
