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
	default:
		return http.StatusOK
	}
}
