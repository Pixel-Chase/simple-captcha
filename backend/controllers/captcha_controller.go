package controllers

import (
	"log"
	"net/http"
	"simple-captcha/config"
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
	riskEngine     *services.RiskEngine
	policy         *services.ChallengePolicy
	config         *config.Config
}

// NewCaptchaController 创建验证码控制器（复用全局服务管理器，避免重复加载字体与图片资源）
func NewCaptchaController() *CaptchaController {
	serviceManager := services.GetServiceManager()

	return &CaptchaController{
		serviceManager: serviceManager,
		riskControl:    serviceManager.GetRiskControlService(),
		riskEngine:     serviceManager.GetRiskEngine(),
		policy:         serviceManager.GetChallengePolicy(),
		config:         config.LoadConfig(),
	}
}

// GetCaptchaHandler 生成验证码处理器
func (ctrl *CaptchaController) GetCaptchaHandler(c *gin.Context) {
	req := middleware.GetCaptchaRequest(c)
	if req == nil {
		log.Printf("获取请求参数失败: ip=%s", helper.GetRealClientIP(c))
		helper.SimpleErrorResponse(c, helper.ErrCodeInternalError, "内部错误")
		return
	}

	clientHash := helper.ClientFingerprint(c)

	// 难度由服务端根据该客户端的历史表现决定；auto 类型也在这里解析为具体题型
	level := ctrl.policy.DifficultyFor(clientHash)
	captchaType := ctrl.policy.ResolveType(req.Type, level)

	captchaService, exists := ctrl.serviceManager.GetCaptchaService(captchaType)
	if !exists {
		log.Printf("不支持的验证码类型: type=%s, ip=%s", captchaType, helper.GetRealClientIP(c))
		helper.SimpleErrorResponse(c, helper.ErrCodeInvalidParam, "不支持的验证码类型")
		return
	}

	challenge := &services.ChallengeRequest{
		Type:       captchaType,
		Mode:       req.Mode,
		Scale:      req.Scale(),
		Difficulty: level,
	}
	if ctrl.config.Captcha.BindClient {
		challenge.ClientHash = clientHash
	}

	captData, err := captchaService.GenerateCaptcha(challenge)
	if err != nil {
		log.Printf("生成验证码失败: type=%s, mode=%s, ip=%s, error=%v",
			captchaType, req.Mode, helper.GetRealClientIP(c), err)
		helper.ErrorResponse(c, err)
		return
	}

	log.Printf("生成验证码成功: type=%s, mode=%s, scale=%d, level=%d, key=%s, ip=%s",
		captchaType, req.Mode, captData.Scale, captData.Difficulty, captData.Key, helper.GetRealClientIP(c))

	response := ctrl.buildCaptchaResponse(captchaType, captData)

	// 验证码响应绝不允许被缓存
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	helper.SuccessResponse(c, response)
}

