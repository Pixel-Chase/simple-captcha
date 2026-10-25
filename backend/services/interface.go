package services

import (
	"simple-captcha/helper"
	"simple-captcha/risk"
)

// ChallengeRequest 出题请求上下文，由 Controller 组装后交给 Service。
// Service 据此选择难度、出图倍率并把客户端指纹写入题目元数据。
type ChallengeRequest struct {
	Type       string // 已解析的具体类型（auto 需在 Controller 层解析完毕）
	Mode       string // light | dark
	Scale      int    // 出图倍率 1-3，对应前端 devicePixelRatio
	Difficulty int    // 难度等级 1-5
	ClientHash string // 客户端指纹哈希，空字符串表示不绑定
}

// VerifyInput 校验输入
type VerifyInput struct {
	Key        string
	Answer     string // dots / point / angle 的原始字符串
	ClientHash string
	Pow        *risk.PowSolution
	// RiskBlocked 风控已判定拦截：模板仍照常执行全部前置检查，只在答案比对阶段强制判失败，
	// 保证拦截与否对外响应完全一致
	RiskBlocked bool
}

// CaptchaService 验证码服务接口
type CaptchaService interface {
	// GenerateCaptcha 生成验证码
	GenerateCaptcha(req *ChallengeRequest) (*CaptchaData, *helper.CaptchaError)

	// GetServiceType 获取服务类型
	GetServiceType() string
}

// VerifyService 验证服务接口
type VerifyService interface {
	// VerifyData 验证数据，成功时第二个返回值为二次验证 key，失败时为提示信息
	VerifyData(in *VerifyInput) (bool, string, *helper.CaptchaError)
}

// RiskControlService 风险控制服务接口
type RiskControlService interface {
	// TrackCaptchaIP 记录一次验证码请求，返回该 IP 在计数窗口内的请求数（用于加难，不拒绝）
	TrackCaptchaIP(ip string) int

	// CheckAdminIP 检查管理员登录 / 改密接口的单 IP 限流
	CheckAdminIP(ip string) bool

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
	// GetCaptchaService 获取验证码服务
	GetCaptchaService(serviceType string) (CaptchaService, bool)

	// GetVerifyService 获取验证服务
	GetVerifyService(serviceType string) (VerifyService, bool)

	// GetAllCaptchaServices 获取所有验证码服务
	GetAllCaptchaServices() map[string]CaptchaService

	// GetAllVerifyServices 获取所有验证服务
	GetAllVerifyServices() map[string]VerifyService

	// GetRiskControlService 获取风险控制服务
	GetRiskControlService() RiskControlService

	// GetCacheService 获取缓存服务
	GetCacheService() CacheService

	// GetRiskEngine 获取风控评分引擎
	GetRiskEngine() *RiskEngine

	// GetChallengePolicy 获取出题策略（难度分级、auto 类型选择）
	GetChallengePolicy() *ChallengePolicy
}

// NewServiceManager 创建服务管理器 - 类型别名
func NewServiceManager() ServiceManager {
	return NewServiceManagerImpl()
}
