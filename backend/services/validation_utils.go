package services

import (
	"strconv"

	"github.com/wenlng/go-captcha/v2/click"
	"github.com/wenlng/go-captcha/v2/rotate"
	"github.com/wenlng/go-captcha/v2/slide"
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

		// 使用框架的CheckPoint方法进行验证
		if !click.CheckPoint(int64(userX), int64(userY), int64(correctX), int64(correctY), int64(correctWidth), int64(correctHeight), int64(tolerance)) {
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

	// 使用框架的CheckPoint方法进行验证
	return slide.CheckPoint(int64(userX), int64(userY), int64(correctX), int64(correctY), int64(tolerance))
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

	// 使用框架的CheckAngle方法进行验证
	return rotate.CheckAngle(int64(userAngleVal), int64(correctAngleVal), int64(tolerance))
}
