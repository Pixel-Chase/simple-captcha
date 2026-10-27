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
	// AppID 出题时所属的应用：提交必须来自同一应用，签发的二次验证 key 也只能被它查询
	AppID string `json:"appId,omitempty"`
}

// BaseService 基础服务类：题目数据的存取；策略值每次使用时从 config.Runtime() 读取
type BaseService struct{}

// NewBaseService 创建基础服务
func NewBaseService() *BaseService {
	return &BaseService{}
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
	rt := config.Runtime()
	if out.Difficulty == 0 {
		out.Difficulty = rt.Captcha.DefaultDifficulty
	}
	out.Difficulty = ClampDifficulty(out.Difficulty)
	out.Scale = ClampScale(out.Scale, rt.Captcha.MaxScale)
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
		AppID:      req.AppID,
	}

	rt := config.Runtime()
	if rt.Pow.Enabled {
		nonce, err := helper.GenerateSecureKey()
		if err != nil {
			return "", nil, helper.NewCaptchaError(helper.ErrCodeInternalError, "生成PoW挑战失败", err.Error())
		}
		meta.PowNonce = nonce
		meta.PowBits = powBits(rt.Pow, req.Difficulty)
	}

	if saveErr := bs.saveCaptchaData(rt, key, data, meta); saveErr != nil {
		return "", nil, saveErr
	}
	return key, meta, nil
}

// powBits 根据难度等级计算 PoW 前导零位数
func powBits(pow config.PowSettings, level int) int {
	bits := pow.Difficulty + GetDifficultyProfile(level).PowExtraBits
	if maxBits := pow.MaxDifficulty; bits > maxBits {
		bits = maxBits
	}
	if bits < 1 {
		bits = 1
	}
	return bits
}

// SaveCaptchaData 保存验证码答案与出题上下文到进程内存
func (bs *BaseService) SaveCaptchaData(key string, data interface{}, meta *ChallengeMeta) *helper.CaptchaError {
	return bs.saveCaptchaData(config.Runtime(), key, data, meta)
}

func (bs *BaseService) saveCaptchaData(rt *config.RuntimeSettings, key string, data interface{}, meta *ChallengeMeta) *helper.CaptchaError {
	if meta == nil {
		meta = &ChallengeMeta{Scale: 1, Difficulty: rt.Captcha.DefaultDifficulty}
	}
	now := time.Now()

	verifyData := map[string]interface{}{
		"data":         data,
		"meta":         meta,
		"createTime":   now.Unix(),
		"createTimeMs": now.UnixMilli(),
		"expireAt":     now.Add(rt.Captcha.Expire()).Unix(),
		"maxAttempts":  rt.Captcha.MaxAttempts,
		"attempts":     0,
		"verified":     false,
	}

	verifyDataBytes, err := json.Marshal(verifyData)
	if err != nil {
		return helper.NewCaptchaError(helper.ErrCodeCaptchaDataError, "序列化验证数据失败", err.Error())
	}

	cache.Shared().Set(cache.KindChallenge, key, verifyDataBytes, rt.Captcha.Expire())
	return nil
}

