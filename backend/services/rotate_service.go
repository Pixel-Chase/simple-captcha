package services

import (
	"log"
	"simple-captcha/helper"

	"github.com/wenlng/go-captcha/v2/base/option"
	"github.com/wenlng/go-captcha/v2/rotate"
)

// RotateCaptchaService 旋转验证码服务
type RotateCaptchaService struct {
	*BaseService
	rotateCapt rotate.Captcha
}

// NewRotateCaptchaService 创建旋转验证码服务
func NewRotateCaptchaService() *RotateCaptchaService {
	service := &RotateCaptchaService{
		BaseService: NewBaseService(),
	}
	service.initCaptcha()
	return service
}

// initCaptcha 初始化验证码生成器
func (s *RotateCaptchaService) initCaptcha() {
	// 创建一个旋转验证码的构建器，并设置旋转角度的范围
	builder := rotate.NewBuilder(rotate.WithRangeAnglePos([]option.RangeVal{
		{Min: 20, Max: 330},
	}))

	// 获取验证码图片资源，并检查是否有错误发生
	images, err := helper.GetImages()
	if err != nil {
		log.Fatalln("加载图片资源失败:", err)
	}

	// 将获取到的图片资源设置到验证码构建器中
	builder.SetResources(
		rotate.WithImages(images),
	)

	// 使用构建器生成旋转类型的验证码实例
	s.rotateCapt = builder.Make()
}

// GenerateCaptcha 生成验证码
func (s *RotateCaptchaService) GenerateCaptcha(mode string) (*CaptchaData, *helper.CaptchaError) {
	// 调用 rotateCapt.Generate() 生成基础验证码数据
	captData, err := s.rotateCapt.Generate()
	if err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeCaptchaGenError, "验证码生成失败", err.Error())
	}

	// 获取验证码数据块
	blockData := captData.GetData()
	if blockData == nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeCaptchaGenError, "验证码数据为空")
	}

	// 转换图片为Base64
	masterImageBase64, err := captData.GetMasterImage().ToBase64()
	if err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeCaptchaGenError, "主图片转换失败", err.Error())
	}

	thumbImageBase64, err := captData.GetThumbImage().ToBase64()
	if err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeCaptchaGenError, "缩略图转换失败", err.Error())
	}

	// 生成验证码key
	key, keyErr := s.GenerateKey(blockData)
	if keyErr != nil {
		return nil, keyErr
	}

	// 保存验证码数据到缓存
	if saveErr := s.SaveCaptchaData(key, blockData); saveErr != nil {
		return nil, saveErr
	}

	// 返回验证码数据
	return &CaptchaData{
		Key:   key,
		Image: masterImageBase64,
		Thumb: thumbImageBase64,
	}, nil
}

// GetServiceType 获取服务类型
func (s *RotateCaptchaService) GetServiceType() string {
	return "rotate"
}

// RotateVerifyServiceImpl 旋转验证服务实现
type RotateVerifyServiceImpl struct {
	*BaseVerifyService
	validator *ValidationUtils
}

// NewRotateVerifyServiceImpl 创建旋转验证服务实现
func NewRotateVerifyServiceImpl() *RotateVerifyServiceImpl {
	return &RotateVerifyServiceImpl{
		BaseVerifyService: NewBaseVerifyService(),
		validator:         NewValidationUtils(),
	}
}

// VerifyData 验证数据
func (s *RotateVerifyServiceImpl) VerifyData(data, key string) (bool, string, *helper.CaptchaError) {
	return s.VerifyDataTemplate(
		data, key,
		s.ParseRotateData,
		func(userData, correctData interface{}) bool {
			userAngle := userData.(map[string]interface{})
			return s.validator.ValidateRotateAngle(userAngle, correctData, 5.0)
		},
	)
}
