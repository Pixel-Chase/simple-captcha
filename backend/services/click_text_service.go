package services

import (
	"log"
	"simple-captcha/helper"

	"github.com/golang/freetype/truetype"
	"github.com/wenlng/go-captcha-assets/bindata/chars"
	"github.com/wenlng/go-captcha-assets/resources/fonts/fzshengsksjw"
	"simple-captcha/internal/gocaptcha/base/imagedata"
	"simple-captcha/internal/gocaptcha/base/option"
	"simple-captcha/internal/gocaptcha/click"
)

// 点选文字的配色：暗色主题使用浅色文字 + 深色提示图，亮色主题使用默认文字色 + 鲜艳提示图
var (
	darkThumbColors  = []string{"#1f55c4", "#780592", "#2f6b00", "#910000", "#864401", "#675901", "#016e5c"}
	darkTextColors   = []string{"#fde98e", "#60c1ff", "#fcb08e", "#fb88ff", "#b4fed4", "#cbfaa9", "#78d6f8"}
	lightThumbColors = []string{"#4a85fb", "#d93ffb", "#56be01", "#ee2b2b", "#cd6904", "#b49b03", "#01ad90"}
)

// ClickTextCaptchaService 点击文字验证码服务
type ClickTextCaptchaService struct {
	*BaseService
	fonts       []*truetype.Font
	backgrounds *BackgroundStore
	chars       []string
	instances   *instanceCache[click.Captcha]
}

// NewClickTextCaptchaService 创建点击文字验证码服务
func NewClickTextCaptchaService(backgrounds *BackgroundStore) *ClickTextCaptchaService {
	fonts, err := fzshengsksjw.GetFont()
	if err != nil {
		log.Fatalln("加载字体失败:", err)
	}

	service := &ClickTextCaptchaService{
		BaseService: NewBaseService(),
		fonts:       []*truetype.Font{fonts},
		backgrounds: backgrounds,
		chars:       chars.GetChineseChars(),
	}
	service.instances = newInstanceCache(service.build)
	// 素材热重载后丢弃已构建的生成器实例，下一次出题使用新素材
	backgrounds.OnReload(service.instances.reset)
	return service
}

// build 按难度 × 倍率 × 主题构建生成器实例
func (s *ClickTextCaptchaService) build(key instanceKey) click.Captcha {
	profile := GetDifficultyProfile(key.level)

	thumbColors := darkThumbColors
	if key.mode == "light" {
		thumbColors = lightThumbColors
	}

	size := imageSize(key.scale)
	opts := []click.Option{
		click.WithImageSize(size),
		click.WithRangeThumbImageSize(thumbSize(key.scale)),
		click.WithRangeLen(profile.ClickLen),
		click.WithRangeVerifyLen(profile.ClickVerifyLen),
		click.WithRangeSize(scaleRange(profile.ClickFontSize, key.scale)),
		click.WithRangeThumbSize(scaleRange(option.RangeVal{Min: 22, Max: 28}, key.scale)),
		click.WithRangeAnglePos(profile.ClickAngles),
		click.WithRangeThumbBgDistort(profile.ThumbDistort),
		click.WithRangeThumbBgCirclesNum(profile.ThumbCircles),
		click.WithRangeThumbBgSlimLineNum(profile.ThumbSlimLines),
		click.WithRangeThumbColors(thumbColors),
	}
	if key.mode != "light" {
		opts = append(opts, click.WithRangeColors(darkTextColors))
	}

	builder := click.NewBuilder(opts...)
	builder.SetResources(
		click.WithChars(s.chars),
		click.WithFonts(s.fonts),
		click.WithBackgrounds(s.backgrounds.Get(size.Width, size.Height)),
	)
	return builder.Make()
}

// GenerateCaptcha 生成验证码
func (s *ClickTextCaptchaService) GenerateCaptcha(req *ChallengeRequest) (*CaptchaData, *helper.CaptchaError) {
	req = s.normalizeRequest(req, s.GetServiceType())
	captcha := s.instances.get(req.Difficulty, req.Scale, req.Mode)
	return generateClickCaptcha(s.BaseService, captcha, req)
}

// GetServiceType 获取服务类型
func (s *ClickTextCaptchaService) GetServiceType() string {
	return "click-text"
}

// generateClickCaptcha 点选类（文字/图形）共用的生成流程
func generateClickCaptcha(bs *BaseService, captcha click.Captcha, req *ChallengeRequest) (*CaptchaData, *helper.CaptchaError) {
	captData, err := captcha.Generate()
	if err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeCaptchaGenError, "验证码生成失败", err.Error())
	}

	dotData := captData.GetData()
	if dotData == nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeCaptchaGenError, "验证码数据为空")
	}

	// 只扰动主图；提示图不含背景素材，保持原样便于辨认
	masterImageBase64, err := imagedata.NewJPEGImageData(newImageJitter().apply(captData.GetMasterImage().Get())).
		ToBase64WithQuality(bs.config.Captcha.ImageQuality)
	if err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeCaptchaGenError, "主图片转换失败", err.Error())
	}

	thumbImageBase64, err := captData.GetThumbImage().ToBase64()
	if err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeCaptchaGenError, "缩略图转换失败", err.Error())
	}

	key, meta, persistErr := bs.persistChallenge(req, dotData)
	if persistErr != nil {
		return nil, persistErr
	}

	size := imageSize(req.Scale)
	thumb := thumbSize(req.Scale)
	return &CaptchaData{
		Key:         key,
		Image:       masterImageBase64,
		Thumb:       thumbImageBase64,
		ImageWidth:  size.Width,
		ImageHeight: size.Height,
		ThumbWidth:  thumb.Width,
		ThumbHeight: thumb.Height,
		Scale:       req.Scale,
		Difficulty:  req.Difficulty,
		PowNonce:    meta.PowNonce,
		PowBits:     meta.PowBits,
	}, nil
}

// ClickTextVerifyService 点击文字验证服务
type ClickTextVerifyService struct {
	*BaseVerifyService
	validator *ValidationUtils
}

// NewClickTextVerifyService 创建点击文字验证服务
func NewClickTextVerifyService() *ClickTextVerifyService {
	return &ClickTextVerifyService{
		BaseVerifyService: NewBaseVerifyService(),
		validator:         NewValidationUtils(),
	}
}

// VerifyData 验证数据
func (s *ClickTextVerifyService) VerifyData(in *VerifyInput) (bool, string, *helper.CaptchaError) {
	return s.VerifyDataTemplate(in, s.ParseClickData, validateClickAnswer(s.validator))
}

// validateClickAnswer 点选答案比对，容差按难度与倍率换算
func validateClickAnswer(validator *ValidationUtils) AnswerValidator {
	return func(userData, correctData interface{}, meta *ChallengeMeta) bool {
		userDots, ok := userData.([]map[string]interface{})
		if !ok {
			return false
		}
		return validator.ValidateClickData(userDots, correctData, ClickToleranceFor(meta))
	}
}
