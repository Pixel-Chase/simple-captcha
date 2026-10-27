package controllers

import (
	"errors"
	"log"
	"net/http"
	"simple-captcha/helper"
	"simple-captcha/middleware"
	"simple-captcha/models"
	"simple-captcha/services"

	"github.com/gin-gonic/gin"
)

// multipartOverhead 上传请求体在图片本体之外允许的额外体积（表单字段、分隔符等）
const multipartOverhead = 1 << 20

// AdminController 管理后台控制器：登录会话、管理员密码、背景图管理与风控观察
type AdminController struct {
	admin      *services.AdminService
	clientRisk *services.ClientRiskStore
}

// NewAdminController 创建管理后台控制器
func NewAdminController(admin *services.AdminService) *AdminController {
	return &AdminController{admin: admin, clientRisk: services.NewClientRiskStore()}
}

// RiskTelemetryHandler 风控观察：返回最近的出题评估与校验评分记录（已脱敏），供调权重时参考
func (ctrl *AdminController) RiskTelemetryHandler(c *gin.Context) {
	var req models.RiskTelemetryRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		helper.ErrorResponse(c, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "参数格式错误", err.Error()))
		return
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > services.MaxRecentTelemetry {
		limit = services.MaxRecentTelemetry
	}

	items, err := ctrl.clientRisk.RecentTelemetry(limit)
	if err != nil {
		log.Printf("读取遥测记录失败: error=%v", err)
		helper.ErrorResponse(c, helper.NewCaptchaError(helper.ErrCodeInternalError, "读取遥测记录失败", err.Error()))
		return
	}
	masked := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		masked = append(masked, services.MaskTelemetry(item))
	}
	helper.SuccessResponse(c, models.RiskTelemetryResponse{Items: masked, Limit: limit})
}

// SetupStatusHandler 是否已完成首次初始化，控制台据此决定显示初始化页还是登录页
func (ctrl *AdminController) SetupStatusHandler(c *gin.Context) {
	initialized, err := ctrl.admin.Initialized()
	if err != nil {
		helper.ErrorResponse(c, err)
		return
	}
	helper.SuccessResponse(c, models.AdminSetupStatusResponse{Initialized: initialized})
}

// SetupHandler 首次初始化：校验一次性口令，写入管理员账号后直接签发会话
func (ctrl *AdminController) SetupHandler(c *gin.Context) {
	var req models.AdminSetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		helper.ErrorResponse(c, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "参数格式错误", err.Error()))
		return
	}

	ip := helper.GetRealClientIP(c)
	token, session, err := ctrl.admin.Setup(req.SetupCode, req.Username, req.Password)
	if err != nil {
		log.Printf("管理后台初始化失败: ip=%s, code=%d", ip, err.Code)
		helper.ErrorResponse(c, err)
		return
	}

	log.Printf("管理后台初始化完成: username=%q, ip=%s", session.Username, ip)
	helper.SuccessResponse(c, loginResponse(token, session))
}

// LoginHandler 管理员登录：先消费行为验证 second_key，再比对账号密码
func (ctrl *AdminController) LoginHandler(c *gin.Context) {
	var req models.AdminLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		helper.ErrorResponse(c, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "参数格式错误", err.Error()))
		return
	}

	ip := helper.GetRealClientIP(c)
	token, session, err := ctrl.admin.Login(req.Username, req.Password, req.CaptchaKey)
	if err != nil {
		// 用户名为外部输入，用 %q 转义避免日志注入
		log.Printf("管理员登录失败: username=%q, ip=%s, code=%d", req.Username, ip, err.Code)
		helper.ErrorResponse(c, err)
		return
	}

	log.Printf("管理员登录成功: username=%q, ip=%s", session.Username, ip)
	helper.SuccessResponse(c, loginResponse(token, session))
}

// ChangePasswordHandler 修改管理员密码：校验当前密码，成功后作废全部旧会话并返回新会话
func (ctrl *AdminController) ChangePasswordHandler(c *gin.Context) {
	var req models.AdminChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		helper.ErrorResponse(c, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "参数格式错误", err.Error()))
		return
	}

	ip := helper.GetRealClientIP(c)
	session := middleware.GetAdminSession(c)
	token, next, err := ctrl.admin.ChangePassword(req.CurrentPassword, req.NewPassword)
	if err != nil {
		log.Printf("修改管理员密码失败: admin=%s, ip=%s, code=%d", sessionName(session), ip, err.Code)
		helper.ErrorResponse(c, err)
		return
	}

	log.Printf("修改管理员密码成功，旧会话已全部失效: admin=%q, ip=%s", next.Username, ip)
	helper.SuccessResponse(c, loginResponse(token, next))
}

