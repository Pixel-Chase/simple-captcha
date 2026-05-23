package models

import (
	"errors"
	"simple-captcha/helper"

	"github.com/go-playground/validator/v10"
)

// CaptchaRequest 验证码请求结构
type CaptchaRequest struct {
	Type string `uri:"type" binding:"required,oneof=click-text click-shape rotate slide-text slide-region"`
	Mode string `form:"mode" binding:"omitempty,oneof=light dark"`
}

// VerifyRequest 验证请求结构
type VerifyRequest struct {
	Type  string `uri:"type" binding:"required,oneof=click-text click-shape rotate slide-text slide-region"`
	Key   string `uri:"key" binding:"required,min=1,max=100"`
	Dots  string `json:"dots" binding:"omitempty"`  // 点选验证码的参数
	Angle string `json:"angle" binding:"omitempty"` // 旋转验证码的参数
	Point string `json:"point" binding:"omitempty"` // 滑动验证码的参数
}

// StateRequest 状态查询请求结构
type StateRequest struct {
	Key string `uri:"key" binding:"required,min=1,max=100"`
}

// CaptchaResponse 验证码响应结构
type CaptchaResponse struct {
	Key         string `json:"key"`
	Image       string `json:"image"`
	Thumb       string `json:"thumb,omitempty"`
	ThumbWidth  int    `json:"thumbWidth,omitempty"`
	ThumbHeight int    `json:"thumbHeight,omitempty"`
	ThumbX      int    `json:"thumbX,omitempty"`
	ThumbY      int    `json:"thumbY,omitempty"`
}

// VerifyResponse 验证响应结构
type VerifyResponse struct {
	SecondKey string `json:"second_key"`
}

// StateResponse 状态响应结构
type StateResponse struct {
	Verified   bool  `json:"verified"`
	VerifyTime int64 `json:"verify_time"`
	ExpireAt   int64 `json:"expire_at"`
}

// ValidateStruct 验证结构体
func ValidateStruct(s interface{}) error {
	validate := validator.New()
	return validate.Struct(s)
}

// ValidateVerifyRequest 验证验证请求的业务逻辑
func (v *VerifyRequest) ValidateVerifyRequest() error {
	switch v.Type {
	case "click-text", "click-shape":
		if v.Dots == "" {
			return errors.New("点击验证码缺少坐标参数")
		}
	case "rotate":
		if v.Angle == "" {
			return errors.New("旋转验证码缺少角度参数")
		}
	case "slide-region", "slide-text":
		if v.Point == "" {
			return errors.New("滑动验证码缺少坐标参数")
		}
	}
	return nil
}

// Validate 验证请求参数（包含结构验证和业务逻辑验证）
func (v *VerifyRequest) Validate() *helper.CaptchaError {
	// 结构验证
	if err := ValidateStruct(v); err != nil {
		return helper.NewCaptchaError(helper.ErrCodeInvalidParam, "参数格式错误", err.Error())
	}

	// 业务逻辑验证
	if err := v.ValidateVerifyRequest(); err != nil {
		return helper.NewCaptchaError(helper.ErrCodeInvalidParam, "参数验证失败", err.Error())
	}

	return nil
}

// Validate 验证验证码请求参数
func (c *CaptchaRequest) Validate() *helper.CaptchaError {
	if err := ValidateStruct(c); err != nil {
		return helper.NewCaptchaError(helper.ErrCodeInvalidParam, "参数格式错误", err.Error())
	}
	return nil
}

// Validate 验证状态查询请求参数
func (s *StateRequest) Validate() *helper.CaptchaError {
	if err := ValidateStruct(s); err != nil {
		return helper.NewCaptchaError(helper.ErrCodeInvalidParam, "参数格式错误", err.Error())
	}
	return nil
}
