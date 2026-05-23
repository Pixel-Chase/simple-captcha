package controllers

import (
	"log"
	"net/http"
	"simple-captcha/helper"
	"simple-captcha/middleware"
	"simple-captcha/models"
	"simple-captcha/services"

	"github.com/gin-gonic/gin"
)

// CaptchaController 验证码控制器
type CaptchaController struct {
	serviceManager services.ServiceManager
	riskControl    services.RiskControlService
}

// NewCaptchaController 创建验证码控制器
func NewCaptchaController() *CaptchaController {
	// 初始化服务管理器
	serviceManager := services.NewServiceManager()

	// 获取风险控制服务
	riskControl := serviceManager.GetRiskControlService()

	return &CaptchaController{
		serviceManager: serviceManager,
		riskControl:    riskControl,
	}
}

// GetCaptchaHandler 生成验证码处理器
func (ctrl *CaptchaController) GetCaptchaHandler(c *gin.Context) {
	// 从中间件获取已验证的请求参数
	req := middleware.GetCaptchaRequest(c)
	if req == nil {
		log.Printf("获取请求参数失败: ip=%s", helper.GetRealClientIP(c))
		helper.SimpleErrorResponse(c, helper.ErrCodeInternalError, "内部错误")
		return
	}

	// 获取验证码服务
	captchaService, exists := ctrl.serviceManager.GetCaptchaService(req.Type)
	if !exists {
		log.Printf("不支持的验证码类型: type=%s, ip=%s", req.Type, helper.GetRealClientIP(c))
		helper.SimpleErrorResponse(c, helper.ErrCodeInvalidParam, "不支持的验证码类型")
		return
	}

	// 生成验证码
	captData, err := captchaService.GenerateCaptcha(req.Mode)
	if err != nil {
		log.Printf("生成验证码失败: type=%s, mode=%s, ip=%s, error=%v",
			req.Type, req.Mode, helper.GetRealClientIP(c), err)
		helper.ErrorResponse(c, err)
		return
	}

	// 记录成功生成日志
	log.Printf("生成验证码成功: type=%s, mode=%s, key=%s, ip=%s",
		req.Type, req.Mode, captData.Key, helper.GetRealClientIP(c))

	// 构建响应数据
	response := ctrl.buildCaptchaResponse(req.Type, captData)

	// 设置防缓存头，确保浏览器不会缓存验证码图片
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	helper.SuccessResponse(c, response)
}

// VerifyCaptchaHandler 验证验证码处理器
func (ctrl *CaptchaController) VerifyCaptchaHandler(c *gin.Context) {
	// 从中间件获取已验证的请求参数
	req := middleware.GetVerifyRequest(c)
	if req == nil {
		log.Printf("获取请求参数失败: ip=%s", helper.GetRealClientIP(c))
		helper.SimpleErrorResponse(c, helper.ErrCodeInternalError, "内部错误")
		return
	}

	// 获取验证服务
	verifyService, exists := ctrl.serviceManager.GetVerifyService(req.Type)
	if !exists {
		log.Printf("不支持的验证类型: type=%s, ip=%s", req.Type, helper.GetRealClientIP(c))
		helper.SimpleErrorResponse(c, helper.ErrCodeInvalidParam, "不支持的验证类型")
		return
	}

	// 执行验证
	verifyData := ctrl.extractVerifyData(req)
	isValid, secondKey, err := verifyService.VerifyData(verifyData, req.Key)

	if err != nil {
		log.Printf("验证过程出错: type=%s, key=%s, ip=%s, error=%v",
			req.Type, req.Key, helper.GetRealClientIP(c), err)
		helper.ErrorResponse(c, err)
		return
	}

	if !isValid {
		log.Printf("验证失败: type=%s, key=%s, ip=%s, message=%s",
			req.Type, req.Key, helper.GetRealClientIP(c), secondKey)
		helper.SimpleErrorResponse(c, helper.ErrCodeVerifyFailed, secondKey)
		return
	}

	// 验证成功
	log.Printf("验证成功: type=%s, key=%s, secondKey=%s, ip=%s",
		req.Type, req.Key, secondKey, helper.GetRealClientIP(c))

	response := models.VerifyResponse{
		SecondKey: secondKey,
	}
	helper.SuccessResponse(c, response)
}

// ValidateCaptchaHandler 验证码状态查询（二次验证）处理器
func (ctrl *CaptchaController) ValidateCaptchaHandler(c *gin.Context) {
	// 从中间件获取已验证的请求参数
	req := middleware.GetStateRequest(c)
	if req == nil {
		log.Printf("获取请求参数失败: ip=%s", helper.GetRealClientIP(c))
		helper.SimpleErrorResponse(c, helper.ErrCodeInternalError, "内部错误")
		return
	}

	// 验证二次验证令牌
	if !ctrl.riskControl.ValidateSecondVerifyToken(req.Key) {
		log.Printf("二次验证令牌无效: key=%s, ip=%s", req.Key, helper.GetRealClientIP(c))
		helper.SimpleErrorResponse(c, helper.ErrCodeExpired, "验证码已失效")
		return
	}

	log.Printf("状态查询成功: key=%s, ip=%s", req.Key, helper.GetRealClientIP(c))

	// 返回成功响应 (纯净 RESTful)
	c.JSON(http.StatusOK, gin.H{
		"valid": true,
	})
}

// buildCaptchaResponse 构建验证码响应
func (ctrl *CaptchaController) buildCaptchaResponse(captchaType string, captData *services.CaptchaData) models.CaptchaResponse {
	response := models.CaptchaResponse{
		Key:   captData.Key,
		Image: captData.Image,
	}

	// 根据验证码类型设置不同的响应字段
	if captchaType == "slide-region" || captchaType == "slide-text" {
		response.Thumb = captData.Tile
		response.ThumbWidth = captData.Width
		response.ThumbHeight = captData.Height
		response.ThumbX = captData.X
		response.ThumbY = captData.Y
	} else {
		response.Thumb = captData.Thumb
	}

	return response
}

// extractVerifyData 提取验证数据
func (ctrl *CaptchaController) extractVerifyData(req *models.VerifyRequest) string {
	switch req.Type {
	case "click-text", "click-shape":
		return req.Dots
	case "rotate":
		return req.Angle
	case "slide-region", "slide-text":
		return req.Point
	default:
		return ""
	}
}

// 为了保持向后兼容，保留原有的函数签名
var (
	captchaController = NewCaptchaController()
)

// GetCaptchaHandler 全局函数（向后兼容）
func GetCaptchaHandler(c *gin.Context) {
	captchaController.GetCaptchaHandler(c)
}

// VerifyCaptchaHandler 全局函数（向后兼容）
func VerifyCaptchaHandler(c *gin.Context) {
	captchaController.VerifyCaptchaHandler(c)
}

// ValidateCaptchaHandler 全局函数（向后兼容）
func ValidateCaptchaHandler(c *gin.Context) {
	captchaController.ValidateCaptchaHandler(c)
}
