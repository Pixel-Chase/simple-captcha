package services

import (
	"log"
	"simple-captcha/helper"

	"github.com/wenlng/go-captcha-assets/resources/tiles"
	"github.com/wenlng/go-captcha/v2/slide"
)

// SlideRegionCaptchaService 滑动区域验证码服务
type SlideRegionCaptchaService struct {
	*BaseService
	slideCapt slide.Captcha
}

// NewSlideRegionCaptchaService 创建滑动区域验证码服务
func NewSlideRegionCaptchaService() *SlideRegionCaptchaService {
	service := &SlideRegionCaptchaService{
		BaseService: NewBaseService(),
	}
	service.initCaptcha()
	return service
}

// initCaptcha 初始化验证码生成器
func (s *SlideRegionCaptchaService) initCaptcha() {
	// 创建一个slide.Builder实例，配置生成图形的数量和是否启用垂直随机布局
	builder := slide.NewBuilder(
		slide.WithGenGraphNumber(2),
		slide.WithEnableGraphVerticalRandom(true),
	)

	// 从images包中获取背景图像资源
	images, err := helper.GetImages()
	if err != nil {
		log.Fatalln("加载图片资源失败:", err)
	}

	// 从tiles包中获取瓷砖图形资源
	graphs, err := tiles.GetTiles()
	if err != nil {
		log.Fatalln("加载瓷砖资源失败:", err)
	}

	// 初始化一个新的GraphImage切片，用于存储处理后的瓷砖图形
	var newGraphs = make([]*slide.GraphImage, 0, len(graphs))
	// 遍历瓷砖图形资源，创建并添加新的GraphImage到切片中
	for i := 0; i < len(graphs); i++ {
		graph := graphs[i]
		newGraphs = append(newGraphs, &slide.GraphImage{
			OverlayImage: graph.OverlayImage,
			MaskImage:    graph.MaskImage,
			ShadowImage:  graph.ShadowImage,
		})
	}

	// 将处理后的图形资源和背景图像设置到Builder中
	builder.SetResources(
		slide.WithGraphImages(newGraphs),
		slide.WithBackgrounds(images),
	)

	// 使用Builder生成最终的滑动验证图形
	s.slideCapt = builder.MakeWithRegion()
}

// GenerateCaptcha 生成验证码
func (s *SlideRegionCaptchaService) GenerateCaptcha(mode string) (*CaptchaData, *helper.CaptchaError) {
	// 生成滑动区域验证码数据
	captData, err := s.slideCapt.Generate()
	if err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeCaptchaGenError, "验证码生成失败", err.Error())
	}

	// 获取生成的验证码数据块
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
		return nil, helper.NewCaptchaError(helper.ErrCodeCaptchaGenError, "瓷砖图片转换失败", err.Error())
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
func (s *SlideRegionVerifyServiceImpl) VerifyData(data, key string) (bool, string, *helper.CaptchaError) {
	return s.VerifyDataTemplate(
		data, key,
		s.ParseSlideData,
		func(userData, correctData interface{}) bool {
			userSlide := userData.(map[string]interface{})
			return s.validator.ValidateSlidePosition(userSlide, correctData, 6.0)
		},
	)
}
