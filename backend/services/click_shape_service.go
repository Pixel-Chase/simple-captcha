package services

import (
	"image"
	"log"
	"simple-captcha/helper"

	"github.com/wenlng/go-captcha-assets/resources/shapes"
	"simple-captcha/internal/gocaptcha/base/option"
	"simple-captcha/internal/gocaptcha/click"
)

// ClickShapeCaptchaService 点击形状验证码服务
type ClickShapeCaptchaService struct {
	*BaseService
	shapes      map[string]image.Image
	backgrounds *BackgroundStore
	instances   *instanceCache[click.Captcha]
}

// NewClickShapeCaptchaService 创建点击形状验证码服务
func NewClickShapeCaptchaService(backgrounds *BackgroundStore) *ClickShapeCaptchaService {
	shapeMaps, err := shapes.GetShapes()
	if err != nil {
		log.Fatalln("加载形状数据失败:", err)
	}

	service := &ClickShapeCaptchaService{
		BaseService: NewBaseService(),
		shapes:      shapeMaps,
		backgrounds: backgrounds,
	}
	service.instances = newInstanceCache(service.build)
	// 素材热重载后丢弃已构建的生成器实例，下一次出题使用新素材
	backgrounds.OnReload(service.instances.reset)
	return service
}

// build 按难度 × 倍率构建生成器实例（图形点选不区分主题）
func (s *ClickShapeCaptchaService) build(key instanceKey) click.Captcha {
	profile := GetDifficultyProfile(key.level)

	size := imageSize(key.scale)
	builder := click.NewBuilder(
		click.WithImageSize(size),
		click.WithRangeThumbImageSize(thumbSize(key.scale)),
		click.WithRangeLen(profile.ClickLen),
		click.WithRangeVerifyLen(profile.ClickVerifyLen),
		click.WithRangeSize(scaleRange(profile.ClickFontSize, key.scale)),
		click.WithRangeThumbSize(scaleRange(option.RangeVal{Min: 22, Max: 28}, key.scale)),
		click.WithRangeThumbBgDistort(profile.ThumbDistort),
		click.WithRangeThumbBgCirclesNum(profile.ThumbCircles),
		click.WithRangeThumbBgSlimLineNum(profile.ThumbSlimLines),
		click.WithIsThumbNonDeformAbility(true),
	)

	builder.SetResources(
		click.WithShapes(s.shapes),
		click.WithBackgrounds(s.backgrounds.Get(size.Width, size.Height)),
	)
	return builder.MakeShape()
}

// GenerateCaptcha 生成验证码
func (s *ClickShapeCaptchaService) GenerateCaptcha(req *ChallengeRequest) (*CaptchaData, *helper.CaptchaError) {
	req = s.normalizeRequest(req, s.GetServiceType())
	// 图形点选不区分明暗主题，统一使用一套实例
	captcha := s.instances.get(req.Difficulty, req.Scale, "")
	return generateClickCaptcha(s.BaseService, captcha, req)
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
func (s *ClickShapeVerifyService) VerifyData(in *VerifyInput) (bool, string, *helper.CaptchaError) {
	return s.VerifyDataTemplate(in, s.ParseClickData, validateClickAnswer(s.validator))
}
