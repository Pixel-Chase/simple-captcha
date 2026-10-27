package middleware

import (
	"errors"
	"io"
	"log"
	"net/http"
	"simple-captcha/config"
	"simple-captcha/helper"
	"simple-captcha/models"

	"github.com/gin-gonic/gin"
)

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

// ValidateIssueRequest 验证出题请求（POST）中间件：URI 携带题型，JSON 请求体携带 mode / dpr / env。
// 空请求体视为未携带环境信号，与 GET 出题等价
func ValidateIssueRequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req models.IssueRequest

		if err := c.ShouldBindUri(&req); err != nil {
			log.Printf("URI参数绑定失败: error=%v, ip=%s", err, c.ClientIP())
			helper.SimpleErrorResponse(c, helper.ErrCodeInvalidParam, "验证码类型参数错误")
			c.Abort()
			return
		}

		// 出题请求体只有环境信号，体积有限，沿用校验接口的上限并在读取阶段硬性截断
		if limit := config.LoadConfig().Security.VerifyMaxBodySize; limit > 0 && c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		}

		if c.Request.Body != nil && c.Request.ContentLength != 0 {
			if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
				var tooLarge *http.MaxBytesError
				if errors.As(err, &tooLarge) {
					log.Printf("请求体超出上限: limit=%d, ip=%s", tooLarge.Limit, c.ClientIP())
					helper.SimpleErrorResponse(c, http.StatusRequestEntityTooLarge, "请求体过大")
					c.Abort()
					return
				}
				log.Printf("JSON参数绑定失败: error=%v, ip=%s", err, c.ClientIP())
				helper.SimpleErrorResponse(c, helper.ErrCodeInvalidParam, "请求体参数错误")
				c.Abort()
				return
			}
		}

		if err := req.Validate(); err != nil {
			log.Printf("参数验证失败: type=%s, mode=%s, error=%v, ip=%s", req.Type, req.Mode, err, c.ClientIP())
			helper.ErrorResponse(c, err)
			c.Abort()
			return
		}

		c.Set("issue_request", &req)
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

		// 校验提交的请求体由轨迹点与环境信号构成，体积有限，在全局上限之外单独收紧，
		// 避免攻击者用超大 JSON 拖慢解析与评分
		if limit := config.LoadConfig().Security.VerifyMaxBodySize; limit > 0 && c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		}

		// 绑定JSON参数
		if err := c.ShouldBindJSON(&req); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				log.Printf("请求体超出上限: limit=%d, ip=%s", tooLarge.Limit, c.ClientIP())
				helper.SimpleErrorResponse(c, http.StatusRequestEntityTooLarge, "请求体过大")
				c.Abort()
				return
			}
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

// GetIssueRequest 从上下文中获取出题请求（POST）
func GetIssueRequest(c *gin.Context) *models.IssueRequest {
	if req, exists := c.Get("issue_request"); exists {
		return req.(*models.IssueRequest)
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
