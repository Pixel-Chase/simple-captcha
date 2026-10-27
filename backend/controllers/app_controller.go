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

// AppController 接入应用管理：每个业务后端一个应用，凭应用密钥调用 S2S 接口
type AppController struct {
	apps *services.AppService
}

// NewAppController 创建接入应用控制器
func NewAppController(apps *services.AppService) *AppController {
	return &AppController{apps: apps}
}

// ListHandler 列出全部接入应用（不含密钥）
func (ctrl *AppController) ListHandler(c *gin.Context) {
	items, err := ctrl.apps.List()
	if err != nil {
		helper.ErrorResponse(c, err)
		return
	}
	helper.SuccessResponse(c, models.AppListResponse{Items: items, Total: len(items)})
}

// CreateHandler 新建接入应用，响应中的明文密钥只出现这一次
func (ctrl *AppController) CreateHandler(c *gin.Context) {
	var req models.AppCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		helper.ErrorResponse(c, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "参数格式错误", err.Error()))
		return
	}

	app, secret, err := ctrl.apps.Create(req.Name, req.Origins)
	if err != nil {
		helper.ErrorResponse(c, err)
		return
	}

	log.Printf("新建接入应用: app=%s, name=%q, admin=%s, ip=%s", app.ID, app.Name, adminName(c), helper.GetRealClientIP(c))
	helper.SuccessResponse(c, models.AppSecretResponse{App: *app, Secret: secret})
}

// UpdateHandler 修改应用名称与启用状态
func (ctrl *AppController) UpdateHandler(c *gin.Context) {
	id, ok := bindAppID(c)
	if !ok {
		return
	}
	var req models.AppUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		helper.ErrorResponse(c, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "参数格式错误", err.Error()))
		return
	}

	app, err := ctrl.apps.Update(id, req.Name, *req.Enabled, req.Origins)
	if err != nil {
		helper.ErrorResponse(c, err)
		return
	}

	log.Printf("修改接入应用: app=%s, name=%q, enabled=%t, admin=%s, ip=%s",
		app.ID, app.Name, app.Enabled, adminName(c), helper.GetRealClientIP(c))
	helper.SuccessResponse(c, app)
}

// RotateSecretHandler 重置应用密钥，旧密钥立即失效
func (ctrl *AppController) RotateSecretHandler(c *gin.Context) {
	id, ok := bindAppID(c)
	if !ok {
		return
	}

	app, secret, err := ctrl.apps.RotateSecret(id)
	if err != nil {
		helper.ErrorResponse(c, err)
		return
	}

	log.Printf("重置接入应用密钥: app=%s, admin=%s, ip=%s", app.ID, adminName(c), helper.GetRealClientIP(c))
	helper.SuccessResponse(c, models.AppSecretResponse{App: *app, Secret: secret})
}

// DeleteHandler 删除接入应用
func (ctrl *AppController) DeleteHandler(c *gin.Context) {
	id, ok := bindAppID(c)
	if !ok {
		return
	}

	if err := ctrl.apps.Delete(id); err != nil {
		helper.ErrorResponse(c, err)
		return
	}

	log.Printf("删除接入应用: app=%s, admin=%s, ip=%s", id, adminName(c), helper.GetRealClientIP(c))
	c.Status(http.StatusNoContent)
}

// bindAppID 解析并校验路径中的应用 ID；格式不合法时直接按不存在处理
func bindAppID(c *gin.Context) (string, bool) {
	var req models.AppIDRequest
	if err := c.ShouldBindUri(&req); err != nil {
		helper.ErrorResponse(c, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "路径参数错误", err.Error()))
		return "", false
	}
	if !services.ValidAppID(req.ID) {
		helper.ErrorResponse(c, helper.ErrAppNotFound)
		return "", false
	}
	return req.ID, true
}

func adminName(c *gin.Context) string {
	return sessionName(middleware.GetAdminSession(c))
}
