package services

import (
	"encoding/json"
	"simple-captcha/cache"
	"simple-captcha/config"
	"simple-captcha/helper"
	"time"
)

// ChallengeMeta 随答案一同落库的出题上下文。
// 校验阶段据此完成客户端绑定、容差按倍率换算以及 PoW 校验。
type ChallengeMeta struct {
	Type       string `json:"type"`
	ClientHash string `json:"client,omitempty"`
	Scale      int    `json:"scale"`
	Difficulty int    `json:"difficulty"`
	PowNonce   string `json:"powNonce,omitempty"`
	PowBits    int    `json:"powBits,omitempty"`
}

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

// normalizeRequest 返回裁剪过倍率与难度的请求副本
func (bs *BaseService) normalizeRequest(req *ChallengeRequest, serviceType string) *ChallengeRequest {
	out := ChallengeRequest{}
	if req != nil {
		out = *req
	}
	if out.Type == "" {
		out.Type = serviceType
	}
	if out.Difficulty == 0 {
		out.Difficulty = bs.config.Captcha.DefaultDifficulty
	}
	out.Difficulty = ClampDifficulty(out.Difficulty)
	out.Scale = ClampScale(out.Scale, bs.config.Captcha.MaxScale)
	return &out
}

// persistChallenge 生成随机 key、组装元数据（含 PoW 挑战）并写入缓存
func (bs *BaseService) persistChallenge(req *ChallengeRequest, data interface{}) (string, *ChallengeMeta, *helper.CaptchaError) {
	key, keyErr := bs.GenerateKey()
	if keyErr != nil {
		return "", nil, keyErr
	}

	meta := &ChallengeMeta{
		Type:       req.Type,
		ClientHash: req.ClientHash,
		Scale:      req.Scale,
		Difficulty: req.Difficulty,
	}

	if bs.config.Captcha.PowEnabled {
		nonce, err := helper.GenerateSecureKey()
		if err != nil {
			return "", nil, helper.NewCaptchaError(helper.ErrCodeInternalError, "生成PoW挑战失败", err.Error())
		}
		meta.PowNonce = nonce
		meta.PowBits = bs.powBits(req.Difficulty)
	}

	if saveErr := bs.SaveCaptchaData(key, data, meta); saveErr != nil {
		return "", nil, saveErr
	}
	return key, meta, nil
}

// powBits 根据难度等级计算 PoW 前导零位数
func (bs *BaseService) powBits(level int) int {
	bits := bs.config.Captcha.PowDifficulty + GetDifficultyProfile(level).PowExtraBits
	if maxBits := bs.config.Captcha.PowMaxDifficulty; bits > maxBits {
		bits = maxBits
	}
	if bits < 1 {
		bits = 1
	}
	return bits
}

// SaveCaptchaData 保存验证码答案与出题上下文到缓存
func (bs *BaseService) SaveCaptchaData(key string, data interface{}, meta *ChallengeMeta) *helper.CaptchaError {
	if meta == nil {
		meta = &ChallengeMeta{Scale: 1, Difficulty: bs.config.Captcha.DefaultDifficulty}
	}
	now := time.Now()

	verifyData := map[string]interface{}{
		"data":         data,
		"meta":         meta,
		"createTime":   now.Unix(),
		"createTimeMs": now.UnixMilli(),
		"expireAt":     now.Add(bs.config.Captcha.CacheExpiration).Unix(),
		"maxAttempts":  bs.config.Captcha.MaxAttempts,
		"attempts":     0,
		"verified":     false,
	}

	verifyDataBytes, err := json.Marshal(verifyData)
	if err != nil {
		return helper.NewCaptchaError(helper.ErrCodeCacheError, "序列化验证数据失败", err.Error())
	}

	if err = cache.Set(key, verifyDataBytes, bs.config.Captcha.CacheExpiration); err != nil {
		return helper.NewCaptchaError(helper.ErrCodeCacheError, "保存验证码数据失败", err.Error())
	}

	return nil
}

