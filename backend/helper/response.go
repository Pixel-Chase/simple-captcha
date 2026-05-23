package helper

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// SuccessResponse 成功响应 (纯净 RESTful: 直接返回 200 及 Data Payload)
func SuccessResponse(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, data)
}

// ErrorResponse 错误响应 (纯净 RESTful: 根据 Error 级别返回具体 HTTP 状态码并结构化错误信息)
func ErrorResponse(c *gin.Context, err *CaptchaError) {
	statusCode := HTTPStatusFromError(err)
	c.JSON(statusCode, gin.H{
		"code":  err.Code,
		"error": err.Message,
	})
}

// ErrorResponseWithDetail 带详细信息的错误响应
func ErrorResponseWithDetail(c *gin.Context, err *CaptchaError, detail string) {
	statusCode := HTTPStatusFromError(err)
	c.JSON(statusCode, gin.H{
		"code":  err.Code,
		"error": err.Message + ": " + detail,
	})
}

// SimpleErrorResponse 简单错误响应
func SimpleErrorResponse(c *gin.Context, code int, message string) {
	var statusCode int
	switch code {
	case 400:
		statusCode = http.StatusBadRequest
	case 401:
		statusCode = http.StatusUnauthorized
	case 403:
		statusCode = http.StatusForbidden
	case 429:
		statusCode = http.StatusTooManyRequests
	case 500:
		statusCode = http.StatusInternalServerError
	default:
		statusCode = http.StatusOK
	}

	c.JSON(statusCode, gin.H{
		"code":  code,
		"error": message,
	})
}
