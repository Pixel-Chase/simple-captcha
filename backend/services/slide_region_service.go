package services

import (
	"simple-captcha/helper"

	"simple-captcha/internal/gocaptcha/slide"
)

// SlideRegionCaptchaService 滑动区域验证码服务
type SlideRegionCaptchaService struct {
	*BaseService
	backgrounds *BackgroundStore
	graphs      []*slide.GraphImage
	instances   *instanceCache[slide.Captcha]
}

// NewSlideRegionCaptchaService 创建滑动区域验证码服务
func NewSlideRegionCaptchaService(backgrounds *BackgroundStore) *SlideRegionCaptchaService {
	service := &SlideRegionCaptchaService{
		BaseService: NewBaseService(),
		backgrounds: backgrounds,
		graphs:      loadSlideGraphs(),
	}
	service.instances = newInstanceCache(service.build)
	// 素材热重载后丢弃已构建的生成器实例，下一次出题使用新素材
	backgrounds.OnReload(service.instances.reset)
	return service
}

// build 按难度 × 倍率构建生成器实例
func (s *SlideRegionCaptchaService) build(key instanceKey) slide.Captcha {
	profile := GetDifficultyProfile(key.level)

	size := imageSize(key.scale)
	builder := slide.NewBuilder(
		slide.WithImageSize(size),
		slide.WithRangeGraphSize(scaleRange(profile.SlideGraphSize, key.scale)),
		slide.WithGenGraphNumber(2),
		slide.WithEnableGraphVerticalRandom(true),
	)
	builder.SetResources(
		slide.WithGraphImages(s.graphs),
		slide.WithBackgrounds(s.backgrounds.Get(size.Width, size.Height)),
	)
	return builder.MakeWithRegion()
}

// GenerateCaptcha 生成验证码
func (s *SlideRegionCaptchaService) GenerateCaptcha(req *ChallengeRequest) (*CaptchaData, *helper.CaptchaError) {
	req = s.normalizeRequest(req, s.GetServiceType())
	captcha := s.instances.get(req.Difficulty, req.Scale, "")
	return generateSlideCaptcha(s.BaseService, captcha, req)
}

// GetServiceType 获取服务类型
func (s *SlideRegionCaptchaService) GetServiceType() string {
	return "slide-region"
}

// SlideRegionVerifyServiceImpl 滑动区域验证服务实现
type SlideRegionVerifyServiceImpl struct {
	*BaseVerifyService
	validator *ValidationUtils
}

// NewSlideRegionVerifyServiceImpl 创建滑动区域验证服务实现
func NewSlideRegionVerifyServiceImpl() *SlideRegionVerifyServiceImpl {
	return &SlideRegionVerifyServiceImpl{
		BaseVerifyService: NewBaseVerifyService(),
		validator:         NewValidationUtils(),
	}
}

// VerifyData 验证数据
func (s *SlideRegionVerifyServiceImpl) VerifyData(in *VerifyInput) (bool, string, *helper.CaptchaError) {
	return s.VerifyDataTemplate(in, s.ParseSlideData, validateSlideAnswer(s.validator))
}
