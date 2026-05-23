package services

import (
	"context"
	"fmt"
	"simple-captcha/cache"
	"simple-captcha/config"
	"simple-captcha/helper"
	"sync"
	"time"
)

// RiskControlServiceImpl 风险控制服务实现
type RiskControlServiceImpl struct {
	config          *config.Config
	ipRequestCounts map[string]*IPRequestInfo
	mutex           sync.RWMutex
	secondTokens    map[string]time.Time
	tokenMutex      sync.RWMutex
}

// IPRequestInfo IP请求信息
type IPRequestInfo struct {
	Count     int
	FirstTime time.Time
	LastTime  time.Time
}

// NewRiskControlService 创建风险控制服务
func NewRiskControlService() *RiskControlServiceImpl {
	service := &RiskControlServiceImpl{
		config:          config.LoadConfig(),
		ipRequestCounts: make(map[string]*IPRequestInfo),
		secondTokens:    make(map[string]time.Time),
	}

	// 启动清理协程
	go service.startCleanupRoutine()

	return service
}

// CheckIPLimit 检查IP限制（优化版）
func (r *RiskControlServiceImpl) CheckIPLimit(ip string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	now := time.Now()
	info, exists := r.ipRequestCounts[ip]

	if !exists {
		// 首次请求
		r.ipRequestCounts[ip] = &IPRequestInfo{
			Count:     1,
			FirstTime: now,
			LastTime:  now,
		}
		return true
	}

	// 检查时间窗口
	if now.Sub(info.FirstTime) > r.config.Captcha.RateLimitWindow {
		// 重置计数器
		info.Count = 1
		info.FirstTime = now
		info.LastTime = now
		return true
	}

	// 智能限流策略
	limit := r.calculateDynamicLimit(info, now)

	// 检查是否超过限制
	if info.Count >= limit {
		return false
	}

	// 增加计数
	info.Count++
	info.LastTime = now

	return true
}

// calculateDynamicLimit 计算动态限制
func (r *RiskControlServiceImpl) calculateDynamicLimit(info *IPRequestInfo, now time.Time) int {
	baseLimit := r.config.Captcha.IPRateLimit

	// 基于请求频率的动态调整
	timeSpan := now.Sub(info.FirstTime)
	if timeSpan > 0 {
		requestRate := float64(info.Count) / timeSpan.Minutes()

		// 如果请求过于频繁（超过基础限制的80%），降低限制
		if requestRate > float64(baseLimit)*0.8 {
			return int(float64(baseLimit) * 0.7) // 降低30%
		}

		// 如果请求很分散，可以稍微放宽限制
		if requestRate < float64(baseLimit)*0.3 {
			return int(float64(baseLimit) * 1.2) // 增加20%
		}
	}

	return baseLimit
}

// ValidateSecondVerifyToken 验证二次验证令牌
func (r *RiskControlServiceImpl) ValidateSecondVerifyToken(token string) bool {
	if token == "" {
		return false
	}

	// 内存 token 采用一次性消费语义，验证成功后立即删除。
	r.tokenMutex.Lock()
	if expireTime, exists := r.secondTokens[token]; exists {
		if time.Now().Before(expireTime) {
			delete(r.secondTokens, token)
			deleted, err := cache.GetRedisClient().Del(context.Background(), token).Result()
			r.tokenMutex.Unlock()
			return err == nil && deleted > 0
		}
		// 过期则删除
		delete(r.secondTokens, token)
		r.tokenMutex.Unlock()
		return false
	}
	r.tokenMutex.Unlock()

	// Redis 中的二次验证 key 也必须一次性消费，避免同一个 second_key 被重复使用。
	deleted, err := cache.GetRedisClient().Del(context.Background(), token).Result()
	return err == nil && deleted > 0
}

// GenerateSecondVerifyToken 生成二次验证令牌
func (r *RiskControlServiceImpl) GenerateSecondVerifyToken(originalKey string) (string, *helper.CaptchaError) {
	token := helper.StringToMD5(fmt.Sprintf("second_%s_%d", originalKey, time.Now().UnixNano()))
	expireTime := time.Now().Add(r.config.Captcha.SecondVerifyExpire)

	// 存储到内存
	r.tokenMutex.Lock()
	r.secondTokens[token] = expireTime
	r.tokenMutex.Unlock()

	// 存储到缓存
	err := cache.Set(token, "verified", r.config.Captcha.SecondVerifyExpire)
	if err != nil {
		return "", helper.NewCaptchaError(helper.ErrCodeCacheError, "生成二次验证令牌失败", err.Error())
	}

	return token, nil
}

