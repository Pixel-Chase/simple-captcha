package controllers

import (
	"io"
	"log"
	"simple-captcha/helper"
	"simple-captcha/middleware"
	"simple-captcha/models"
	"simple-captcha/services"

	"github.com/gin-gonic/gin"
)

// settingsMaxBody 策略配置请求体上限：23 个字段的 JSON 远小于该值
const settingsMaxBody = 16 << 10

// SettingsController 管理后台「策略配置」：读取与保存验证码与风控策略，保存后立即生效
type SettingsController struct {
	settings *services.SettingsService
}

// NewSettingsController 创建策略配置控制器
func NewSettingsController(settings *services.SettingsService) *SettingsController {
	return &SettingsController{settings: settings}
}

// GetHandler 当前值与默认值
func (ctrl *SettingsController) GetHandler(c *gin.Context) {
	current, defaults := ctrl.settings.Current()
	helper.SuccessResponse(c, models.SettingsResponse{Settings: current, Defaults: defaults})
}

// UpdateHandler 保存策略：校验失败返回 400 与字段级中文提示，成功后下一次出题即生效
func (ctrl *SettingsController) UpdateHandler(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, settingsMaxBody+1))
	if err != nil || len(body) > settingsMaxBody {
		helper.ErrorResponse(c, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "请求体过大或读取失败"))
		return
	}

	next, cerr := ctrl.settings.Update(body)
	if cerr != nil {
		helper.ErrorResponse(c, cerr)
		return
	}

	session := middleware.GetAdminSession(c)
	log.Printf("策略配置已更新: admin=%s, ip=%s", sessionName(session), helper.GetRealClientIP(c))
	helper.SuccessResponse(c, gin.H{"settings": next})
}
