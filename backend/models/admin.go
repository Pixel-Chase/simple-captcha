package models

import "simple-captcha/services"

// AdminLoginRequest 管理员登录请求
type AdminLoginRequest struct {
	Username string `json:"username" binding:"required,max=64"`
	Password string `json:"password" binding:"required,max=128"`
	// CaptchaKey 行为验证成功后 SDK 返回的 second_key，服务端一次性消费
	CaptchaKey string `json:"captchaKey" binding:"required,max=100"`
}

// AdminLoginResponse 登录成功响应
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
