package middleware

import (
	"log"
	"simple-captcha/helper"
	"simple-captcha/models"

	"github.com/gin-gonic/gin"
)

// ValidateInterface 验证接口
type ValidateInterface interface {
	Validate() *helper.CaptchaError
}

// ValidationMiddleware 通用验证中间件
func ValidationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

// ValidateCaptchaRequest 验证验证码请求中间件
func ValidateCaptchaRequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req models.CaptchaRequest

		// 绑定URI参数
		if err := c.ShouldBindUri(&req); err != nil {
			log.Printf("URI参数绑定失败: error=%v, ip=%s", err, c.ClientIP())
			helper.SimpleErrorResponse(c, helper.ErrCodeInvalidParam, "验证码类型参数错误")
			c.Abort()
			return
		}

		// 绑定查询参数
		if err := c.ShouldBindQuery(&req); err != nil {
			log.Printf("查询参数绑定失败: error=%v, ip=%s", err, c.ClientIP())
			helper.SimpleErrorResponse(c, helper.ErrCodeInvalidParam, "查询参数错误")
			c.Abort()
			return
		}

		// 验证参数
		if err := req.Validate(); err != nil {
			log.Printf("参数验证失败: req=%+v, error=%v, ip=%s", req, err, c.ClientIP())
			helper.ErrorResponse(c, err)
			c.Abort()
			return
		}

		// 将验证后的请求对象存储到上下文中
		c.Set("captcha_request", &req)
		c.Next()
	}
}

// ValidateVerifyRequest 验证验证请求中间件
func ValidateVerifyRequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req models.VerifyRequest

		// 绑定URI参数
		if err := c.ShouldBindUri(&req); err != nil {
			log.Printf("URI参数绑定失败: error=%v, ip=%s", err, c.ClientIP())
			helper.SimpleErrorResponse(c, helper.ErrCodeInvalidParam, "路径参数错误")
			c.Abort()
			return
		}

		// 绑定JSON参数
		if err := c.ShouldBindJSON(&req); err != nil {
			log.Printf("JSON参数绑定失败: error=%v, ip=%s", err, c.ClientIP())
			helper.SimpleErrorResponse(c, helper.ErrCodeInvalidParam, "请求体参数错误")
			c.Abort()
			return
		}

		// 验证参数
		if err := req.Validate(); err != nil {
			log.Printf("参数验证失败: req=%+v, error=%v, ip=%s", req, err, c.ClientIP())
			helper.ErrorResponse(c, err)
			c.Abort()
			return
		}

		// 将验证后的请求对象存储到上下文中
		c.Set("verify_request", &req)
		c.Next()
	}
}

// ValidateStateRequest 验证状态查询请求中间件（GET，参数来自 URI）
func ValidateStateRequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req models.StateRequest

		// 绑定URI参数（GET 请求无 body）
		if err := c.ShouldBindUri(&req); err != nil {
			log.Printf("URI参数绑定失败: error=%v, ip=%s", err, c.ClientIP())
			helper.SimpleErrorResponse(c, helper.ErrCodeInvalidParam, "路径参数错误")
			c.Abort()
			return
		}

		// 验证参数
		if err := req.Validate(); err != nil {
			log.Printf("参数验证失败: req=%+v, error=%v, ip=%s", req, err, c.ClientIP())
			helper.ErrorResponse(c, err)
			c.Abort()
			return
		}

		// 将验证后的请求对象存储到上下文中
		c.Set("state_request", &req)
		c.Next()
	}
}

// GetCaptchaRequest 从上下文中获取验证码请求
func GetCaptchaRequest(c *gin.Context) *models.CaptchaRequest {
	if req, exists := c.Get("captcha_request"); exists {
		return req.(*models.CaptchaRequest)
	}
	return nil
}

// GetVerifyRequest 从上下文中获取验证请求
func GetVerifyRequest(c *gin.Context) *models.VerifyRequest {
	if req, exists := c.Get("verify_request"); exists {
		return req.(*models.VerifyRequest)
	}
	return nil
}

// GetStateRequest 从上下文中获取状态查询请求
func GetStateRequest(c *gin.Context) *models.StateRequest {
	if req, exists := c.Get("state_request"); exists {
		return req.(*models.StateRequest)
	}
	return nil
}
