package services

import (
	"log"
	"simple-captcha/helper"

	"github.com/wenlng/go-captcha/v2/slide"
)

// SlideRegionCaptchaService 滑动区域验证码服务
type SlideRegionCaptchaService struct {
	*BaseService
	backgrounds *backgroundCache
	graphs      []*slide.GraphImage
	instances   *instanceCache[slide.Captcha]
}

// NewSlideRegionCaptchaService 创建滑动区域验证码服务
func NewSlideRegionCaptchaService() *SlideRegionCaptchaService {
	images, err := helper.GetImages()
	if err != nil {
		log.Fatalln("加载图片资源失败:", err)
	}

	service := &SlideRegionCaptchaService{
		BaseService: NewBaseService(),
		backgrounds: newBackgroundCache(images),
		graphs:      loadSlideGraphs(),
	}
	service.instances = newInstanceCache(service.build)
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
		slide.WithBackgrounds(s.backgrounds.get(size.Width, size.Height)),
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
