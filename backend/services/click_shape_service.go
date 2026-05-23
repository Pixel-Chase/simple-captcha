package services

import (
	"log"
	"simple-captcha/helper"

	"github.com/wenlng/go-captcha-assets/resources/shapes"
	"github.com/wenlng/go-captcha/v2/base/option"
	"github.com/wenlng/go-captcha/v2/click"
)

// ClickShapeCaptchaService 点击形状验证码服务
type ClickShapeCaptchaService struct {
	*BaseService
	shapeCapt click.Captcha
}

// NewClickShapeCaptchaService 创建点击形状验证码服务
func NewClickShapeCaptchaService() *ClickShapeCaptchaService {
	service := &ClickShapeCaptchaService{
		BaseService: NewBaseService(),
	}
	service.initCaptcha()
	return service
}

// initCaptcha 初始化验证码生成器
func (s *ClickShapeCaptchaService) initCaptcha() {
	// 创建一个新的构建器实例，并设置各种配置选项
	builder := click.NewBuilder(
		click.WithRangeLen(option.RangeVal{Min: 3, Max: 6}),
		click.WithRangeVerifyLen(option.RangeVal{Min: 2, Max: 3}),
		click.WithRangeThumbBgDistort(1),
		click.WithIsThumbNonDeformAbility(true),
	)

	// 加载形状数据，这些数据对于验证码生成至关重要
	shapeMaps, err := shapes.GetShapes()
	if err != nil {
		log.Fatalln("加载形状数据失败:", err)
	}

	// 加载图像资源，用于背景等用途
	images, err := helper.GetImages()
	if err != nil {
		log.Fatalln("加载图片资源失败:", err)
	}

	// 设置构建器所需的资源
	builder.SetResources(
		click.WithShapes(shapeMaps),
		click.WithBackgrounds(images),
	)

	// 使用构建器生成形状验证码
	s.shapeCapt = builder.MakeWithShape()
}

// GenerateCaptcha 生成验证码
func (s *ClickShapeCaptchaService) GenerateCaptcha(mode string) (*CaptchaData, *helper.CaptchaError) {
	// 生成验证码数据
	captData, err := s.shapeCapt.Generate()
	if err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeCaptchaGenError, "验证码生成失败", err.Error())
	}

	// 获取验证码中的点数据
	dotData := captData.GetData()
	if dotData == nil {
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
	key, keyErr := s.GenerateKey(dotData)
	if keyErr != nil {
		return nil, keyErr
	}

	// 保存验证码数据到缓存
	if saveErr := s.SaveCaptchaData(key, dotData); saveErr != nil {
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
func (s *ClickShapeCaptchaService) GetServiceType() string {
	return "click-shape"
}

// ClickShapeVerifyService 点击形状验证服务
type ClickShapeVerifyService struct {
	*BaseVerifyService
	validator *ValidationUtils
}

// NewClickShapeVerifyService 创建点击形状验证服务
func NewClickShapeVerifyService() *ClickShapeVerifyService {
	return &ClickShapeVerifyService{
		BaseVerifyService: NewBaseVerifyService(),
		validator:         NewValidationUtils(),
	}
}

// VerifyData 验证数据
func (s *ClickShapeVerifyService) VerifyData(data, key string) (bool, string, *helper.CaptchaError) {
	return s.VerifyDataTemplate(
		data, key,
		s.ParseClickData,
		func(userData, correctData interface{}) bool {
			userDots := userData.([]map[string]interface{})
			return s.validator.ValidateClickData(userDots, correctData, 25.0)
		},
	)
}
