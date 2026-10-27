package models

import (
	"simple-captcha/config"
	"simple-captcha/services"
)

// AdminLoginRequest 管理员登录请求
type AdminLoginRequest struct {
	Username string `json:"username" binding:"required,max=64"`
	Password string `json:"password" binding:"required,max=128"`
	// CaptchaKey 行为验证成功后 SDK 返回的 second_key，服务端一次性消费
	CaptchaKey string `json:"captchaKey" binding:"required,max=100"`
}

// AdminSetupStatusResponse 是否已完成首次初始化
type AdminSetupStatusResponse struct {
	Initialized bool `json:"initialized"`
}

// AdminSetupRequest 首次初始化：启动日志中的一次性口令 + 管理员用户名与密码；长度与字符规则在服务层校验
type AdminSetupRequest struct {
	SetupCode string `json:"setupCode" binding:"required,max=64"`
	Username  string `json:"username" binding:"required,max=256"`
	Password  string `json:"password" binding:"required,max=128"`
}

// AdminLoginResponse 登录成功响应；初始化与修改密码成功后也以此结构返回新签发的会话
type AdminLoginResponse struct {
	Token     string `json:"token"`
	Username  string `json:"username"`
	ExpiresAt int64  `json:"expiresAt"` // Unix 秒
}

// AdminSessionResponse 当前会话信息
type AdminSessionResponse struct {
	Username  string `json:"username"`
	ExpiresAt int64  `json:"expiresAt"` // Unix 秒
}

// AdminChangePasswordRequest 修改管理员密码；新密码规则在服务层校验
type AdminChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword" binding:"required,max=128"`
	NewPassword     string `json:"newPassword" binding:"required,max=128"`
}

// BackgroundListResponse 背景图列表
type BackgroundListResponse struct {
	Items       []services.BackgroundInfo  `json:"items"`
	Total       int                        `json:"total"`
	Constraints services.UploadConstraints `json:"constraints"`
}

// BackgroundNameRequest 路径参数：背景图文件名
type BackgroundNameRequest struct {
	Name string `uri:"name" binding:"required,min=1,max=160"`
}

// AppCreateRequest 新建接入应用；名称长度与字符、来源格式在服务层校验
type AppCreateRequest struct {
	Name string `json:"name" binding:"required,max=200"`
	// Origins 允许发起出题与提交的来源，例如 https://www.example.com；可以为空，之后再配置
	Origins []string `json:"origins" binding:"max=100,dive,max=300"`
}

// AppUpdateRequest 修改接入应用：整体替换名称与启用状态；origins 缺省时保持不变，提供时整体替换
type AppUpdateRequest struct {
	Name    string    `json:"name" binding:"required,max=200"`
	Enabled *bool     `json:"enabled" binding:"required"`
	Origins *[]string `json:"origins" binding:"omitempty,max=100,dive,max=300"`
}

// AppIDRequest 路径参数：应用 ID
type AppIDRequest struct {
	ID string `uri:"id" binding:"required,max=32"`
}

// AppListResponse 接入应用列表
type AppListResponse struct {
	Items []services.AppInfo `json:"items"`
	Total int                `json:"total"`
}

// AppSecretResponse 新建应用或重置密钥的响应；明文密钥只在这里出现一次
type AppSecretResponse struct {
	App    services.AppInfo `json:"app"`
	Secret string           `json:"secret"`
}

// SettingsResponse 策略配置：当前值与默认值（控制台据此显示每项的默认值并支持恢复默认）
type SettingsResponse struct {
	Settings config.RuntimeSettings `json:"settings"`
	Defaults config.RuntimeSettings `json:"defaults"`
}

// RiskTelemetryRequest 风控观察：最近 N 条遥测记录，超过上限的 limit 在服务层裁剪
type RiskTelemetryRequest struct {
	Limit int `form:"limit" binding:"omitempty,min=1"`
}

// RiskTelemetryResponse 风控观察响应；条目已脱敏（ip 只保留前两段、ua 为摘要、client 为前缀）
type RiskTelemetryResponse struct {
	Items []map[string]interface{} `json:"items"`
	Limit int                      `json:"limit"`
}
