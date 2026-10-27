package controllers

import (
	"log"
	"net/http"
	"simple-captcha/config"
	"simple-captcha/helper"
	"simple-captcha/middleware"
	"simple-captcha/models"
	"simple-captcha/risk"
	"simple-captcha/services"
	"sync"

	"github.com/gin-gonic/gin"
)

// CaptchaController 验证码控制器
type CaptchaController struct {
	serviceManager services.ServiceManager
	riskControl    services.RiskControlService
	riskEngine     *services.RiskEngine
	policy         *services.ChallengePolicy
	clientRisk     *services.ClientRiskStore
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
		clientRisk:     serviceManager.GetClientRiskStore(),
		config:         config.LoadConfig(),
	}
}

// issueParams 出题流程的输入
type issueParams struct {
	requestedType string
	mode          string
	scale         int
	dpr           float64
	env           *risk.Env
}

// IssueCaptchaHandler 生成验证码处理器（POST：请求体携带环境信号，出题前据此加难）
func (ctrl *CaptchaController) IssueCaptchaHandler(c *gin.Context) {
	req := middleware.GetIssueRequest(c)
	if req == nil {
		log.Printf("获取请求参数失败: ip=%s", helper.GetRealClientIP(c))
		helper.SimpleErrorResponse(c, helper.ErrCodeInternalError, "内部错误")
		return
	}
	ctrl.issue(c, issueParams{requestedType: req.Type, mode: req.Mode, scale: req.Scale(), dpr: req.Dpr, env: req.Env})
}

// requestSignals 提取请求头信号并附上指纹参考列表（未配置时为 nil，指纹只记录不计分）
func (ctrl *CaptchaController) requestSignals(c *gin.Context, dpr float64) *risk.RequestSignals {
	signals := helper.RequestSignalsFrom(c, dpr)
	signals.Fingerprints = ctrl.serviceManager.GetTLSFingerprintList()
	return signals
}

// issue 出题：决定难度与题型、生成题目、记录出题评估
func (ctrl *CaptchaController) issue(c *gin.Context, p issueParams) {
	ip := helper.GetRealClientIP(c)
	clientHash := helper.ClientFingerprint(c)

	// 同一份 env 被复制到多个客户端重放时，fp 哈希相同而客户端哈希不同；只在携带 fp 的出题请求上计数
	fpReplays := 0
	if p.env != nil && p.env.Fingerprint != "" && ctrl.clientRisk != nil {
		fpReplays = ctrl.clientRisk.TouchFingerprint(p.env.Fingerprint, clientHash)
	}

	// 难度由服务端根据客户端历史表现、出口 IP 请求量、请求头核验、环境信号与指纹重放决定，只升不降；auto 类型也在这里解析为具体题型
	ipRequests := middleware.GetIPRequestCount(c)
	signals := ctrl.requestSignals(c, p.dpr)
	decision := ctrl.policy.Decide(services.IssueInput{
		ClientHash: clientHash,
		IPRequests: ipRequests,
		Env:        p.env,
		Request:    signals,
		FPReplays:  fpReplays,
	})
	captchaType := ctrl.policy.ResolveType(p.requestedType, decision.Level)

	captchaService, exists := ctrl.serviceManager.GetCaptchaService(captchaType)
	if !exists {
		log.Printf("不支持的验证码类型: type=%s, ip=%s", captchaType, ip)
		helper.SimpleErrorResponse(c, helper.ErrCodeInvalidParam, "不支持的验证码类型")
		return
	}

	challenge := &services.ChallengeRequest{
		Type:       captchaType,
		Mode:       p.mode,
		Scale:      p.scale,
		Difficulty: decision.Level,
		AppID:      middleware.GetRequestAppID(c),
	}
	if config.Runtime().Captcha.BindClient {
		challenge.ClientHash = clientHash
	}

	captData, err := captchaService.GenerateCaptcha(challenge)
	if err != nil {
		log.Printf("生成验证码失败: type=%s, mode=%s, ip=%s, error=%v", captchaType, p.mode, ip, err)
		helper.ErrorResponse(c, err)
		return
	}

	log.Printf("生成验证码成功: type=%s, mode=%s, scale=%d, level=%d, boost=%d, envScore=%d, key=%s, ip=%s, ipRequests=%d",
		captchaType, p.mode, captData.Scale, captData.Difficulty, decision.Boost, decision.EnvScore, captData.Key, ip, ipRequests)

	ctrl.riskEngine.RecordIssue(&services.IssueContext{
		Type:       captchaType,
		ClientHash: clientHash,
		IP:         ip,
		Key:        captData.Key,
		Env:        p.env,
		Request:    signals,
		AppID:      challenge.AppID,
	}, decision)

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
		Request:    ctrl.requestSignals(c, 0),
		Pow:        req.Pow,
		AppID:      middleware.GetRequestAppID(c),
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
		AppID:       evalCtx.AppID,
		RiskBlocked: blocked,
	}
	if config.Runtime().Captcha.BindClient {
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

	// 只能查询并消费本应用签发的 key：属于其他应用的 key 与无效 key 的响应相同，且不会被消费
	if !ctrl.riskControl.ConsumeSecondVerifyToken(req.Key, middleware.GetAppID(c)) {
		log.Printf("二次验证令牌无效: key=%s, app=%s, ip=%s", req.Key, middleware.GetAppID(c), helper.GetRealClientIP(c))
		helper.SimpleErrorResponse(c, helper.ErrCodeExpired, "验证码已失效")
		return
	}

	log.Printf("状态查询成功: key=%s, app=%s, ip=%s", req.Key, middleware.GetAppID(c), helper.GetRealClientIP(c))

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

// 路由注册用的全局处理函数共用一个默认控制器。
// 默认控制器在首个请求时才创建：包初始化阶段不触发服务管理器（加载素材、打开数据库、生成初始化口令），
// reset-admin 子命令与测试的 TestMain 才能在此之前完成各自的准备
var (
	captchaControllerOnce sync.Once
	captchaController     *CaptchaController
)

func defaultCaptchaController() *CaptchaController {
	captchaControllerOnce.Do(func() {
		captchaController = NewCaptchaController()
	})
	return captchaController
}

// IssueCaptchaHandler 全局函数（POST 出题）
func IssueCaptchaHandler(c *gin.Context) {
	defaultCaptchaController().IssueCaptchaHandler(c)
}

// VerifyCaptchaHandler 全局函数（提交答案）
func VerifyCaptchaHandler(c *gin.Context) {
	defaultCaptchaController().VerifyCaptchaHandler(c)
}

// ValidateCaptchaHandler 全局函数（S2S 状态查询）
func ValidateCaptchaHandler(c *gin.Context) {
	defaultCaptchaController().ValidateCaptchaHandler(c)
}