// SessionHandler 返回当前会话信息，供前端恢复登录态时校验令牌
func (ctrl *AdminController) SessionHandler(c *gin.Context) {
	session := middleware.GetAdminSession(c)
	if session == nil {
		helper.ErrorResponse(c, helper.ErrUnauthorized)
		return
	}
	helper.SuccessResponse(c, models.AdminSessionResponse{
		Username:  session.Username,
		ExpiresAt: session.ExpiresAt.Unix(),
	})
}

// LogoutHandler 作废当前会话令牌
func (ctrl *AdminController) LogoutHandler(c *gin.Context) {
	ctrl.admin.Logout(middleware.BearerToken(c))
	c.Status(http.StatusNoContent)
}

// ListBackgroundsHandler 列出全部背景图及上传约束
func (ctrl *AdminController) ListBackgroundsHandler(c *gin.Context) {
	items, err := ctrl.admin.ListBackgrounds()
	if err != nil {
		log.Printf("读取背景图列表失败: error=%v", err)
		helper.ErrorResponse(c, err)
		return
	}
	helper.SuccessResponse(c, models.BackgroundListResponse{
		Items:       items,
		Total:       len(items),
		Constraints: ctrl.admin.Constraints(),
	})
}

// UploadBackgroundHandler 上传一张背景图（multipart 字段名 file），成功后素材即时生效
func (ctrl *AdminController) UploadBackgroundHandler(c *gin.Context) {
	// 在 multipart 解析前就按上传上限截断请求体，超大文件不会被完整读入内存
	limit := ctrl.admin.Constraints().MaxUploadSize + multipartOverhead
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)

	file, err := c.FormFile("file")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			helper.ErrorResponse(c, helper.NewCaptchaError(helper.ErrCodeImageTooLarge, "图片体积超出限制"))
			return
		}
		helper.ErrorResponse(c, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "缺少 file 字段或表单格式错误", err.Error()))
		return
	}

	src, err := file.Open()
	if err != nil {
		helper.ErrorResponse(c, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "读取上传文件失败", err.Error()))
		return
	}
	defer src.Close()

	session := middleware.GetAdminSession(c)
	info, cerr := ctrl.admin.SaveBackground(file.Filename, src)
	if cerr != nil {
		log.Printf("上传背景图失败: filename=%q, admin=%s, ip=%s, error=%v",
			file.Filename, sessionName(session), helper.GetRealClientIP(c), cerr)
		helper.ErrorResponse(c, cerr)
		return
	}

	log.Printf("上传背景图成功: name=%s, size=%d, dimension=%dx%d, admin=%s, ip=%s",
		info.Name, info.Size, info.Width, info.Height, sessionName(session), helper.GetRealClientIP(c))
	helper.SuccessResponse(c, info)
}

// PreviewBackgroundHandler 回传背景图原文件供管理页预览
func (ctrl *AdminController) PreviewBackgroundHandler(c *gin.Context) {
	var req models.BackgroundNameRequest
	if err := c.ShouldBindUri(&req); err != nil {
		helper.ErrorResponse(c, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "路径参数错误", err.Error()))
		return
	}

	path, err := ctrl.admin.ResolveBackgroundPath(req.Name)
	if err != nil {
		helper.ErrorResponse(c, err)
		return
	}

	// 预览图允许浏览器私有缓存，覆盖分组级的 no-store
	header := c.Writer.Header()
	header.Set("Cache-Control", "private, max-age=300")
	header.Del("Pragma")
	header.Del("Expires")
	header.Set("Content-Disposition", "inline")
	c.File(path)
}

// DeleteBackgroundHandler 删除一张背景图，成功后素材即时生效
func (ctrl *AdminController) DeleteBackgroundHandler(c *gin.Context) {
	var req models.BackgroundNameRequest
	if err := c.ShouldBindUri(&req); err != nil {
		helper.ErrorResponse(c, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "路径参数错误", err.Error()))
		return
	}

	session := middleware.GetAdminSession(c)
	if err := ctrl.admin.DeleteBackground(req.Name); err != nil {
		log.Printf("删除背景图失败: name=%q, admin=%s, ip=%s, error=%v",
			req.Name, sessionName(session), helper.GetRealClientIP(c), err)
		helper.ErrorResponse(c, err)
		return
	}

	log.Printf("删除背景图成功: name=%s, admin=%s, ip=%s", req.Name, sessionName(session), helper.GetRealClientIP(c))
	c.Status(http.StatusNoContent)
}

func loginResponse(token string, session *services.AdminSession) models.AdminLoginResponse {
	return models.AdminLoginResponse{
		Token:     token,
		Username:  session.Username,
		ExpiresAt: session.ExpiresAt.Unix(),
	}
}

func sessionName(session *services.AdminSession) string {
	if session == nil {
		return "-"
	}
	return session.Username
}
