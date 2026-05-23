package services

import (
	"log"
	"simple-captcha/helper"

	"github.com/wenlng/go-captcha-assets/resources/tiles"
	"github.com/wenlng/go-captcha/v2/slide"
)

// SlideTextCaptchaService 滑动文字验证码服务
type SlideTextCaptchaService struct {
	*BaseService
	slideCapt slide.Captcha
}

// NewSlideTextCaptchaService 创建滑动文字验证码服务
func NewSlideTextCaptchaService() *SlideTextCaptchaService {
	service := &SlideTextCaptchaService{
		BaseService: NewBaseService(),
	}
	service.initCaptcha()
	return service
}

// initCaptcha 初始化验证码生成器
func (s *SlideTextCaptchaService) initCaptcha() {
	// 创建一个新的构建器实例，启用垂直随机布局
	builder := slide.NewBuilder(
		slide.WithEnableGraphVerticalRandom(true),
	)

	// 获取并检查图像资源，如果获取失败则退出程序
	images, err := helper.GetImages()
	if err != nil {
		log.Fatalln("加载图片资源失败:", err)
	}

	// 获取并检查瓷砖资源，如果获取失败则退出程序
	graphs, err := tiles.GetTiles()
	if err != nil {
		log.Fatalln("加载瓷砖资源失败:", err)
	}

	// 初始化一个新的切片来存储处理后的图形图像
	var newGraphs = make([]*slide.GraphImage, 0, len(graphs))
	// 处理每个图形，创建并添加新的图形图像到切片中
	for i := 0; i < len(graphs); i++ {
		graph := graphs[i]
		newGraphs = append(newGraphs, &slide.GraphImage{
			OverlayImage: graph.OverlayImage,
			MaskImage:    graph.MaskImage,
			ShadowImage:  graph.ShadowImage,
		})
	}

	// 设置资源
	builder.SetResources(
		slide.WithGraphImages(newGraphs),
		slide.WithBackgrounds(images),
	)

	// 生成基础滑动验证码资源
	s.slideCapt = builder.Make()
}

// GenerateCaptcha 生成验证码
func (s *SlideTextCaptchaService) GenerateCaptcha(mode string) (*CaptchaData, *helper.CaptchaError) {
	// 生成基础滑动验证码数据
	captData, err := s.slideCapt.Generate()
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

	tileImageBase64, err := captData.GetTileImage().ToBase64()
	if err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeCaptchaGenError, "滑动块图片转换失败", err.Error())
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
		Key:    key,
		Image:  masterImageBase64,
		Tile:   tileImageBase64,
		Width:  blockData.Width,
		Height: blockData.Height,
		X:      blockData.TileX,
		Y:      blockData.TileY,
	}, nil
}

// GetServiceType 获取服务类型
func (s *SlideTextCaptchaService) GetServiceType() string {
	return "slide-text"
}

// SlideTextVerifyServiceImpl 滑动文字验证服务实现
type SlideTextVerifyServiceImpl struct {
	*BaseVerifyService
	validator *ValidationUtils
}

// NewSlideTextVerifyServiceImpl 创建滑动文字验证服务实现
func NewSlideTextVerifyServiceImpl() *SlideTextVerifyServiceImpl {
	return &SlideTextVerifyServiceImpl{
		BaseVerifyService: NewBaseVerifyService(),
		validator:         NewValidationUtils(),
	}
}

// VerifyData 验证数据
func (s *SlideTextVerifyServiceImpl) VerifyData(data, key string) (bool, string, *helper.CaptchaError) {
	return s.VerifyDataTemplate(
		data, key,
		s.ParseSlideData,
		func(userData, correctData interface{}) bool {
			userSlide := userData.(map[string]interface{})
			return s.validator.ValidateSlidePosition(userSlide, correctData, 6.0)
		},
	)
}
