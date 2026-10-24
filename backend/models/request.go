package models

import (
	"errors"
	"math"
	"simple-captcha/helper"
	"simple-captcha/risk"
	"sync"

	"github.com/go-playground/validator/v10"
)

// 遥测数据体积上限，防止超大请求体拖慢评分
const (
	maxTracePoints    = 600
	maxTraceClicks    = 50
	maxEnvString      = 512
	maxEnvListLength  = 32
	maxPowNonceLength = 64
)

// CaptchaRequest 验证码请求结构
type CaptchaRequest struct {
	Type string  `uri:"type" binding:"required,oneof=auto click-text click-shape rotate slide-text slide-region"`
	Mode string  `form:"mode" binding:"omitempty,oneof=light dark"`
	Dpr  float64 `form:"dpr" binding:"omitempty,min=1,max=3"` // 前端 devicePixelRatio，决定出图倍率
}

// Scale 把 devicePixelRatio 换算为整数出图倍率（1-3）
func (c *CaptchaRequest) Scale() int {
	if c.Dpr <= 0 {
		return 1
	}
	scale := int(math.Round(c.Dpr))
	if scale < 1 {
		return 1
	}
	if scale > 3 {
		return 3
	}
	return scale
}

// VerifyRequest 验证请求结构
type VerifyRequest struct {
	Type string `uri:"type" json:"-" binding:"required,oneof=click-text click-shape rotate slide-text slide-region"`
	Key  string `uri:"key" json:"-" binding:"required,min=1,max=100"`

	Dots  string `json:"dots" binding:"omitempty,max=512"` // 点选验证码的参数
	Angle string `json:"angle" binding:"omitempty,max=16"` // 旋转验证码的参数
	Point string `json:"point" binding:"omitempty,max=64"` // 滑动验证码的参数

	// 行为与环境遥测（可选，旧版 SDK 不携带）
	Trace *risk.Trace       `json:"trace,omitempty"`
	Env   *risk.Env         `json:"env,omitempty"`
	Pow   *risk.PowSolution `json:"pow,omitempty"`
}

// StateRequest 状态查询请求结构
type StateRequest struct {
	Key string `uri:"key" binding:"required,min=1,max=100"`
}

// PowChallenge 下发给前端的工作量证明挑战
type PowChallenge struct {
	Nonce      string `json:"nonce"`
	Difficulty int    `json:"difficulty"` // SHA-256 前导零位数
}

// CaptchaResponse 验证码响应结构
type CaptchaResponse struct {
	Key   string `json:"key"`
	Type  string `json:"type"` // 实际下发的题型（auto 解析后的结果）
	Image string `json:"image"`

	Thumb       string `json:"thumb,omitempty"`
	ThumbWidth  int    `json:"thumbWidth,omitempty"`
	ThumbHeight int    `json:"thumbHeight,omitempty"`
	ThumbX      int    `json:"thumbX,omitempty"`
	ThumbY      int    `json:"thumbY,omitempty"`

	// 主图位图尺寸，前端据此把 CSS 坐标换算回图像坐标
	ImageWidth  int `json:"imageWidth"`
	ImageHeight int `json:"imageHeight"`
	Scale       int `json:"scale"`
	Difficulty  int `json:"difficulty"`

	Pow *PowChallenge `json:"pow,omitempty"`
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

var (
	structValidator     *validator.Validate
	structValidatorOnce sync.Once
)

// getValidator 返回与 Gin 一致、按 binding 标签校验的实例
func getValidator() *validator.Validate {
	structValidatorOnce.Do(func() {
		structValidator = validator.New()
		structValidator.SetTagName("binding")
	})
	return structValidator
}

// ValidateStruct 验证结构体
func ValidateStruct(s interface{}) error {
	return getValidator().Struct(s)
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

	if v.Trace != nil {
		if len(v.Trace.Points) > maxTracePoints {
			return errors.New("轨迹采样点数量超出限制")
		}
		if len(v.Trace.Clicks) > maxTraceClicks {
			return errors.New("点击记录数量超出限制")
		}
	}

	if v.Env != nil {
		for _, s := range []string{v.Env.UA, v.Env.Platform, v.Env.Timezone, v.Env.WebGL, v.Env.Fingerprint} {
			if len(s) > maxEnvString {
				return errors.New("环境信息字段超出长度限制")
			}
		}
		if len(v.Env.Languages) > maxEnvListLength || len(v.Env.Automation) > maxEnvListLength {
			return errors.New("环境信息列表超出长度限制")
		}
	}

	if v.Pow != nil {
		if len(v.Pow.Nonce) > maxPowNonceLength || len(v.Pow.Solution) > risk.MaxPowSolutionLength {
			return errors.New("工作量证明参数格式错误")
		}
	}

	return nil
}

// Validate 验证请求参数（包含结构验证和业务逻辑验证）
func (v *VerifyRequest) Validate() *helper.CaptchaError {
	if err := ValidateStruct(v); err != nil {
		return helper.NewCaptchaError(helper.ErrCodeInvalidParam, "参数格式错误", err.Error())
	}

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
