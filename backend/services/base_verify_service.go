package services

import (
	"simple-captcha/config"
	"simple-captcha/helper"
	"simple-captcha/risk"
	"strconv"
	"strings"
	"time"
)

// BaseVerifyService 基础验证服务
type BaseVerifyService struct {
	*BaseService
}

// NewBaseVerifyService 创建基础验证服务
func NewBaseVerifyService() *BaseVerifyService {
	return &BaseVerifyService{
		BaseService: NewBaseService(),
	}
}

// AnswerValidator 答案比对函数：userData 为解析后的用户答案，correctData 为缓存中的正确答案，
// meta 提供倍率与难度，用于计算容差。
type AnswerValidator func(userData, correctData interface{}, meta *ChallengeMeta) bool

// VerifyDataTemplate 验证流程模板：
// 尝试次数 → 客户端绑定 → 时间窗口 → 工作量证明 → 答案比对 → 签发二次验证 key。
// 风控拦截（in.RiskBlocked）在答案比对阶段生效，此前各步与未拦截时完全一致。
func (bvs *BaseVerifyService) VerifyDataTemplate(
	in *VerifyInput,
	parseUserData func(string) (interface{}, *helper.CaptchaError),
	validateData AnswerValidator,
) (bool, string, *helper.CaptchaError) {
	if in == nil {
		return false, "参数错误", helper.NewCaptchaError(helper.ErrCodeInvalidParam, "参数错误")
	}
	key := in.Key
	// 整个校验流程使用同一份策略快照
	rt := config.Runtime()

	// 应用绑定：提交的 appId 必须与出题时一致。不一致时与"题目不存在"的响应完全相同，
	// 也不计尝试次数、不作废题目，别的应用无法借此探测或耗尽这道题
	if current, err := bvs.GetCaptchaData(key); err != nil {
		return false, "验证码已失效", err
	} else if ParseMeta(current).AppID != in.AppID {
		return false, "验证码已失效", helper.NewCaptchaError(helper.ErrCodeExpired, "验证码不存在或已失效")
	}

	// 检查是否达到最大尝试次数
	if maxReached, err := bvs.IsMaxAttemptsReached(key); err != nil {
		return false, "验证码已失效", err
	} else if maxReached {
		bvs.DeleteCaptchaData(key)
		return false, "尝试次数过多，请重新获取验证码", helper.NewCaptchaError(helper.ErrCodeTooManyAttempts, "尝试次数过多")
	}

	// 增加尝试次数
	attempts, incErr := bvs.IncrementAttempts(key)
	if incErr != nil {
		return false, "验证失败", incErr
	}

	// 获取验证码数据
	verifyData, getErr := bvs.GetCaptchaData(key)
	if getErr != nil {
		return false, "验证码已失效", getErr
	}
	meta := ParseMeta(verifyData)
	maxAttempts := attemptLimit(verifyData, rt)

	// 客户端绑定：出题与提交必须来自同一客户端，不匹配视为 key 被转移使用，直接作废
	if rt.Captcha.BindClient && meta.ClientHash != "" && meta.ClientHash != in.ClientHash {
		bvs.DeleteCaptchaData(key)
		return false, helper.ErrClientMismatch.Message, helper.NewCaptchaError(helper.ErrCodeClientMismatch, "客户端指纹不匹配")
	}

	// 时间窗口：出题到提交间隔过短判定为自动化提交，计入一次失败
	if minSolve := rt.Captcha.MinSolveTime(); minSolve > 0 {
		if created := CreatedAt(verifyData); !created.IsZero() && time.Since(created) < minSolve {
			return bvs.failResult(key, attempts, maxAttempts, helper.ErrTooFast.Message, helper.ErrCodeTooFast)
		}
	}

	// 工作量证明：nonce 与难度均以服务端落库值为准，客户端无法自行降低难度
	if meta.PowBits > 0 {
		switch {
		case in.Pow == nil || in.Pow.Solution == "":
			if rt.Pow.Required {
				return bvs.failResult(key, attempts, maxAttempts, helper.ErrPowInvalid.Message, helper.ErrCodePowInvalid)
			}
		case in.Pow.Nonce != meta.PowNonce || !risk.VerifyPow(meta.PowNonce, key, in.Pow.Solution, meta.PowBits):
			return bvs.failResult(key, attempts, maxAttempts, helper.ErrPowInvalid.Message, helper.ErrCodePowInvalid)
		}
	}

	// 解析用户提交的数据
	userData, parseErr := parseUserData(in.Answer)
	if parseErr != nil {
		return false, parseErr.Message, parseErr
	}

	correctData, ok := verifyData["data"]
	if !ok {
		bvs.DeleteCaptchaData(key)
		return false, "验证码数据异常", helper.NewCaptchaError(helper.ErrCodeCacheError, "验证码数据异常")
	}

	// 风控拦截下沉到这一步：前面的尝试次数、客户端绑定、时间窗口、PoW、答案解析全部照常执行，
	// 响应与未拦截时一致，攻击者无法通过故意触发前置错误来探测自己是否被拦截。
	// 比对照常执行以保持耗时一致，结果被强制判为失败，同样消耗一次尝试机会
	answerMatched := validateData(userData, correctData, meta)
	if in.RiskBlocked || !answerMatched {
		return bvs.failResult(key, attempts, maxAttempts, "验证失败", helper.ErrCodeVerifyFailed)
	}

	// 验证成功：原题作废，签发一次性二次验证 key
	bvs.DeleteCaptchaData(key)

	secondKey, err := helper.GenerateTimestampedID()
	if err != nil {
		return false, "生成二次验证ID失败", helper.NewCaptchaError(helper.ErrCodeInternalError, "生成二次验证ID失败")
	}

	secondVerifyData := map[string]interface{}{
		"verified":   true,
		"verifyTime": time.Now().Unix(),
		"expireAt":   time.Now().Add(rt.Captcha.SecondVerifyExpire()).Unix(),
		"origin_key": key,
		"type":       meta.Type,
		"appId":      meta.AppID,
		"token":      helper.GenerateRandomString(16),
	}

	if saveErr := bvs.SaveSecondVerifyData(secondKey, secondVerifyData); saveErr != nil {
		return false, "设置二次验证ID失败", saveErr
	}

	return true, secondKey, nil
}

