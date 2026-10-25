package services

import (
	"log"
	"simple-captcha/helper"

	"github.com/wenlng/go-captcha-assets/resources/tiles"
	"simple-captcha/internal/gocaptcha/base/imagedata"
	"simple-captcha/internal/gocaptcha/slide"
)

// loadSlideGraphs 加载滑块拼图块资源
func loadSlideGraphs() []*slide.GraphImage {
	graphs, err := tiles.GetTiles()
	if err != nil {
		log.Fatalln("加载瓷砖资源失败:", err)
	}

	newGraphs := make([]*slide.GraphImage, 0, len(graphs))
	for _, graph := range graphs {
		newGraphs = append(newGraphs, &slide.GraphImage{
			OverlayImage: graph.OverlayImage,
			MaskImage:    graph.MaskImage,
			ShadowImage:  graph.ShadowImage,
		})
	}
	return newGraphs
}

// SlideTextCaptchaService 滑块拼图验证码服务
type SlideTextCaptchaService struct {
	*BaseService
	backgrounds *BackgroundStore
	graphs      []*slide.GraphImage
	instances   *instanceCache[slide.Captcha]
}

// NewSlideTextCaptchaService 创建滑块拼图验证码服务
func NewSlideTextCaptchaService(backgrounds *BackgroundStore) *SlideTextCaptchaService {
	service := &SlideTextCaptchaService{
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
func (s *SlideTextCaptchaService) build(key instanceKey) slide.Captcha {
	profile := GetDifficultyProfile(key.level)

	size := imageSize(key.scale)
	builder := slide.NewBuilder(
		slide.WithImageSize(size),
		slide.WithRangeGraphSize(scaleRange(profile.SlideGraphSize, key.scale)),
		slide.WithEnableGraphVerticalRandom(true),
	)
	builder.SetResources(
		slide.WithGraphImages(s.graphs),
		slide.WithBackgrounds(s.backgrounds.Get(size.Width, size.Height)),
	)
	return builder.Make()
}

// GenerateCaptcha 生成验证码
func (s *SlideTextCaptchaService) GenerateCaptcha(req *ChallengeRequest) (*CaptchaData, *helper.CaptchaError) {
	req = s.normalizeRequest(req, s.GetServiceType())
	captcha := s.instances.get(req.Difficulty, req.Scale, "")
	return generateSlideCaptcha(s.BaseService, captcha, req)
}

// GetServiceType 获取服务类型
func (s *SlideTextCaptchaService) GetServiceType() string {
	return "slide-text"
}

// generateSlideCaptcha 滑块类（拼图/区域）共用的生成流程
func generateSlideCaptcha(bs *BaseService, captcha slide.Captcha, req *ChallengeRequest) (*CaptchaData, *helper.CaptchaError) {
	captData, err := captcha.Generate()
	if err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeCaptchaGenError, "验证码生成失败", err.Error())
	}

	blockData := captData.GetData()
	if blockData == nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeCaptchaGenError, "验证码数据为空")
	}

	// 主图与拼图块共用一组颜色扰动，对齐后颜色一致
	jitter := newImageJitter()
	masterImageBase64, err := imagedata.NewJPEGImageData(jitter.apply(captData.GetMasterImage().Get())).
		ToBase64WithQuality(bs.config.Captcha.ImageQuality)
	if err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeCaptchaGenError, "主图片转换失败", err.Error())
	}

	tileImageBase64, err := imagedata.NewPNGImageData(jitter.apply(captData.GetTileImage().Get())).ToBase64()
	if err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeCaptchaGenError, "滑动块图片转换失败", err.Error())
	}

	key, meta, persistErr := bs.persistChallenge(req, blockData)
	if persistErr != nil {
		return nil, persistErr
	}

	size := imageSize(req.Scale)
	return &CaptchaData{
		Key:         key,
		Image:       masterImageBase64,
		Tile:        tileImageBase64,
		Width:       blockData.Width,
		Height:      blockData.Height,
		X:           blockData.TileX,
		Y:           blockData.TileY,
		ImageWidth:  size.Width,
		ImageHeight: size.Height,
		Scale:       req.Scale,
		Difficulty:  req.Difficulty,
		PowNonce:    meta.PowNonce,
		PowBits:     meta.PowBits,
	}, nil
}

// SlideTextVerifyServiceImpl 滑块拼图验证服务实现
type SlideTextVerifyServiceImpl struct {
	*BaseVerifyService
	validator *ValidationUtils
}

// NewSlideTextVerifyServiceImpl 创建滑块拼图验证服务实现
func NewSlideTextVerifyServiceImpl() *SlideTextVerifyServiceImpl {
	return &SlideTextVerifyServiceImpl{
		BaseVerifyService: NewBaseVerifyService(),
		validator:         NewValidationUtils(),
	}
}

// VerifyData 验证数据
func (s *SlideTextVerifyServiceImpl) VerifyData(in *VerifyInput) (bool, string, *helper.CaptchaError) {
	return s.VerifyDataTemplate(in, s.ParseSlideData, validateSlideAnswer(s.validator))
}

// validateSlideAnswer 滑块答案比对，容差按难度与倍率换算
func validateSlideAnswer(validator *ValidationUtils) AnswerValidator {
	return func(userData, correctData interface{}, meta *ChallengeMeta) bool {
		userSlide, ok := userData.(map[string]interface{})
		if !ok {
			return false
		}
		return validator.ValidateSlidePosition(userSlide, correctData, SlideToleranceFor(meta))
	}
}
