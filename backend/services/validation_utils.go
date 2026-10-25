package services

import (
	"strconv"

	"simple-captcha/internal/gocaptcha/click"
	"simple-captcha/internal/gocaptcha/rotate"
	"simple-captcha/internal/gocaptcha/slide"
)

// ValidationUtils 验证工具类
type ValidationUtils struct{}

// NewValidationUtils 创建验证工具实例
func NewValidationUtils() *ValidationUtils {
	return &ValidationUtils{}
}

// ValidateClickData 验证点击数据
func (vu *ValidationUtils) ValidateClickData(userDots []map[string]interface{}, correctData interface{}, tolerance float64) bool {
	// 将正确数据转换为map[int]*click.Dot格式
	correctDataMap, ok := correctData.(map[string]interface{})
	if !ok {
		return false
	}

	// 检查点击数量是否匹配
	if len(userDots) != len(correctDataMap) {
		return false
	}

	// 验证每个点击位置
	for i, userDot := range userDots {
		// 获取对应的正确点数据
		correctDotData, exists := correctDataMap[strconv.Itoa(i)]
		if !exists {
			return false
		}

		correctDot, ok := correctDotData.(map[string]interface{})
		if !ok {
			return false
		}

		// 获取用户点击坐标
		userX, userXOk := userDot["x"].(float64)
		userY, userYOk := userDot["y"].(float64)
		if !userXOk || !userYOk {
			return false
		}

		// 获取正确坐标和尺寸信息
		correctX, xOk := correctDot["x"].(float64)
		correctY, yOk := correctDot["y"].(float64)
		correctWidth, wOk := correctDot["width"].(float64)
		correctHeight, hOk := correctDot["height"].(float64)
		if !xOk || !yOk || !wOk || !hOk {
			return false
		}

		if !click.Validate(int(userX), int(userY), int(correctX), int(correctY), int(correctWidth), int(correctHeight), int(tolerance)) {
			return false
		}
	}

	return true
}

// ValidateSlidePosition 验证滑动位置
func (vu *ValidationUtils) ValidateSlidePosition(userSlide map[string]interface{}, correctData interface{}, tolerance float64) bool {
	// 将正确数据转换为*slide.Block格式
	correctDataMap, ok := correctData.(map[string]interface{})
	if !ok {
		return false
	}

	// 获取用户提交的X坐标
	userX, userXOk := userSlide["x"].(float64)
	if !userXOk {
		return false
	}

	// 获取用户提交的Y坐标
	userY, userYOk := userSlide["y"].(float64)
	if !userYOk {
		return false
	}

	// 获取正确的X坐标
	correctX, xOk := correctDataMap["x"].(float64)
	if !xOk {
		return false
	}

	// 获取正确的Y坐标
	correctY, yOk := correctDataMap["y"].(float64)
	if !yOk {
		return false
	}

	return slide.Validate(int(userX), int(userY), int(correctX), int(correctY), int(tolerance))
}

// ValidateRotateAngle 验证旋转角度
func (vu *ValidationUtils) ValidateRotateAngle(userAngle map[string]interface{}, correctData interface{}, tolerance float64) bool {
	// 将正确数据转换为*rotate.Block格式
	correctDataMap, ok := correctData.(map[string]interface{})
	if !ok {
		return false
	}

	// 获取用户提交的角度
	userAngleVal, userOk := userAngle["angle"].(float64)
	if !userOk {
		return false
	}

	// 获取正确的角度
	correctAngleVal, correctOk := correctDataMap["angle"].(float64)
	if !correctOk {
		return false
	}

	return rotate.Validate(int(userAngleVal), int(correctAngleVal), int(tolerance))
}