// ResetIPLimit 重置IP限制
func (r *RiskControlServiceImpl) ResetIPLimit(ip string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	delete(r.ipRequestCounts, ip)
}

// GetIPRequestInfo 获取IP请求信息
func (r *RiskControlServiceImpl) GetIPRequestInfo(ip string) *IPRequestInfo {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if info, exists := r.ipRequestCounts[ip]; exists {
		// 返回副本，避免并发问题
		return &IPRequestInfo{
			Count:     info.Count,
			FirstTime: info.FirstTime,
			LastTime:  info.LastTime,
		}
	}

	return nil
}

// GetAllIPRequestInfo 获取所有IP请求信息
func (r *RiskControlServiceImpl) GetAllIPRequestInfo() map[string]*IPRequestInfo {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	result := make(map[string]*IPRequestInfo)
	for ip, info := range r.ipRequestCounts {
		result[ip] = &IPRequestInfo{
			Count:     info.Count,
			FirstTime: info.FirstTime,
			LastTime:  info.LastTime,
		}
	}

	return result
}

// startCleanupRoutine 启动清理协程
func (r *RiskControlServiceImpl) startCleanupRoutine() {
	ticker := time.NewTicker(5 * time.Minute) // 每5分钟清理一次
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.cleanup()
		}
	}
}

// cleanup 清理过期数据
func (r *RiskControlServiceImpl) cleanup() {
	now := time.Now()

	// 清理IP请求计数
	r.mutex.Lock()
	for ip, info := range r.ipRequestCounts {
		if now.Sub(info.LastTime) > r.config.Captcha.RateLimitWindow*2 {
			delete(r.ipRequestCounts, ip)
		}
	}
	r.mutex.Unlock()

	// 清理二次验证令牌
	r.tokenMutex.Lock()
	for token, expireTime := range r.secondTokens {
		if now.After(expireTime) {
			delete(r.secondTokens, token)
		}
	}
	r.tokenMutex.Unlock()
}

// GetStatistics 获取统计信息
func (r *RiskControlServiceImpl) GetStatistics() map[string]interface{} {
	r.mutex.RLock()
	ipCount := len(r.ipRequestCounts)
	r.mutex.RUnlock()

	r.tokenMutex.RLock()
	tokenCount := len(r.secondTokens)
	r.tokenMutex.RUnlock()

	return map[string]interface{}{
		"active_ips":    ipCount,
		"active_tokens": tokenCount,
		"rate_limit":    r.config.Captcha.IPRateLimit,
		"time_window":   r.config.Captcha.RateLimitWindow.String(),
		"token_expire":  r.config.Captcha.SecondVerifyExpire.String(),
	}
}

// GetIPLimitStatus 获取IP限制状态
func (r *RiskControlServiceImpl) GetIPLimitStatus(ip string) map[string]interface{} {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	info, exists := r.ipRequestCounts[ip]
	if !exists {
		return map[string]interface{}{
			"ip":         ip,
			"count":      0,
			"limit":      r.config.Captcha.IPRateLimit,
			"remaining":  r.config.Captcha.IPRateLimit,
			"reset_time": nil,
			"blocked":    false,
		}
	}

	now := time.Now()
	limit := r.calculateDynamicLimit(info, now)
	remaining := limit - info.Count
	if remaining < 0 {
		remaining = 0
	}

	resetTime := info.FirstTime.Add(r.config.Captcha.RateLimitWindow)

	return map[string]interface{}{
		"ip":            ip,
		"count":         info.Count,
		"limit":         limit,
		"remaining":     remaining,
		"reset_time":    resetTime,
		"blocked":       info.Count >= limit,
		"first_request": info.FirstTime,
		"last_request":  info.LastTime,
		"window_left":   resetTime.Sub(now),
	}
}
