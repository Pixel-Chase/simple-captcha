package helper

import (
	"fmt"
	"net/http"
)

// 定义错误码常量
const (
	ErrCodeSuccess         = 0
	ErrCodeInvalidParam    = 400
	ErrCodeTooManyRequests = 429
	ErrCodeInternalError   = 500
	ErrCodeCacheError      = 1001
	ErrCodeCaptchaGenError = 1002
	ErrCodeVerifyFailed    = 1003
	ErrCodeExpired         = 1004
	ErrCodeTooManyAttempts = 1005
	ErrCodeTooFast         = 1006 // 出题到提交间隔过短，疑似自动化
	ErrCodeClientMismatch  = 1007 // 提交客户端与出题客户端不一致
	ErrCodeRiskBlocked     = 1008 // 风控评分过高被拦截（仅用于服务端标识；对外统一返回 ErrCodeVerifyFailed，避免成为调参探针）
	ErrCodePowInvalid      = 1009 // 工作量证明缺失或无效

	// 管理后台错误码（2xxx）
	ErrCodeUnauthorized       = 2001 // 未登录或会话已过期
	ErrCodeLoginFailed        = 2002 // 用户名或密码错误
	ErrCodeCaptchaRequired    = 2003 // 登录前置行为验证缺失或已失效
	ErrCodeInvalidImage       = 2004 // 上传文件不是受支持的图片或不满足尺寸要求
	ErrCodeImageTooLarge      = 2005 // 上传体积超限
	ErrCodeLastBackground     = 2006 // 至少需要保留一张背景图
	ErrCodeBackgroundNotFound = 2007 // 背景图不存在
)

// CaptchaError 自定义错误类型
type CaptchaError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

func (e *CaptchaError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("code: %d, message: %s, detail: %s", e.Code, e.Message, e.Detail)
	}
	return fmt.Sprintf("code: %d, message: %s", e.Code, e.Message)
}

// NewCaptchaError 创建新的验证码错误
func NewCaptchaError(code int, message string, detail ...string) *CaptchaError {
	err := &CaptchaError{
		Code:    code,
		Message: message,
	}
	if len(detail) > 0 {
		err.Detail = detail[0]
	}
	return err
}

// 预定义的错误
var (
	ErrInvalidParam    = NewCaptchaError(ErrCodeInvalidParam, "参数错误")
	ErrTooManyRequests = NewCaptchaError(ErrCodeTooManyRequests, "请求过于频繁，请稍后再试")
	ErrInternalError   = NewCaptchaError(ErrCodeInternalError, "内部服务器错误")
	ErrCacheError      = NewCaptchaError(ErrCodeCacheError, "缓存操作失败")
	ErrCaptchaGenError = NewCaptchaError(ErrCodeCaptchaGenError, "验证码生成失败")
	ErrVerifyFailed    = NewCaptchaError(ErrCodeVerifyFailed, "验证失败")
	ErrExpired         = NewCaptchaError(ErrCodeExpired, "验证码已过期")
	ErrTooManyAttempts = NewCaptchaError(ErrCodeTooManyAttempts, "验证尝试次数过多")
	ErrTooFast         = NewCaptchaError(ErrCodeTooFast, "操作过快，请重试")
	ErrClientMismatch  = NewCaptchaError(ErrCodeClientMismatch, "验证码已失效，请重新获取")
	ErrRiskBlocked     = NewCaptchaError(ErrCodeRiskBlocked, "验证失败")
	ErrPowInvalid      = NewCaptchaError(ErrCodePowInvalid, "验证失败，请刷新后重试")

	ErrUnauthorized       = NewCaptchaError(ErrCodeUnauthorized, "未登录或会话已过期")
	ErrLoginFailed        = NewCaptchaError(ErrCodeLoginFailed, "用户名或密码错误")
	ErrCaptchaRequired    = NewCaptchaError(ErrCodeCaptchaRequired, "请先完成行为验证")
	ErrLastBackground     = NewCaptchaError(ErrCodeLastBackground, "至少需要保留一张背景图")
	ErrBackgroundNotFound = NewCaptchaError(ErrCodeBackgroundNotFound, "背景图不存在")
)

// HTTPStatusFromError 根据错误码返回HTTP状态码
func HTTPStatusFromError(err *CaptchaError) int {
	switch err.Code {
	case ErrCodeInvalidParam:
		return http.StatusBadRequest
	case ErrCodeTooManyRequests:
		return http.StatusTooManyRequests
	case ErrCodeInternalError, ErrCodeCacheError, ErrCodeCaptchaGenError:
		return http.StatusInternalServerError
	case ErrCodeUnauthorized, ErrCodeLoginFailed:
		return http.StatusUnauthorized
	case ErrCodeCaptchaRequired, ErrCodeInvalidImage:
		return http.StatusBadRequest
	case ErrCodeImageTooLarge:
		return http.StatusRequestEntityTooLarge
	case ErrCodeLastBackground:
		return http.StatusConflict
	case ErrCodeBackgroundNotFound:
		return http.StatusNotFound
	default:
		return http.StatusOK
	}
}
