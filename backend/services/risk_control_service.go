package services

import (
	"encoding/json"
	"fmt"
	"simple-captcha/cache"
	"simple-captcha/config"
	"simple-captcha/helper"
	"time"
)

const (
	// captchaRateLimitPrefix 验证码请求的单 IP 计数
	captchaRateLimitPrefix = "captcha:"
	// adminRateLimitPrefix 管理员接口的计数与验证码请求分开存放：同一出口下的验证码流量不能挤占管理员的额度
	adminRateLimitPrefix = "admin:"
	// ipCounterWindow 单 IP 计数窗口固定为 1 分钟，所有阈值都按每分钟配置
	ipCounterWindow = time.Minute
)

// RiskControlServiceImpl 风险控制服务实现：IP 计数与限流 + 二次验证令牌，数据都在进程内存
type RiskControlServiceImpl struct {
	config *config.Config
	store  *cache.Memory
}

// NewRiskControlService 创建风险控制服务
func NewRiskControlService() *RiskControlServiceImpl {
	return &RiskControlServiceImpl{config: config.LoadConfig(), store: cache.Shared()}
}

// CaptchaRateLimitKey 验证码请求单 IP 计数在进程内存中的 key（KindRateLimit），测试中用来预填或清空窗口
func CaptchaRateLimitKey(ip string) string {
	return captchaRateLimitPrefix + ip
}

// TrackCaptchaIP 记录一次来自 ip 的验证码请求，返回最近一分钟内的请求数，只用于给出题加难（见 IPPressureBoost），从不拒绝。
// 按 IP 拒绝挡不住会换 IP 的攻击者，却会让企业 NAT、运营商 CGNAT 后共用出口的正常用户一起无法使用。
// 计数达到 IPPressureSaturation 后加难已到顶，之后的请求不再写入窗口，持续高频请求也不会把计数撑大。
func (r *RiskControlServiceImpl) TrackCaptchaIP(ip string) int {
	count, _ := r.store.Hit(CaptchaRateLimitKey(ip), ipCounterWindow, IPPressureSaturation(config.Runtime().Captcha.IPEscalatePerMinute))
	return count
}

// CheckAdminIP 管理员登录、初始化与改密的单 IP 限流，每分钟超过 ADMIN_IP_LIMIT_PER_MINUTE 即拒绝
func (r *RiskControlServiceImpl) CheckAdminIP(ip string) bool {
	_, allowed := r.store.Hit(adminRateLimitPrefix+ip, ipCounterWindow, r.config.Admin.IPLimitPerMin)
	return allowed
}

// ConsumeSecondVerifyToken 一次性消费二次验证 key：key 必须属于 appID，读取、比对与删除在同一把锁内完成，
// 并发查询只有一次成功。属于其他应用的 key 视为无效，并且不消费——一个应用无法用掉另一个应用的 key
func (r *RiskControlServiceImpl) ConsumeSecondVerifyToken(token, appID string) bool {
	if token == "" || appID == "" {
		return false
	}
	consumed := false
	r.store.Update(cache.KindChallenge, token, 0, func(old []byte, ok bool) ([]byte, bool) {
		if !ok {
			return nil, false
		}
		var record struct {
			AppID string `json:"appId"`
		}
		if json.Unmarshal(old, &record) != nil || record.AppID != appID {
			return old, true
		}
		consumed = true
		return nil, false
	})
	return consumed
}

// GenerateSecondVerifyToken 直接签发一个属于 appID 的二次验证 key（测试用，等价于该应用完成一次行为验证）
func (r *RiskControlServiceImpl) GenerateSecondVerifyToken(appID string) (string, *helper.CaptchaError) {
	token := helper.StringToMD5(fmt.Sprintf("second_%s_%d", appID, time.Now().UnixNano()))
	payload, err := json.Marshal(map[string]interface{}{"verified": true, "appId": appID})
	if err != nil {
		return "", helper.NewCaptchaError(helper.ErrCodeInternalError, "生成二次验证令牌失败", err.Error())
	}
	r.store.Set(cache.KindChallenge, token, payload, config.Runtime().Captcha.SecondVerifyExpire())
	return token, nil
}

// GetStatistics 获取统计信息
func (r *RiskControlServiceImpl) GetStatistics() map[string]interface{} {
	rt := config.Runtime()
	return map[string]interface{}{
		"challenge_entries":         r.store.Len(cache.KindChallenge),
		"ip_escalate_per_minute":    rt.Captcha.IPEscalatePerMinute,
		"admin_ip_limit_per_minute": r.config.Admin.IPLimitPerMin,
		"token_expire":              rt.Captcha.SecondVerifyExpire().String(),
	}
}