// GetCaptchaData 从进程内存获取验证码数据
func (bs *BaseService) GetCaptchaData(key string) (map[string]interface{}, *helper.CaptchaError) {
	raw, ok := cache.Shared().Get(cache.KindChallenge, key)
	if !ok {
		return nil, helper.NewCaptchaError(helper.ErrCodeExpired, "验证码不存在或已失效")
	}

	var verifyData map[string]interface{}
	if err := json.Unmarshal(raw, &verifyData); err != nil {
		cache.Shared().Del(cache.KindChallenge, key)
		return nil, helper.NewCaptchaError(helper.ErrCodeCaptchaDataError, "验证码数据格式错误")
	}

	if expireAt, ok := verifyData["expireAt"].(float64); ok {
		if time.Now().Unix() > int64(expireAt) {
			cache.Shared().Del(cache.KindChallenge, key)
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

// UpdateCaptchaData 更新验证码数据：过期时间沿用出题时的设定；题目已失效时不会重新写入
func (bs *BaseService) UpdateCaptchaData(key string, verifyData map[string]interface{}) *helper.CaptchaError {
	verifyDataBytes, err := json.Marshal(verifyData)
	if err != nil {
		return helper.NewCaptchaError(helper.ErrCodeCaptchaDataError, "序列化验证数据失败", err.Error())
	}

	existed := false
	// 条目不存在时不会重新写入，ttl 参数不会生效
	cache.Shared().Update(cache.KindChallenge, key, 0, func(_ []byte, ok bool) ([]byte, bool) {
		existed = ok
		return verifyDataBytes, ok
	})
	if !existed {
		return helper.NewCaptchaError(helper.ErrCodeExpired, "验证码不存在或已失效")
	}
	return nil
}

// DeleteCaptchaData 删除验证码数据
func (bs *BaseService) DeleteCaptchaData(key string) {
	cache.Shared().Del(cache.KindChallenge, key)
}

// GenerateKey 生成与答案无关的随机验证码 key
func (bs *BaseService) GenerateKey() (string, *helper.CaptchaError) {
	key, err := helper.GenerateSecureKey()
	if err != nil {
		return "", helper.NewCaptchaError(helper.ErrCodeInternalError, "生成验证码key失败", err.Error())
	}
	return key, nil
}

// IncrementAttempts 原子地把尝试次数加一：并发提交同一道题时每次都会被计入，不会因读改写交错而少算
func (bs *BaseService) IncrementAttempts(key string) (int, *helper.CaptchaError) {
	attempts := 0
	var cerr *helper.CaptchaError
	cache.Shared().Update(cache.KindChallenge, key, 0, func(old []byte, ok bool) ([]byte, bool) {
		if !ok {
			cerr = helper.NewCaptchaError(helper.ErrCodeExpired, "验证码不存在或已失效")
			return nil, false
		}
		var verifyData map[string]interface{}
		if err := json.Unmarshal(old, &verifyData); err != nil {
			cerr = helper.NewCaptchaError(helper.ErrCodeCaptchaDataError, "验证码数据格式错误")
			return nil, false
		}
		count, _ := verifyData["attempts"].(float64)
		count++
		verifyData["attempts"] = count
		next, err := json.Marshal(verifyData)
		if err != nil {
			cerr = helper.NewCaptchaError(helper.ErrCodeCaptchaDataError, "序列化验证数据失败", err.Error())
			return old, true
		}
		attempts = int(count)
		return next, true
	})
	return attempts, cerr
}

// IsMaxAttemptsReached 检查是否达到最大尝试次数
func (bs *BaseService) IsMaxAttemptsReached(key string) (bool, *helper.CaptchaError) {
	verifyData, err := bs.GetCaptchaData(key)
	if err != nil {
		return true, err
	}

	attempts, _ := verifyData["attempts"].(float64)
	return int(attempts) >= attemptLimit(verifyData, config.Runtime()), nil
}

// attemptLimit 题目的尝试次数上限：以出题时写入的值为准，与过期时间一样不受之后的策略修改影响，
// 同一道题从入口检查到失败计数只用这一个上限；记录缺少该字段时退回当前策略
func attemptLimit(verifyData map[string]interface{}, rt *config.RuntimeSettings) int {
	if limit, ok := verifyData["maxAttempts"].(float64); ok && limit > 0 {
		return int(limit)
	}
	return rt.Captcha.MaxAttempts
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
		return helper.NewCaptchaError(helper.ErrCodeCaptchaDataError, "序列化二次验证数据失败", err.Error())
	}

	cache.Shared().Set(cache.KindChallenge, key, dataBytes, config.Runtime().Captcha.SecondVerifyExpire())
	return nil
}