// VerifyCaptchaHandler 验证验证码处理器
func (ctrl *CaptchaController) VerifyCaptchaHandler(c *gin.Context) {
	req := middleware.GetVerifyRequest(c)
	if req == nil {
		log.Printf("获取请求参数失败: ip=%s", helper.GetRealClientIP(c))
		helper.SimpleErrorResponse(c, helper.ErrCodeInternalError, "内部错误")
		return
	}

	ip := helper.GetRealClientIP(c)
	clientHash := helper.ClientFingerprint(c)

	verifyService, exists := ctrl.serviceManager.GetVerifyService(req.Type)
	if !exists {
		log.Printf("不支持的验证类型: type=%s, ip=%s", req.Type, ip)
		helper.SimpleErrorResponse(c, helper.ErrCodeInvalidParam, "不支持的验证类型")
		return
	}

	// 行为 / 环境风控评分：enforce 模式下达到拦截阈值时不在此处短路返回，而是交给校验模板
	// 在答案比对阶段强制判失败。前置检查（次数、绑定、时间窗口、PoW、解析）照常执行并返回
	// 与未拦截完全一致的响应，"被拦截"这一信号只留在服务端日志
	evalCtx := &services.EvaluationContext{
		Type:       req.Type,
		ClientHash: clientHash,
		IP:         ip,
		Key:        req.Key,
		Trace:      req.Trace,
		Env:        req.Env,
		Pow:        req.Pow,
	}
	riskResult := ctrl.riskEngine.Evaluate(evalCtx)
	blocked := ctrl.riskEngine.ShouldBlock(riskResult)
	if blocked {
		log.Printf("风控拦截: type=%s, key=%s, ip=%s, score=%d, reasons=%v", req.Type, req.Key, ip, riskResult.Score, riskResult.Reasons)
	}

	input := &services.VerifyInput{
		Key:         req.Key,
		Answer:      ctrl.extractVerifyData(req),
		Pow:         req.Pow,
		RiskBlocked: blocked,
	}
	if ctrl.config.Captcha.BindClient {
		input.ClientHash = clientHash
	}

	isValid, secondKey, err := verifyService.VerifyData(input)
	ctrl.riskEngine.Record(evalCtx, riskResult, isValid)

	if err != nil {
		log.Printf("验证过程出错: type=%s, key=%s, ip=%s, error=%v", req.Type, req.Key, ip, err)
		helper.ErrorResponse(c, err)
		return
	}

	if !isValid {
		log.Printf("验证失败: type=%s, key=%s, ip=%s, message=%s", req.Type, req.Key, ip, secondKey)
		helper.SimpleErrorResponse(c, helper.ErrCodeVerifyFailed, secondKey)
		return
	}

	log.Printf("验证成功: type=%s, key=%s, secondKey=%s, ip=%s", req.Type, req.Key, secondKey, ip)

	helper.SuccessResponse(c, models.VerifyResponse{SecondKey: secondKey})
}

// ValidateCaptchaHandler 验证码状态查询（二次验证）处理器
func (ctrl *CaptchaController) ValidateCaptchaHandler(c *gin.Context) {
	req := middleware.GetStateRequest(c)
	if req == nil {
		log.Printf("获取请求参数失败: ip=%s", helper.GetRealClientIP(c))
		helper.SimpleErrorResponse(c, helper.ErrCodeInternalError, "内部错误")
		return
	}

	if !ctrl.riskControl.ValidateSecondVerifyToken(req.Key) {
		log.Printf("二次验证令牌无效: key=%s, ip=%s", req.Key, helper.GetRealClientIP(c))
		helper.SimpleErrorResponse(c, helper.ErrCodeExpired, "验证码已失效")
		return
	}

	log.Printf("状态查询成功: key=%s, ip=%s", req.Key, helper.GetRealClientIP(c))

	c.JSON(http.StatusOK, gin.H{
		"valid": true,
	})
}

// buildCaptchaResponse 构建验证码响应
func (ctrl *CaptchaController) buildCaptchaResponse(captchaType string, captData *services.CaptchaData) models.CaptchaResponse {
	response := models.CaptchaResponse{
		Key:         captData.Key,
		Type:        captchaType,
		Image:       captData.Image,
		ImageWidth:  captData.ImageWidth,
		ImageHeight: captData.ImageHeight,
		Scale:       captData.Scale,
		Difficulty:  captData.Difficulty,
	}

	switch captchaType {
	case "slide-region", "slide-text":
		response.Thumb = captData.Tile
		response.ThumbWidth = captData.Width
		response.ThumbHeight = captData.Height
		response.ThumbX = captData.X
		response.ThumbY = captData.Y
	default:
		response.Thumb = captData.Thumb
		response.ThumbWidth = captData.ThumbWidth
		response.ThumbHeight = captData.ThumbHeight
	}

	if captData.PowBits > 0 {
		response.Pow = &models.PowChallenge{
			Nonce:      captData.PowNonce,
			Difficulty: captData.PowBits,
		}
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
