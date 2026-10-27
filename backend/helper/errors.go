package helper

import (
	"fmt"
	"net/http"
)

// 定义错误码常量
const (
	ErrCodeInvalidParam     = 400
	ErrCodeTooManyRequests  = 429
	ErrCodeInternalError    = 500
	ErrCodeCaptchaDataError = 1001 // 题目或二次验证数据（进程内存）序列化、解析失败
	ErrCodeCaptchaGenError  = 1002
	ErrCodeVerifyFailed     = 1003
	ErrCodeExpired          = 1004
	ErrCodeTooManyAttempts  = 1005
	ErrCodeTooFast          = 1006 // 出题到提交间隔过短，疑似自动化
	ErrCodeClientMismatch   = 1007 // 提交客户端与出题客户端不一致
	ErrCodeRiskBlocked      = 1008 // 风控评分过高被拦截（仅用于服务端标识；对外统一返回 ErrCodeVerifyFailed，避免成为调参探针）
	ErrCodePowInvalid       = 1009 // 工作量证明缺失或无效
	ErrCodeAppInvalid       = 1010 // appId 对应的应用不存在或已停用
	ErrCodeOriginNotAllowed = 1011 // 请求来源（Origin）不在应用的允许名单内，或缺少 Origin

	// 管理后台错误码（2xxx）
	ErrCodeUnauthorized       = 2001 // 未登录或会话已过期
	ErrCodeLoginFailed        = 2002 // 用户名或密码错误
	ErrCodeCaptchaRequired    = 2003 // 登录前置行为验证缺失或已失效
	ErrCodeInvalidImage       = 2004 // 上传文件不是受支持的图片或不满足尺寸要求
	ErrCodeImageTooLarge      = 2005 // 上传体积超限
	ErrCodeLastBackground     = 2006 // 至少需要保留一张背景图
	ErrCodeBackgroundNotFound = 2007 // 背景图不存在
	// 2008 原为"尚未修改初始密码"，随强制改密一并删除，编号不再复用
	ErrCodeWrongPassword      = 2009 // 修改密码时当前密码错误
	ErrCodeAppNotFound        = 2010 // 接入应用不存在
	ErrCodeAppNameConflict    = 2011 // 接入应用名称重复
	ErrCodeNotInitialized     = 2012 // 尚未初始化管理员账号，控制台据此切换到初始化页面
	ErrCodeAlreadyInitialized = 2013 // 已初始化，不能再次初始化
	ErrCodeSetupCodeInvalid   = 2014 // 初始化口令错误（见服务启动日志）
	ErrCodeSystemAppReadonly  = 2015 // 内置系统应用不可删除、停用、改名、轮换密钥或修改来源

	// S2S 接口错误码（3xxx）
	ErrCodeAppUnauthorized = 3001 // 应用密钥缺失、无效或应用已停用
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
	ErrVerifyFailed   = NewCaptchaError(ErrCodeVerifyFailed, "验证失败")
	ErrTooFast        = NewCaptchaError(ErrCodeTooFast, "操作过快，请重试")
	ErrClientMismatch = NewCaptchaError(ErrCodeClientMismatch, "验证码已失效，请重新获取")
	ErrPowInvalid     = NewCaptchaError(ErrCodePowInvalid, "验证失败，请刷新后重试")

	ErrUnauthorized       = NewCaptchaError(ErrCodeUnauthorized, "未登录或会话已过期")
	ErrLoginFailed        = NewCaptchaError(ErrCodeLoginFailed, "用户名或密码错误")
	ErrCaptchaRequired    = NewCaptchaError(ErrCodeCaptchaRequired, "请先完成行为验证")
	ErrLastBackground     = NewCaptchaError(ErrCodeLastBackground, "至少需要保留一张背景图")
	ErrBackgroundNotFound = NewCaptchaError(ErrCodeBackgroundNotFound, "背景图不存在")
	ErrWrongPassword      = NewCaptchaError(ErrCodeWrongPassword, "当前密码错误")
	ErrAppNotFound        = NewCaptchaError(ErrCodeAppNotFound, "应用不存在")
	ErrAppNameConflict    = NewCaptchaError(ErrCodeAppNameConflict, "已存在同名应用")
	ErrNotInitialized     = NewCaptchaError(ErrCodeNotInitialized, "管理后台尚未初始化")
	ErrAlreadyInitialized = NewCaptchaError(ErrCodeAlreadyInitialized, "管理后台已初始化")
	ErrSetupCodeInvalid   = NewCaptchaError(ErrCodeSetupCodeInvalid, "初始化口令错误")
	ErrSystemAppReadonly  = NewCaptchaError(ErrCodeSystemAppReadonly, "系统应用不可修改")
	ErrAppInvalid         = NewCaptchaError(ErrCodeAppInvalid, "应用不存在或已停用")
	ErrOriginNotAllowed   = NewCaptchaError(ErrCodeOriginNotAllowed, "当前来源不允许访问该应用")

	ErrAppUnauthorized = NewCaptchaError(ErrCodeAppUnauthorized, "应用密钥缺失或无效")
)

// HTTPStatusFromError 根据错误码返回HTTP状态码
func HTTPStatusFromError(err *CaptchaError) int {
	switch err.Code {
	case ErrCodeInvalidParam:
		return http.StatusBadRequest
	case ErrCodeTooManyRequests:
		return http.StatusTooManyRequests
	case ErrCodeInternalError, ErrCodeCaptchaDataError, ErrCodeCaptchaGenError:
		return http.StatusInternalServerError
	case ErrCodeUnauthorized, ErrCodeLoginFailed:
		return http.StatusUnauthorized
	case ErrCodeCaptchaRequired, ErrCodeInvalidImage, ErrCodeWrongPassword:
		return http.StatusBadRequest
	case ErrCodeSetupCodeInvalid, ErrCodeAppInvalid, ErrCodeOriginNotAllowed:
		return http.StatusForbidden
	case ErrCodeSystemAppReadonly:
		return http.StatusBadRequest
	case ErrCodeImageTooLarge:
		return http.StatusRequestEntityTooLarge
	case ErrCodeLastBackground, ErrCodeAppNameConflict, ErrCodeNotInitialized, ErrCodeAlreadyInitialized:
		return http.StatusConflict
	case ErrCodeBackgroundNotFound, ErrCodeAppNotFound:
		return http.StatusNotFound
	case ErrCodeAppUnauthorized:
		return http.StatusUnauthorized
	default:
		return http.StatusOK
	}
}