// failResult 统一处理失败：剩余次数耗尽时作废题目。maxAttempts 是出题时写入题目的上限，
// 与入口的 IsMaxAttemptsReached 一致，不读当前策略
func (bvs *BaseVerifyService) failResult(key string, attempts, maxAttempts int, message string, code int) (bool, string, *helper.CaptchaError) {
	if remaining := maxAttempts - attempts; remaining <= 0 {
		bvs.DeleteCaptchaData(key)
		return false, "尝试次数过多，请重新获取验证码", helper.NewCaptchaError(helper.ErrCodeTooManyAttempts, "尝试次数过多")
	}
	return false, message, helper.NewCaptchaError(code, message)
}

// ParseClickData 解析点击数据，格式为 "x1,y1,x2,y2,..."
func (bvs *BaseVerifyService) ParseClickData(data string) (interface{}, *helper.CaptchaError) {
	coords := strings.Split(data, ",")
	if len(coords) < 2 || len(coords)%2 != 0 || len(coords) > 40 {
		return nil, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "点击坐标格式错误")
	}

	userDots := []map[string]interface{}{}
	for i := 0; i < len(coords); i += 2 {
		x, err1 := strconv.ParseFloat(strings.TrimSpace(coords[i]), 64)
		y, err2 := strconv.ParseFloat(strings.TrimSpace(coords[i+1]), 64)
		if err1 != nil || err2 != nil {
			return nil, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "点击坐标格式错误")
		}

		userDots = append(userDots, map[string]interface{}{
			"x": x,
			"y": y,
		})
	}

	return userDots, nil
}

// ParseSlideData 解析滑动数据，格式为 "x,y"
func (bvs *BaseVerifyService) ParseSlideData(data string) (interface{}, *helper.CaptchaError) {
	coords := strings.Split(data, ",")
	if len(coords) != 2 {
		return nil, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "滑动坐标格式错误")
	}

	x, err1 := strconv.ParseFloat(strings.TrimSpace(coords[0]), 64)
	y, err2 := strconv.ParseFloat(strings.TrimSpace(coords[1]), 64)
	if err1 != nil || err2 != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "滑动坐标格式错误")
	}

	return map[string]interface{}{
		"x": x,
		"y": y,
	}, nil
}

// ParseRotateData 解析旋转验证数据（角度）
func (bvs *BaseVerifyService) ParseRotateData(data string) (interface{}, *helper.CaptchaError) {
	angle, err := strconv.Atoi(strings.TrimSpace(data))
	if err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "旋转角度格式错误")
	}

	if angle < 0 || angle > 360 {
		return nil, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "旋转角度超出范围")
	}

	return map[string]interface{}{"angle": float64(angle)}, nil
}
