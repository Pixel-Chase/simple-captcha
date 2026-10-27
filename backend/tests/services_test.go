package tests

import (
	"fmt"
	"simple-captcha/config"
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

	assert.NotNil(t, manager.GetClientRiskStore())
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
	rt := config.Runtime()
	manager := services.NewServiceManager()
	riskControl := manager.GetRiskControlService()

	// 限流计数存放在进程共用的内存存储中，使用唯一 IP 避免与其他用例互相干扰
	testIP := fmt.Sprintf("198.51.100.%d-%d", time.Now().UnixNano()%250, time.Now().UnixNano())

	// 验证码请求只计数不拒绝：计数逐次累加，直到加难封顶
	saturation := services.IPPressureSaturation(rt.Captcha.IPEscalatePerMinute)
	for i := 1; i <= saturation; i++ {
		if !assert.Equal(t, i, riskControl.TrackCaptchaIP(testIP)) {
			return
		}
	}

	// 封顶后不再写入窗口，计数保持不变
	for i := 0; i < 10; i++ {
		assert.Equal(t, saturation, riskControl.TrackCaptchaIP(testIP))
	}
}

func TestRiskControlService_SecondVerifyToken(t *testing.T) {
	manager := services.NewServiceManager()
	riskControl := manager.GetRiskControlService()

	// 测试无效token
	result := riskControl.ConsumeSecondVerifyToken("invalid-token", "app_aaaaaaaaaaaa")
	assert.False(t, result, "无效token应该返回false")

	// 如果是具体实现，测试token生成
	if riskImpl, ok := riskControl.(*services.RiskControlServiceImpl); ok {
		token, err := riskImpl.GenerateSecondVerifyToken("app_aaaaaaaaaaaa")
		assert.Nil(t, err)
		assert.NotEmpty(t, token)

		// 属于其他应用：无效且不消费
		assert.False(t, riskControl.ConsumeSecondVerifyToken(token, "app_bbbbbbbbbbbb"))
		assert.False(t, riskControl.ConsumeSecondVerifyToken(token, ""))

		// 本应用：首次有效，之后无效
		result := riskControl.ConsumeSecondVerifyToken(token, "app_aaaaaaaaaaaa")
		assert.True(t, result, "有效token应该返回true")

		result = riskControl.ConsumeSecondVerifyToken(token, "app_aaaaaaaaaaaa")
		assert.False(t, result, "已消费的token不应该再次通过验证")
	}
}

func TestRiskControlService_SecondVerifyDataConsumedOnce(t *testing.T) {
	baseService := services.NewBaseService()
	riskControl := services.NewRiskControlService()
	token := "test-second-verify-token"

	err := baseService.SaveSecondVerifyData(token, map[string]interface{}{
		"verified": true,
		"appId":    "app_aaaaaaaaaaaa",
	})
	assert.Nil(t, err)
	defer baseService.DeleteCaptchaData(token)

	result := riskControl.ConsumeSecondVerifyToken(token, "app_aaaaaaaaaaaa")
	assert.True(t, result, "二次验证令牌首次应验证通过")

	result = riskControl.ConsumeSecondVerifyToken(token, "app_aaaaaaaaaaaa")
	assert.False(t, result, "二次验证令牌消费后不应该再次通过")
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

func BenchmarkRiskControlService_TrackCaptchaIP(b *testing.B) {
	manager := services.NewServiceManager()
	riskControl := manager.GetRiskControlService()
	testIP := fmt.Sprintf("198.51.100.b-%d", time.Now().UnixNano())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		riskControl.TrackCaptchaIP(testIP)
	}
}
