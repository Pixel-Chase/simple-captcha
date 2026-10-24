package services

import (
	"context"
	"fmt"
	"log"
	"simple-captcha/cache"
	"simple-captcha/config"
	"simple-captcha/helper"
	"sync"
	"time"
)

// RiskControlServiceImpl 风险控制服务实现：IP 限流 + 二次验证令牌
type RiskControlServiceImpl struct {
	config   *config.Config
	limiter  RateLimiter        // 主限流器（Redis 滑动窗口），可能为 nil
	fallback *memoryRateLimiter // Redis 不可用时的兜底
	backend  string             // 当前限流后端名称

	secondTokens map[string]time.Time
	tokenMutex   sync.RWMutex
}

// NewRiskControlService 创建风险控制服务
func NewRiskControlService() *RiskControlServiceImpl {
	cfg := config.LoadConfig()
	service := &RiskControlServiceImpl{
		config:       cfg,
		fallback:     newMemoryRateLimiter(cfg.Captcha.IPRateLimit, cfg.Captcha.RateLimitWindow),
		backend:      "memory",
		secondTokens: make(map[string]time.Time),
	}

	if cfg.Security.RateLimitBackend == "redis" {
		service.limiter = newRedisSlidingWindowLimiter(cfg.Captcha.IPRateLimit, cfg.Captcha.RateLimitWindow)
		service.backend = "redis"
	}

	go service.startCleanupRoutine()

	return service
}

// CheckIPLimit 检查 IP 是否仍在限流额度内；Redis 不可用时自动回退到内存限流
func (r *RiskControlServiceImpl) CheckIPLimit(ip string) bool {
	if r.limiter != nil {
		allowed, err := r.limiter.Allow(ip)
		if err == nil {
			return allowed
		}
		log.Printf("Redis 限流不可用，回退内存限流: ip=%s, error=%v", ip, err)
	}

	allowed, _ := r.fallback.Allow(ip)
	return allowed
}

// ValidateSecondVerifyToken 验证二次验证令牌（一次性消费）
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

	r.tokenMutex.Lock()
	r.secondTokens[token] = expireTime
	r.tokenMutex.Unlock()

	if err := cache.Set(token, "verified", r.config.Captcha.SecondVerifyExpire); err != nil {
		return "", helper.NewCaptchaError(helper.ErrCodeCacheError, "生成二次验证令牌失败", err.Error())
	}

	return token, nil
}

// startCleanupRoutine 定期清理过期的内存数据
func (r *RiskControlServiceImpl) startCleanupRoutine() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		r.cleanup()
	}
}

// cleanup 清理过期数据
func (r *RiskControlServiceImpl) cleanup() {
	now := time.Now()

	r.fallback.cleanup(now)

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
	r.tokenMutex.RLock()
	tokenCount := len(r.secondTokens)
	r.tokenMutex.RUnlock()

	return map[string]interface{}{
		"backend":            r.backend,
		"memory_tracked_ips": r.fallback.size(),
		"active_tokens":      tokenCount,
		"rate_limit":         r.config.Captcha.IPRateLimit,
		"time_window":        r.config.Captcha.RateLimitWindow.String(),
		"token_expire":       r.config.Captcha.SecondVerifyExpire.String(),
	}
}
