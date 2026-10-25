package tests

import (
	"fmt"
	"simple-captcha/services"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestServiceManager_Initialization(t *testing.T) {
	// 测试服务管理器初始化
	manager := services.NewServiceManager()
	assert.NotNil(t, manager)

	// 验证服务是否正确注册
	captchaService, exists := manager.GetCaptchaService("click-text")
	assert.True(t, exists)
	assert.NotNil(t, captchaService)

	verifyService, exists := manager.GetVerifyService("click-text")
	assert.True(t, exists)
	assert.NotNil(t, verifyService)

	riskControl := manager.GetRiskControlService()
	assert.NotNil(t, riskControl)

	cacheService := manager.GetCacheService()
	assert.NotNil(t, cacheService)
}

func TestServiceManager_GetAllServices(t *testing.T) {
	manager := services.NewServiceManager()

	// 测试获取所有验证码服务
	captchaServices := manager.GetAllCaptchaServices()
	assert.Len(t, captchaServices, 5) // 应该有5种验证码类型

	expectedTypes := []string{"click-text", "click-shape", "rotate", "slide-text", "slide-region"}
	for _, serviceType := range expectedTypes {
		_, exists := captchaServices[serviceType]
		assert.True(t, exists, "验证码服务 %s 应该存在", serviceType)
	}

	// 测试获取所有验证服务
	verifyServices := manager.GetAllVerifyServices()
	assert.Len(t, verifyServices, 5) // 应该有5种验证服务

	for _, serviceType := range expectedTypes {
		_, exists := verifyServices[serviceType]
		assert.True(t, exists, "验证服务 %s 应该存在", serviceType)
	}
}

func TestServiceManager_ValidateServices(t *testing.T) {
	manager := services.NewServiceManager()

	// 验证服务完整性
	if managerImpl, ok := manager.(*services.ServiceManagerImpl); ok {
		err := managerImpl.ValidateServices()
		assert.Nil(t, err)
	}
}

func TestServiceManager_Statistics(t *testing.T) {
	manager := services.NewServiceManager()

	// 获取服务统计信息
	if managerImpl, ok := manager.(*services.ServiceManagerImpl); ok {
		stats := managerImpl.GetServiceStatistics()
		assert.NotNil(t, stats)

		// 验证统计信息
		assert.Equal(t, 5, stats["captcha_services_count"])
		assert.Equal(t, 5, stats["verify_services_count"])
		assert.Equal(t, true, stats["initialized"])

		// 验证服务类型列表
		captchaTypes, ok := stats["captcha_types"].([]string)
		assert.True(t, ok)
		assert.Len(t, captchaTypes, 5)

		verifyTypes, ok := stats["verify_types"].([]string)
		assert.True(t, ok)
		assert.Len(t, verifyTypes, 5)
	}
}

func TestRiskControlService_IPLimit(t *testing.T) {
	manager := services.NewServiceManager()
	riskControl := manager.GetRiskControlService()

	// 限流计数存放在 Redis 中会跨测试运行保留，使用唯一 IP 避免互相干扰
	testIP := fmt.Sprintf("198.51.100.%d-%d", time.Now().UnixNano()%250, time.Now().UnixNano())

	// 测试IP限制检查
	for i := 0; i < 30; i++ { // 默认限制是30次
		result := riskControl.CheckIPLimit(testIP)
		assert.True(t, result, "前30次请求应该被允许")
	}

	// 第31次请求应该被拒绝
	result := riskControl.CheckIPLimit(testIP)
	assert.False(t, result, "第31次请求应该被拒绝")
}

func TestRiskControlService_SecondVerifyToken(t *testing.T) {
	manager := services.NewServiceManager()
	riskControl := manager.GetRiskControlService()

	// 测试无效token
	result := riskControl.ValidateSecondVerifyToken("invalid-token")
	assert.False(t, result, "无效token应该返回false")

	// 如果是具体实现，测试token生成
	if riskImpl, ok := riskControl.(*services.RiskControlServiceImpl); ok {
		token, err := riskImpl.GenerateSecondVerifyToken("test-key")
		assert.Nil(t, err)
		assert.NotEmpty(t, token)

		// 验证生成的token
		result := riskControl.ValidateSecondVerifyToken(token)
		assert.True(t, result, "有效token应该返回true")

		result = riskControl.ValidateSecondVerifyToken(token)
		assert.False(t, result, "已消费的token不应该再次通过验证")
	}
}

func TestRiskControlService_SecondVerifyDataConsumedFromRedis(t *testing.T) {
	baseService := services.NewBaseService()
	riskControl := services.NewRiskControlService()
	token := "test-second-verify-token"

	err := baseService.SaveSecondVerifyData(token, map[string]interface{}{
		"verified": true,
	})
	assert.Nil(t, err)
	defer baseService.DeleteCaptchaData(token)

	result := riskControl.ValidateSecondVerifyToken(token)
	assert.True(t, result, "Redis中的二次验证数据首次应验证通过")

	result = riskControl.ValidateSecondVerifyToken(token)
	assert.False(t, result, "Redis中的二次验证数据消费后不应该再次通过")
}

func TestCacheService_Operations(t *testing.T) {
	manager := services.NewServiceManager()
	cacheService := manager.GetCacheService()

	testKey := "test-cache-key"
	testValue := "test-cache-value"

	// 测试设置缓存
	err := cacheService.Set(testKey, testValue, 60) // 60秒过期
	assert.NoError(t, err)

	// 测试检查存在
	exists := cacheService.Exists(testKey)
	assert.True(t, exists)

	// 测试获取缓存
	data, err := cacheService.Get(testKey)
	assert.NoError(t, err)
	assert.NotNil(t, data)

	// 测试删除缓存
	err = cacheService.Delete(testKey)
	assert.NoError(t, err)

	// 验证删除后不存在
	exists = cacheService.Exists(testKey)
	assert.False(t, exists)
}

func TestBaseService_CaptchaDataOperations(t *testing.T) {
	baseService := services.NewBaseService()
	testKey := "test-captcha-key"
	// JSON 往返后数值类型为 float64
	testData := map[string]interface{}{
		"x": float64(100),
		"y": float64(200),
	}

	// 测试保存验证码数据（nil meta 使用默认元数据）
	err := baseService.SaveCaptchaData(testKey, testData, nil)
	assert.Nil(t, err)

	// 测试获取验证码数据
	retrievedData, err := baseService.GetCaptchaData(testKey)
	assert.Nil(t, err)
	assert.NotNil(t, retrievedData)

	// 验证数据内容
	data, ok := retrievedData["data"]
	assert.True(t, ok)
	assert.Equal(t, testData, data)

	// 测试增加尝试次数
	attempts, err := baseService.IncrementAttempts(testKey)
	assert.Nil(t, err)
	assert.Equal(t, 1, attempts)

	// 测试检查最大尝试次数
	maxReached, err := baseService.IsMaxAttemptsReached(testKey)
	assert.Nil(t, err)
	assert.False(t, maxReached) // 第一次尝试，不应该达到最大值

	// 测试标记为已验证
	err = baseService.MarkAsVerified(testKey)
	assert.Nil(t, err)

	// 清理测试数据
	baseService.DeleteCaptchaData(testKey)
}

// 基准测试
func BenchmarkServiceManager_GetCaptchaService(b *testing.B) {
	manager := services.NewServiceManager()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = manager.GetCaptchaService("click-text")
	}
}

func BenchmarkRiskControlService_CheckIPLimit(b *testing.B) {
	manager := services.NewServiceManager()
	riskControl := manager.GetRiskControlService()
	testIP := fmt.Sprintf("198.51.100.b-%d", time.Now().UnixNano())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		riskControl.CheckIPLimit(testIP)
	}
}
