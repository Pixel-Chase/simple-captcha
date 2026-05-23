package services

import (
	"log"
	"simple-captcha/helper"

	"github.com/golang/freetype/truetype"
	"github.com/wenlng/go-captcha-assets/bindata/chars"
	"github.com/wenlng/go-captcha-assets/resources/fonts/fzshengsksjw"
	"github.com/wenlng/go-captcha/v2/base/option"
	"github.com/wenlng/go-captcha/v2/click"
)

// ClickTextCaptchaService 点击文字验证码服务
type ClickTextCaptchaService struct {
	*BaseService
	textCapt      click.Captcha
	lightTextCapt click.Captcha
}

// NewClickTextCaptchaService 创建点击文字验证码服务
func NewClickTextCaptchaService() *ClickTextCaptchaService {
	service := &ClickTextCaptchaService{
		BaseService: NewBaseService(),
	}
	service.initCaptcha()
	return service
}

// initCaptcha 初始化验证码生成器
func (s *ClickTextCaptchaService) initCaptcha() {
	// 创建一个新的click builder对象，并设置其基本属性
	builder := click.NewBuilder(
		click.WithRangeLen(option.RangeVal{Min: 4, Max: 6}),
		click.WithRangeVerifyLen(option.RangeVal{Min: 2, Max: 4}),
		click.WithRangeThumbColors([]string{
			"#1f55c4", "#780592", "#2f6b00", "#910000", "#864401", "#675901", "#016e5c",
		}),
		click.WithRangeColors([]string{
			"#fde98e", "#60c1ff", "#fcb08e", "#fb88ff", "#b4fed4", "#cbfaa9", "#78d6f8",
		}),
	)

	// 加载字体资源
	fonts, err := fzshengsksjw.GetFont()
	if err != nil {
		log.Fatalln("加载字体失败:", err)
	}

	// 加载图片资源
	images, err := helper.GetImages()
	if err != nil {
		log.Fatalln("加载图片失败:", err)
	}

	// 设置builder的资源
	builder.SetResources(
		click.WithChars(chars.GetChineseChars()),
		click.WithFonts([]*truetype.Font{fonts}),
		click.WithBackgrounds(images),
	)

	// 生成基础验证码
	s.textCapt = builder.Make()

	// 清空builder配置
	builder.Clear()

	// 设置亮色主题配置
	builder.SetOptions(
		click.WithRangeLen(option.RangeVal{Min: 4, Max: 6}),
		click.WithRangeVerifyLen(option.RangeVal{Min: 2, Max: 4}),
		click.WithRangeThumbColors([]string{
			"#4a85fb", "#d93ffb", "#56be01", "#ee2b2b", "#cd6904", "#b49b03", "#01ad90",
		}),
	)

	// 重新设置资源
	builder.SetResources(
		click.WithChars(chars.GetChineseChars()),
		click.WithFonts([]*truetype.Font{fonts}),
		click.WithBackgrounds(images),
	)

	// 生成亮色主题验证码
	s.lightTextCapt = builder.Make()
}

// GenerateCaptcha 生成验证码
func (s *ClickTextCaptchaService) GenerateCaptcha(mode string) (*CaptchaData, *helper.CaptchaError) {
	// 选择验证码生成器
	var captcha click.Captcha
	if mode == "light" {
		captcha = s.lightTextCapt
	} else {
		captcha = s.textCapt
	}

	// 生成验证码数据
	captData, err := captcha.Generate()
	if err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeCaptchaGenError, "验证码生成失败", err.Error())
	}

	// 获取验证码数据块
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
func (s *ClickTextCaptchaService) GetServiceType() string {
	return "click-text"
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
func (s *ClickTextVerifyService) VerifyData(data, key string) (bool, string, *helper.CaptchaError) {
	return s.VerifyDataTemplate(
		data, key,
		s.ParseClickData,
		func(userData, correctData interface{}) bool {
			userDots := userData.([]map[string]interface{})
			return s.validator.ValidateClickData(userDots, correctData, 25.0)
		},
	)
}
