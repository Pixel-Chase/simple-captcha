package services

import (
	"fmt"
	"log"
	"simple-captcha/config"
	"simple-captcha/helper"
	"simple-captcha/risk"
	"sync"
)

// ServiceManagerImpl 服务管理器实现
type ServiceManagerImpl struct {
	captchaServices map[string]CaptchaService
	verifyServices  map[string]VerifyService
	backgrounds     *BackgroundStore
	admin           *AdminService
	apps            *AppService
	riskControl     RiskControlService
	cache           CacheService
	clientRisk      *ClientRiskStore
	riskEngine      *RiskEngine
	policy          *ChallengePolicy
	tlsFingerprints *risk.FingerprintList
	mutex           sync.RWMutex
	initialized     bool
}

// NewServiceManagerImpl 创建服务管理器实现
func NewServiceManagerImpl() *ServiceManagerImpl {
	manager := &ServiceManagerImpl{
		captchaServices: make(map[string]CaptchaService),
		verifyServices:  make(map[string]VerifyService),
	}

	// 自动初始化默认服务
	manager.initializeDefaultServices()

	return manager
}

// initializeDefaultServices 初始化默认服务
func (sm *ServiceManagerImpl) initializeDefaultServices() {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if sm.initialized {
		return
	}

	// 初始化缓存服务
	sm.cache = NewRedisCacheService()

	// 初始化风险控制服务（IP 限流 + 二次验证令牌）
	sm.riskControl = NewRiskControlService()

	// 初始化客户端风控状态、评分引擎与出题策略
	sm.clientRisk = NewClientRiskStore()
	sm.riskEngine = NewRiskEngine(sm.clientRisk)
	sm.policy = NewChallengePolicy(sm.clientRisk)

	cfg := config.LoadConfig()

	// TLS / HTTP2 指纹参考列表（可选）：配置校验阶段已确认文件可读且为合法 JSON
	if path := cfg.Risk.TLSFingerprintList; path != "" {
		list, err := risk.LoadFingerprintList(path)
		if err != nil {
			log.Fatalln("加载 TLS 指纹列表失败:", err)
		}
		sm.tlsFingerprints = list
		log.Printf("TLS 指纹列表已加载: path=%s, browser=%d, deny=%d", path, len(list.Browser), len(list.Deny))
	}

	// 背景图仓库：全部题型共用一份解码后的素材，管理接口增删素材后可热重载
	backgrounds, err := NewBackgroundStore(helper.ResolveImageDir(cfg.Captcha.ImagePath))
	if err != nil {
		log.Fatalln("加载背景图失败:", err)
	}
	sm.backgrounds = backgrounds

	// 注册验证码服务
	sm.captchaServices["click-text"] = NewClickTextCaptchaService(backgrounds)
	sm.captchaServices["click-shape"] = NewClickShapeCaptchaService(backgrounds)
	sm.captchaServices["rotate"] = NewRotateCaptchaService(backgrounds)
	sm.captchaServices["slide-text"] = NewSlideTextCaptchaService(backgrounds)
	sm.captchaServices["slide-region"] = NewSlideRegionCaptchaService(backgrounds)

	// 管理后台：管理员凭据与接入应用持久化在工作目录下的 data；登录前置验证码由风控服务消费，素材增删作用于同一个仓库
	admin, err := NewAdminService(cfg.Admin, backgrounds, sm.riskControl)
	if err != nil {
		log.Fatalln("初始化管理后台失败:", err)
	}
	sm.admin = admin
	apps, err := NewAppService(cfg.Admin.DataDir)
	if err != nil {
		log.Fatalln("加载接入应用失败:", err)
	}
	sm.apps = apps

	// 注册验证服务
	sm.verifyServices["click-text"] = NewClickTextVerifyService()
	sm.verifyServices["click-shape"] = NewClickShapeVerifyService()
	sm.verifyServices["rotate"] = NewRotateVerifyServiceImpl()
	sm.verifyServices["slide-text"] = NewSlideTextVerifyServiceImpl()
	sm.verifyServices["slide-region"] = NewSlideRegionVerifyServiceImpl()

	sm.initialized = true

	log.Println("服务管理器初始化完成")
}

// GetCaptchaService 获取验证码服务
func (sm *ServiceManagerImpl) GetCaptchaService(serviceType string) (CaptchaService, bool) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	service, exists := sm.captchaServices[serviceType]
	return service, exists
}

// GetVerifyService 获取验证服务
func (sm *ServiceManagerImpl) GetVerifyService(serviceType string) (VerifyService, bool) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	service, exists := sm.verifyServices[serviceType]
	return service, exists
}

// GetRiskControlService 获取风险控制服务
func (sm *ServiceManagerImpl) GetRiskControlService() RiskControlService {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	return sm.riskControl
}

// GetCacheService 获取缓存服务
func (sm *ServiceManagerImpl) GetCacheService() CacheService {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	return sm.cache
}

// GetRiskEngine 获取风控评分引擎
func (sm *ServiceManagerImpl) GetRiskEngine() *RiskEngine {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	return sm.riskEngine
}

// GetChallengePolicy 获取出题策略
func (sm *ServiceManagerImpl) GetChallengePolicy() *ChallengePolicy {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	return sm.policy
}

// GetClientRiskStore 获取客户端风控状态存储
func (sm *ServiceManagerImpl) GetClientRiskStore() *ClientRiskStore {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	return sm.clientRisk
}

