package services

import (
	"fmt"
	"log"
	"simple-captcha/helper"
	"sync"
)

// ServiceManagerImpl 服务管理器实现
type ServiceManagerImpl struct {
	captchaServices map[string]CaptchaService
	verifyServices  map[string]VerifyService
	riskControl     RiskControlService
	cache           CacheService
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

	// 初始化风险控制服务
	sm.riskControl = NewRiskControlService()

	// 注册验证码服务
	sm.captchaServices["click-text"] = NewClickTextCaptchaService()
	sm.captchaServices["click-shape"] = NewClickShapeCaptchaService()
	sm.captchaServices["rotate"] = NewRotateCaptchaService()
	sm.captchaServices["slide-text"] = &SlideTextService{}
	sm.captchaServices["slide-region"] = &SlideRegionService{}

	// 注册验证服务
	sm.verifyServices["click-text"] = NewClickTextVerifyService()
	sm.verifyServices["click-shape"] = NewClickShapeVerifyService()
	sm.verifyServices["rotate"] = &RotateVerifyService{}
	sm.verifyServices["slide-text"] = &SlideTextVerifyService{}
	sm.verifyServices["slide-region"] = &SlideVerifyService{}

	sm.initialized = true

	log.Println("服务管理器初始化完成")
}

// RegisterCaptchaService 注册验证码服务
func (sm *ServiceManagerImpl) RegisterCaptchaService(serviceType string, service CaptchaService) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	sm.captchaServices[serviceType] = service
	log.Printf("注册验证码服务: %s", serviceType)
}

// RegisterVerifyService 注册验证服务
func (sm *ServiceManagerImpl) RegisterVerifyService(serviceType string, service VerifyService) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	sm.verifyServices[serviceType] = service
	log.Printf("注册验证服务: %s", serviceType)
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

// SetRiskControlService 设置风险控制服务
func (sm *ServiceManagerImpl) SetRiskControlService(service RiskControlService) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	sm.riskControl = service
	log.Println("设置风险控制服务")
}

// GetRiskControlService 获取风险控制服务
func (sm *ServiceManagerImpl) GetRiskControlService() RiskControlService {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	return sm.riskControl
}

// SetCacheService 设置缓存服务
func (sm *ServiceManagerImpl) SetCacheService(service CacheService) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	sm.cache = service
	log.Println("设置缓存服务")
}

// GetCacheService 获取缓存服务
func (sm *ServiceManagerImpl) GetCacheService() CacheService {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	return sm.cache
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
	sm.riskControl = nil
	sm.cache = nil
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
