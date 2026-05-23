package services

import (
	"simple-captcha/helper"
)

// CaptchaService 验证码服务接口
type CaptchaService interface {
	// GenerateCaptcha 生成验证码
	GenerateCaptcha(mode string) (*CaptchaData, *helper.CaptchaError)

	// GetServiceType 获取服务类型
	GetServiceType() string
}

// VerifyService 验证服务接口
type VerifyService interface {
	// VerifyData 验证数据
	VerifyData(data, key string) (bool, string, *helper.CaptchaError)
}

// RiskControlService 风险控制服务接口
type RiskControlService interface {
	// CheckIPLimit 检查IP限制
	CheckIPLimit(ip string) bool

	// ValidateSecondVerifyToken 验证二次验证令牌
	ValidateSecondVerifyToken(token string) bool
}

// CacheService 缓存服务接口
type CacheService interface {
	// Set 设置缓存
	Set(key string, value interface{}, expiration int64) error

	// Get 获取缓存
	Get(key string) ([]byte, error)

	// Delete 删除缓存
	Delete(key string) error

	// Exists 检查是否存在
	Exists(key string) bool

	// Ping 检查连接状态
	Ping() error
}

// ServiceManager 服务管理器接口
type ServiceManager interface {
	// RegisterCaptchaService 注册验证码服务
	RegisterCaptchaService(serviceType string, service CaptchaService)

	// RegisterVerifyService 注册验证服务
	RegisterVerifyService(serviceType string, service VerifyService)

	// GetCaptchaService 获取验证码服务
	GetCaptchaService(serviceType string) (CaptchaService, bool)

	// GetVerifyService 获取验证服务
	GetVerifyService(serviceType string) (VerifyService, bool)

	// GetAllCaptchaServices 获取所有验证码服务
	GetAllCaptchaServices() map[string]CaptchaService

	// GetAllVerifyServices 获取所有验证服务
	GetAllVerifyServices() map[string]VerifyService

	// SetRiskControlService 设置风险控制服务
	SetRiskControlService(service RiskControlService)

	// GetRiskControlService 获取风险控制服务
	GetRiskControlService() RiskControlService

	// SetCacheService 设置缓存服务
	SetCacheService(service CacheService)

	// GetCacheService 获取缓存服务
	GetCacheService() CacheService
}

// NewServiceManager 创建服务管理器 - 类型别名
func NewServiceManager() ServiceManager {
	return NewServiceManagerImpl()
}
