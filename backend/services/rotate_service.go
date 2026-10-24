package services

import (
	"simple-captcha/helper"

	"github.com/wenlng/go-captcha/v2/rotate"
)

// 1 倍率下旋转内圆可选尺寸
var baseRotateThumbSizes = []int{140, 150, 160, 170}

// RotateCaptchaService 旋转验证码服务
type RotateCaptchaService struct {
	*BaseService
	backgrounds *BackgroundStore
	instances   *instanceCache[rotate.Captcha]
}

// NewRotateCaptchaService 创建旋转验证码服务
func NewRotateCaptchaService(backgrounds *BackgroundStore) *RotateCaptchaService {
	service := &RotateCaptchaService{
		BaseService: NewBaseService(),
		backgrounds: backgrounds,
	}
	service.instances = newInstanceCache(service.build)
	// 素材热重载后丢弃已构建的生成器实例，下一次出题使用新素材
	backgrounds.OnReload(service.instances.reset)
	return service
}

// build 按难度 × 倍率构建生成器实例
func (s *RotateCaptchaService) build(key instanceKey) rotate.Captcha {
	profile := GetDifficultyProfile(key.level)

	square := baseRotateSize * key.scale
	builder := rotate.NewBuilder(
		rotate.WithImageSquareSize(square),
		rotate.WithRangeThumbImageSquareSize(scaleInts(baseRotateThumbSizes, key.scale)),
		rotate.WithRangeAnglePos(profile.RotateAngles),
	)
	builder.SetResources(
		rotate.WithImages(s.backgrounds.Get(square, square)),
	)
	return builder.Make()
}

// GenerateCaptcha 生成验证码
func (s *RotateCaptchaService) GenerateCaptcha(req *ChallengeRequest) (*CaptchaData, *helper.CaptchaError) {
	req = s.normalizeRequest(req, s.GetServiceType())
	captcha := s.instances.get(req.Difficulty, req.Scale, "")

	captData, err := captcha.Generate()
	if err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeCaptchaGenError, "验证码生成失败", err.Error())
	}

	blockData := captData.GetData()
	if blockData == nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeCaptchaGenError, "验证码数据为空")
	}

	// 旋转主图中心为透明圆孔，必须保留 Alpha 通道，因此沿用 PNG
	masterImageBase64, err := captData.GetMasterImage().ToBase64()
	if err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeCaptchaGenError, "主图片转换失败", err.Error())
	}

	thumbImageBase64, err := captData.GetThumbImage().ToBase64()
	if err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeCaptchaGenError, "缩略图转换失败", err.Error())
	}

	key, meta, persistErr := s.persistChallenge(req, blockData)
	if persistErr != nil {
		return nil, persistErr
	}

	square := baseRotateSize * req.Scale
	return &CaptchaData{
		Key:         key,
		Image:       masterImageBase64,
		Thumb:       thumbImageBase64,
		ImageWidth:  square,
		ImageHeight: square,
		ThumbWidth:  blockData.Width,
		ThumbHeight: blockData.Height,
		Scale:       req.Scale,
		Difficulty:  req.Difficulty,
		PowNonce:    meta.PowNonce,
		PowBits:     meta.PowBits,
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
func (s *RotateVerifyServiceImpl) VerifyData(in *VerifyInput) (bool, string, *helper.CaptchaError) {
	return s.VerifyDataTemplate(
		in,
		s.ParseRotateData,
		func(userData, correctData interface{}, meta *ChallengeMeta) bool {
			userAngle, ok := userData.(map[string]interface{})
			if !ok {
				return false
			}
			return s.validator.ValidateRotateAngle(userAngle, correctData, RotateToleranceFor(meta))
		},
	)
}
