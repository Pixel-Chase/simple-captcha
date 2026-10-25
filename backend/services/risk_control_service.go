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

const (
	// adminRateLimitPrefix 管理员接口的计数与验证码请求分开存放：同一出口下的验证码流量不能挤占管理员的额度
	adminRateLimitPrefix = "admin:"
	// ipCounterWindow 单 IP 计数窗口固定为 1 分钟，所有阈值都按每分钟配置
	ipCounterWindow = time.Minute
)

// RiskControlServiceImpl 风险控制服务实现：IP 计数与限流 + 二次验证令牌
type RiskControlServiceImpl struct {
	config  *config.Config
	limiter *redisSlidingWindowLimiter // 单 IP 计数，多实例共享

	secondTokens map[string]time.Time
	tokenMutex   sync.RWMutex
}

// NewRiskControlService 创建风险控制服务
func NewRiskControlService() *RiskControlServiceImpl {
	cfg := config.LoadConfig()
	service := &RiskControlServiceImpl{
		config:       cfg,
		limiter:      newRedisSlidingWindowLimiter(ipCounterWindow),
		secondTokens: make(map[string]time.Time),
	}

	go service.startCleanupRoutine()

	return service
}

// TrackCaptchaIP 记录一次来自 ip 的验证码请求，返回最近一分钟内的请求数，只用于给出题加难（见 IPPressureBoost），从不拒绝。
// 按 IP 拒绝挡不住会换 IP 的攻击者，却会让企业 NAT、运营商 CGNAT 后共用出口的正常用户一起无法使用。
// 计数达到 IPPressureSaturation 后加难已到顶，之后的请求不再写入窗口，持续高频请求也不会把计数集合撑大。
func (r *RiskControlServiceImpl) TrackCaptchaIP(ip string) int {
	count, _ := r.hit(ip, IPPressureSaturation(r.config.Captcha.IPEscalatePerMin))
	return count
}

// CheckAdminIP 管理员登录与改密的单 IP 限流，每分钟超过 ADMIN_IP_LIMIT_PER_MINUTE 即拒绝
func (r *RiskControlServiceImpl) CheckAdminIP(ip string) bool {
	_, allowed := r.hit(adminRateLimitPrefix+ip, r.config.Admin.IPLimitPerMin)
	return allowed
}

// hit 记录一次请求。Redis 出错时放行且不计数：验证码只是少加难；管理员登录也要靠 Redis 消费验证码凭证，
// Redis 不可用时本就无法登录，放行不会多出绕过路径
func (r *RiskControlServiceImpl) hit(key string, limit int) (int, bool) {
	count, allowed, err := r.limiter.Hit(key, limit)
	if err != nil {
		log.Printf("单 IP 计数失败，本次放行: key=%s, error=%v", key, err)
		return 0, true
	}
	return count, allowed
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
		"active_tokens":             tokenCount,
		"ip_escalate_per_minute":    r.config.Captcha.IPEscalatePerMin,
		"admin_ip_limit_per_minute": r.config.Admin.IPLimitPerMin,
		"token_expire":              r.config.Captcha.SecondVerifyExpire.String(),
	}
}
