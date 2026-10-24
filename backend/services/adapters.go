package services

import (
	"simple-captcha/helper"
)

// 以下适配器按需惰性初始化底层服务，保留给外部代码（含测试）引用。

// ClickTextService 点击文字验证码服务适配器
type ClickTextService struct {
	service *ClickTextCaptchaService
}

func (s *ClickTextService) GenerateCaptcha(req *ChallengeRequest) (*CaptchaData, *helper.CaptchaError) {
	if s.service == nil {
		s.service = NewClickTextCaptchaService()
	}
	return s.service.GenerateCaptcha(req)
}

func (s *ClickTextService) GetServiceType() string {
	return "click-text"
}

// ClickShapeService 点击形状验证码服务适配器
type ClickShapeService struct {
	service *ClickShapeCaptchaService
}

func (s *ClickShapeService) GenerateCaptcha(req *ChallengeRequest) (*CaptchaData, *helper.CaptchaError) {
	if s.service == nil {
		s.service = NewClickShapeCaptchaService()
	}
	return s.service.GenerateCaptcha(req)
}

func (s *ClickShapeService) GetServiceType() string {
	return "click-shape"
}

// RotateService 旋转验证码服务适配器
type RotateService struct {
	service *RotateCaptchaService
}

func (s *RotateService) GenerateCaptcha(req *ChallengeRequest) (*CaptchaData, *helper.CaptchaError) {
	if s.service == nil {
		s.service = NewRotateCaptchaService()
	}
	return s.service.GenerateCaptcha(req)
}

func (s *RotateService) GetServiceType() string {
	return "rotate"
}

// SlideTextService 滑动文字验证码服务适配器
type SlideTextService struct {
	service *SlideTextCaptchaService
}

func (s *SlideTextService) GenerateCaptcha(req *ChallengeRequest) (*CaptchaData, *helper.CaptchaError) {
	if s.service == nil {
		s.service = NewSlideTextCaptchaService()
	}
	return s.service.GenerateCaptcha(req)
}

func (s *SlideTextService) GetServiceType() string {
	return "slide-text"
}

// SlideRegionService 滑动区域验证码服务适配器
type SlideRegionService struct {
	service *SlideRegionCaptchaService
}

func (s *SlideRegionService) GenerateCaptcha(req *ChallengeRequest) (*CaptchaData, *helper.CaptchaError) {
	if s.service == nil {
		s.service = NewSlideRegionCaptchaService()
	}
	return s.service.GenerateCaptcha(req)
}

func (s *SlideRegionService) GetServiceType() string {
	return "slide-region"
}

// ClickVerifyService 点击验证服务适配器
type ClickVerifyService struct {
	service *ClickTextVerifyService
}

func (s *ClickVerifyService) VerifyData(in *VerifyInput) (bool, string, *helper.CaptchaError) {
	if s.service == nil {
		s.service = NewClickTextVerifyService()
	}
	return s.service.VerifyData(in)
}

// ClickShapeVerifyServiceAdapter 点击形状验证服务适配器
type ClickShapeVerifyServiceAdapter struct {
	service *ClickShapeVerifyService
}

func (s *ClickShapeVerifyServiceAdapter) VerifyData(in *VerifyInput) (bool, string, *helper.CaptchaError) {
	if s.service == nil {
		s.service = NewClickShapeVerifyService()
	}
	return s.service.VerifyData(in)
}

// RotateVerifyService 旋转验证服务适配器
type RotateVerifyService struct {
	service *RotateVerifyServiceImpl
}

func (s *RotateVerifyService) VerifyData(in *VerifyInput) (bool, string, *helper.CaptchaError) {
	if s.service == nil {
		s.service = NewRotateVerifyServiceImpl()
	}
	return s.service.VerifyData(in)
}

// SlideVerifyService 滑动验证服务适配器
type SlideVerifyService struct {
	service *SlideRegionVerifyServiceImpl
}

func (s *SlideVerifyService) VerifyData(in *VerifyInput) (bool, string, *helper.CaptchaError) {
	if s.service == nil {
		s.service = NewSlideRegionVerifyServiceImpl()
	}
	return s.service.VerifyData(in)
}

// SlideTextVerifyService 滑动文字验证服务适配器
type SlideTextVerifyService struct {
	service *SlideTextVerifyServiceImpl
}

func (s *SlideTextVerifyService) VerifyData(in *VerifyInput) (bool, string, *helper.CaptchaError) {
	if s.service == nil {
		s.service = NewSlideTextVerifyServiceImpl()
	}
	return s.service.VerifyData(in)
}