// GetTLSFingerprintList 获取 TLS / HTTP2 指纹参考列表，未配置时为 nil
func (sm *ServiceManagerImpl) GetTLSFingerprintList() *risk.FingerprintList {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	return sm.tlsFingerprints
}

// SetTLSFingerprintList 替换指纹参考列表
func (sm *ServiceManagerImpl) SetTLSFingerprintList(list *risk.FingerprintList) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	sm.tlsFingerprints = list
}

// GetBackgroundStore 获取全部题型共用的背景图仓库
func (sm *ServiceManagerImpl) GetBackgroundStore() *BackgroundStore {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	return sm.backgrounds
}

// GetAdminService 获取管理后台服务
func (sm *ServiceManagerImpl) GetAdminService() *AdminService {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	return sm.admin
}

// GetAppService 获取接入应用服务（S2S 接口鉴权）
func (sm *ServiceManagerImpl) GetAppService() *AppService {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	return sm.apps
}

// GetAllCaptchaServices 获取所有验证码服务
func (sm *ServiceManagerImpl) GetAllCaptchaServices() map[string]CaptchaService {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	result := make(map[string]CaptchaService)
	for k, v := range sm.captchaServices {
		result[k] = v
	}
	return result
}

// GetAllVerifyServices 获取所有验证服务
func (sm *ServiceManagerImpl) GetAllVerifyServices() map[string]VerifyService {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	result := make(map[string]VerifyService)
	for k, v := range sm.verifyServices {
		result[k] = v
	}
	return result
}

// ValidateServices 验证所有服务是否正常
func (sm *ServiceManagerImpl) ValidateServices() *helper.CaptchaError {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	// 检查必要的服务是否存在
	if sm.riskControl == nil {
		return helper.NewCaptchaError(helper.ErrCodeInternalError, "风险控制服务未初始化")
	}

	if sm.cache == nil {
		return helper.NewCaptchaError(helper.ErrCodeInternalError, "缓存服务未初始化")
	}

	if sm.admin == nil || sm.apps == nil {
		return helper.NewCaptchaError(helper.ErrCodeInternalError, "管理后台服务未初始化")
	}

	// 检查验证码服务
	requiredCaptchaServices := []string{"click-text", "click-shape", "rotate", "slide-text", "slide-region"}
	for _, serviceType := range requiredCaptchaServices {
		if _, exists := sm.captchaServices[serviceType]; !exists {
			return helper.NewCaptchaError(helper.ErrCodeInternalError, fmt.Sprintf("验证码服务 %s 未注册", serviceType))
		}
	}

	// 检查验证服务
	requiredVerifyServices := []string{"click-text", "click-shape", "rotate", "slide-text", "slide-region"}
	for _, serviceType := range requiredVerifyServices {
		if _, exists := sm.verifyServices[serviceType]; !exists {
			return helper.NewCaptchaError(helper.ErrCodeInternalError, fmt.Sprintf("验证服务 %s 未注册", serviceType))
		}
	}

	return nil
}

// GetServiceStatistics 获取服务统计信息
func (sm *ServiceManagerImpl) GetServiceStatistics() map[string]interface{} {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	stats := map[string]interface{}{
		"captcha_services_count": len(sm.captchaServices),
		"verify_services_count":  len(sm.verifyServices),
		"initialized":            sm.initialized,
	}

	// 添加风险控制统计
	if riskService, ok := sm.riskControl.(*RiskControlServiceImpl); ok {
		stats["risk_control"] = riskService.GetStatistics()
	}

	// 背景素材与接入应用数量
	if sm.backgrounds != nil {
		stats["background_count"] = sm.backgrounds.Count()
	}
	if sm.apps != nil {
		stats["app_count"] = sm.apps.Count()
	}

	// 添加服务列表
	captchaTypes := make([]string, 0, len(sm.captchaServices))
	for serviceType := range sm.captchaServices {
		captchaTypes = append(captchaTypes, serviceType)
	}
	stats["captcha_types"] = captchaTypes

	verifyTypes := make([]string, 0, len(sm.verifyServices))
	for serviceType := range sm.verifyServices {
		verifyTypes = append(verifyTypes, serviceType)
	}
	stats["verify_types"] = verifyTypes

	return stats
}

// Shutdown 关闭服务管理器
func (sm *ServiceManagerImpl) Shutdown() {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	log.Println("正在关闭服务管理器...")

	// 清理资源
	sm.captchaServices = make(map[string]CaptchaService)
	sm.verifyServices = make(map[string]VerifyService)
	sm.backgrounds = nil
	sm.admin = nil
	sm.apps = nil
	sm.riskControl = nil
	sm.cache = nil
	sm.clientRisk = nil
	sm.riskEngine = nil
	sm.policy = nil
	sm.initialized = false

	log.Println("服务管理器已关闭")
}

// 全局服务管理器实例
var (
	globalServiceManager *ServiceManagerImpl
	serviceManagerOnce   sync.Once
)

// GetServiceManager 获取全局服务管理器实例
func GetServiceManager() *ServiceManagerImpl {
	serviceManagerOnce.Do(func() {
		globalServiceManager = NewServiceManagerImpl()
	})
	return globalServiceManager
}

// GetStats 获取服务统计信息（兼容接口）
func (sm *ServiceManagerImpl) GetStats() map[string]interface{} {
	return sm.GetServiceStatistics()
}
