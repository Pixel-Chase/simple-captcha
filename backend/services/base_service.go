package services

import (
	"encoding/json"
	"simple-captcha/cache"
	"simple-captcha/config"
	"simple-captcha/helper"
	"time"
)

// BaseService 基础服务类
type BaseService struct {
	config *config.Config
}

// NewBaseService 创建基础服务
func NewBaseService() *BaseService {
	return &BaseService{
		config: config.LoadConfig(),
	}
}

// SaveCaptchaData 保存验证码数据到缓存
func (bs *BaseService) SaveCaptchaData(key string, data interface{}) *helper.CaptchaError {
	// 创建包含多次尝试逻辑的验证数据结构
	verifyData := map[string]interface{}{
		"data":        data,
		"createTime":  time.Now().Unix(),
		"expireAt":    time.Now().Add(bs.config.Captcha.CacheExpiration).Unix(),
		"maxAttempts": bs.config.Captcha.MaxAttempts,
		"attempts":    0,
		"verified":    false,
	}

	// 将验证数据结构序列化为JSON
	verifyDataBytes, err := json.Marshal(verifyData)
	if err != nil {
		return helper.NewCaptchaError(helper.ErrCodeCacheError, "序列化验证数据失败", err.Error())
	}

	// 将数据写入缓存
	err = cache.Set(key, verifyDataBytes, bs.config.Captcha.CacheExpiration)
	if err != nil {
		return helper.NewCaptchaError(helper.ErrCodeCacheError, "保存验证码数据失败", err.Error())
	}

	return nil
}

// GetCaptchaData 从缓存获取验证码数据
func (bs *BaseService) GetCaptchaData(key string) (map[string]interface{}, *helper.CaptchaError) {
	// 检查缓存中是否存在
	if !cache.Exists(key) {
		return nil, helper.NewCaptchaError(helper.ErrCodeExpired, "验证码不存在或已失效")
	}

	// 获取缓存数据
	cacheDataByte := cache.Get(key)
	if cacheDataByte == nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeCacheError, "获取验证码数据失败")
	}

	// 解析验证数据
	var verifyData map[string]interface{}
	if err := json.Unmarshal([]byte(*cacheDataByte), &verifyData); err != nil {
		// 清理无效数据
		cache.Del(key)
		return nil, helper.NewCaptchaError(helper.ErrCodeCacheError, "验证码数据格式错误")
	}

	// 检查是否已过期
	if expireAt, ok := verifyData["expireAt"].(float64); ok {
		if time.Now().Unix() > int64(expireAt) {
			cache.Del(key) // 清理过期数据
			return nil, helper.NewCaptchaError(helper.ErrCodeExpired, "验证码已过期")
		}
	}

	return verifyData, nil
}

// UpdateCaptchaData 更新验证码数据
func (bs *BaseService) UpdateCaptchaData(key string, verifyData map[string]interface{}) *helper.CaptchaError {
	// 序列化数据
	verifyDataBytes, err := json.Marshal(verifyData)
	if err != nil {
		return helper.NewCaptchaError(helper.ErrCodeCacheError, "序列化验证数据失败", err.Error())
	}

	// 更新缓存
	err = cache.Set(key, verifyDataBytes, bs.config.Captcha.CacheExpiration)
	if err != nil {
		return helper.NewCaptchaError(helper.ErrCodeCacheError, "更新验证码数据失败", err.Error())
	}

	return nil
}

// DeleteCaptchaData 删除验证码数据
func (bs *BaseService) DeleteCaptchaData(key string) {
	cache.Del(key)
}

// GenerateKey 生成验证码key
func (bs *BaseService) GenerateKey(data interface{}) (string, *helper.CaptchaError) {
	// 将数据转换为JSON格式并生成MD5哈希值作为键
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return "", helper.NewCaptchaError(helper.ErrCodeInternalError, "生成验证码key失败", err.Error())
	}

	key := helper.StringToMD5(string(dataBytes))
	return key, nil
}

// IncrementAttempts 增加尝试次数
func (bs *BaseService) IncrementAttempts(key string) (int, *helper.CaptchaError) {
	verifyData, err := bs.GetCaptchaData(key)
	if err != nil {
		return 0, err
	}

	// 获取当前尝试次数
	attempts, ok := verifyData["attempts"].(float64)
	if !ok {
		attempts = 0
	}

	// 增加尝试次数
	attempts++
	verifyData["attempts"] = attempts

	// 更新缓存
	if updateErr := bs.UpdateCaptchaData(key, verifyData); updateErr != nil {
		return int(attempts), updateErr
	}

	return int(attempts), nil
}

// IsMaxAttemptsReached 检查是否达到最大尝试次数
func (bs *BaseService) IsMaxAttemptsReached(key string) (bool, *helper.CaptchaError) {
	verifyData, err := bs.GetCaptchaData(key)
	if err != nil {
		return true, err
	}

	attempts, ok := verifyData["attempts"].(float64)
	if !ok {
		attempts = 0
	}

	maxAttempts, ok := verifyData["maxAttempts"].(float64)
	if !ok {
		maxAttempts = float64(bs.config.Captcha.MaxAttempts)
	}

	return attempts >= maxAttempts, nil
}

// MarkAsVerified 标记为已验证
func (bs *BaseService) MarkAsVerified(key string) *helper.CaptchaError {
	verifyData, err := bs.GetCaptchaData(key)
	if err != nil {
		return err
	}

	verifyData["verified"] = true
	verifyData["verifyTime"] = time.Now().Unix()

	return bs.UpdateCaptchaData(key, verifyData)
}

// SaveSecondVerifyData 保存二次验证数据到缓存
func (bs *BaseService) SaveSecondVerifyData(key string, data map[string]interface{}) *helper.CaptchaError {
	// 将二次验证数据序列化为JSON
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return helper.NewCaptchaError(helper.ErrCodeCacheError, "序列化二次验证数据失败", err.Error())
	}

	// 将数据写入缓存，过期时间15分钟
	err = cache.Set(key, dataBytes, 15*time.Minute)
	if err != nil {
		return helper.NewCaptchaError(helper.ErrCodeCacheError, "保存二次验证数据失败", err.Error())
	}

	return nil
}
