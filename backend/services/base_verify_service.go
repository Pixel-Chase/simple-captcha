package services

import (
	"simple-captcha/helper"
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

// VerifyDataTemplate 验证数据模板方法
func (bvs *BaseVerifyService) VerifyDataTemplate(
	data, key string,
	parseUserData func(string) (interface{}, *helper.CaptchaError),
	validateData func(interface{}, interface{}) bool,
) (bool, string, *helper.CaptchaError) {
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

	// 解析用户提交的数据
	userData, parseErr := parseUserData(data)
	if parseErr != nil {
		return false, parseErr.Message, parseErr
	}

	// 获取正确的数据
	correctData, ok := verifyData["data"]
	if !ok {
		bvs.DeleteCaptchaData(key)
		return false, "验证码数据异常", helper.NewCaptchaError(helper.ErrCodeCacheError, "验证码数据异常")
	}

	// 验证数据
	isValid := validateData(userData, correctData)

	if isValid {
		// 验证成功，删除原缓存
		bvs.DeleteCaptchaData(key)

		// 生成二次验证令牌
		secondKey, err := helper.GenerateTimestampedID()
		if err != nil {
			return false, "生成二次验证ID失败", helper.NewCaptchaError(helper.ErrCodeInternalError, "生成二次验证ID失败")
		}

		// 创建二次验证状态数据
		secondVerifyData := map[string]interface{}{
			"verified":   true,
			"verifyTime": time.Now().Unix(),
			"expireAt":   time.Now().Add(15 * time.Minute).Unix(),
			"origin_key": key,                             // 记录原始验证码key
			"token":      helper.GenerateRandomString(16), // 添加随机令牌
		}

		// 将二次验证数据存储到缓存
		if saveErr := bvs.SaveSecondVerifyData(secondKey, secondVerifyData); saveErr != nil {
			return false, "设置二次验证ID失败", saveErr
		}

		return true, secondKey, nil
	}

	// 验证失败
	maxAttempts := bvs.config.Captcha.MaxAttempts
	remainingAttempts := maxAttempts - attempts
	if remainingAttempts <= 0 {
		bvs.DeleteCaptchaData(key)
		return false, "尝试次数过多，请重新获取验证码", helper.NewCaptchaError(helper.ErrCodeTooManyAttempts, "尝试次数过多")
	}

	return false, "验证失败", helper.NewCaptchaError(helper.ErrCodeVerifyFailed, "验证失败")
}

// ParseClickData 解析点击数据
func (bvs *BaseVerifyService) ParseClickData(data string) (interface{}, *helper.CaptchaError) {
	// 处理前端发送的格式，data直接是坐标字符串 "184,59,280,31"
	// 解析坐标字符串
	coords := strings.Split(data, ",")
	if len(coords) < 2 || len(coords)%2 != 0 {
		return nil, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "点击坐标格式错误")
	}

	// 转换为验证逻辑需要的格式
	userDots := []map[string]interface{}{}
	for i := 0; i < len(coords); i += 2 {
		x, err1 := strconv.ParseFloat(coords[i], 64)
		y, err2 := strconv.ParseFloat(coords[i+1], 64)
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

// ParseSlideData 解析滑动数据
func (bvs *BaseVerifyService) ParseSlideData(data string) (interface{}, *helper.CaptchaError) {
	// 处理前端发送的格式，data直接是坐标字符串 "130,16"
	// 解析坐标字符串
	coords := strings.Split(data, ",")
	if len(coords) != 2 {
		return nil, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "滑动坐标格式错误")
	}

	x, err1 := strconv.ParseFloat(coords[0], 64)
	y, err2 := strconv.ParseFloat(coords[1], 64)
	if err1 != nil || err2 != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "滑动坐标格式错误")
	}

	// 转换为验证逻辑需要的格式
	userSlide := map[string]interface{}{
		"x": x,
		"y": y,
	}

	return userSlide, nil
}

// ParseRotateData 解析旋转验证数据
func (bvs *BaseVerifyService) ParseRotateData(data string) (interface{}, *helper.CaptchaError) {
	angle, err := strconv.Atoi(data)
	if err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "旋转角度格式错误")
	}

	// 验证角度范围
	if angle < 0 || angle > 360 {
		return nil, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "旋转角度超出范围")
	}

	return map[string]interface{}{"angle": float64(angle)}, nil
}