// GetCaptchaData 从缓存获取验证码数据
func (bs *BaseService) GetCaptchaData(key string) (map[string]interface{}, *helper.CaptchaError) {
	if !cache.Exists(key) {
		return nil, helper.NewCaptchaError(helper.ErrCodeExpired, "验证码不存在或已失效")
	}

	cacheDataByte := cache.Get(key)
	if cacheDataByte == nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeCacheError, "获取验证码数据失败")
	}

	var verifyData map[string]interface{}
	if err := json.Unmarshal([]byte(*cacheDataByte), &verifyData); err != nil {
		cache.Del(key)
		return nil, helper.NewCaptchaError(helper.ErrCodeCacheError, "验证码数据格式错误")
	}

	if expireAt, ok := verifyData["expireAt"].(float64); ok {
		if time.Now().Unix() > int64(expireAt) {
			cache.Del(key)
			return nil, helper.NewCaptchaError(helper.ErrCodeExpired, "验证码已过期")
		}
	}

	return verifyData, nil
}

// ParseMeta 从缓存记录中还原出题上下文；旧记录缺少 meta 时返回 1 倍率默认值
func ParseMeta(verifyData map[string]interface{}) *ChallengeMeta {
	meta := &ChallengeMeta{Scale: 1}
	raw, ok := verifyData["meta"]
	if !ok || raw == nil {
		return meta
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return meta
	}
	_ = json.Unmarshal(b, meta)
	if meta.Scale <= 0 {
		meta.Scale = 1
	}
	return meta
}

// CreatedAt 返回题目签发时间（毫秒精度），缺失时返回零值
func CreatedAt(verifyData map[string]interface{}) time.Time {
	if ms, ok := verifyData["createTimeMs"].(float64); ok && ms > 0 {
		return time.UnixMilli(int64(ms))
	}
	if sec, ok := verifyData["createTime"].(float64); ok && sec > 0 {
		return time.Unix(int64(sec), 0)
	}
	return time.Time{}
}

// UpdateCaptchaData 更新验证码数据
func (bs *BaseService) UpdateCaptchaData(key string, verifyData map[string]interface{}) *helper.CaptchaError {
	verifyDataBytes, err := json.Marshal(verifyData)
	if err != nil {
		return helper.NewCaptchaError(helper.ErrCodeCacheError, "序列化验证数据失败", err.Error())
	}

	if err = cache.Set(key, verifyDataBytes, bs.config.Captcha.CacheExpiration); err != nil {
		return helper.NewCaptchaError(helper.ErrCodeCacheError, "更新验证码数据失败", err.Error())
	}

	return nil
}

// DeleteCaptchaData 删除验证码数据
func (bs *BaseService) DeleteCaptchaData(key string) {
	cache.Del(key)
}

// GenerateKey 生成与答案无关的随机验证码 key
func (bs *BaseService) GenerateKey() (string, *helper.CaptchaError) {
	key, err := helper.GenerateSecureKey()
	if err != nil {
		return "", helper.NewCaptchaError(helper.ErrCodeInternalError, "生成验证码key失败", err.Error())
	}
	return key, nil
}

// IncrementAttempts 增加尝试次数
func (bs *BaseService) IncrementAttempts(key string) (int, *helper.CaptchaError) {
	verifyData, err := bs.GetCaptchaData(key)
	if err != nil {
		return 0, err
	}

	attempts, ok := verifyData["attempts"].(float64)
	if !ok {
		attempts = 0
	}

	attempts++
	verifyData["attempts"] = attempts

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
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return helper.NewCaptchaError(helper.ErrCodeCacheError, "序列化二次验证数据失败", err.Error())
	}

	if err = cache.Set(key, dataBytes, bs.config.Captcha.SecondVerifyExpire); err != nil {
		return helper.NewCaptchaError(helper.ErrCodeCacheError, "保存二次验证数据失败", err.Error())
	}

	return nil
}
