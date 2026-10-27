package services

import "simple-captcha/config"

// settingsSource 返回当前策略快照；为 nil 时读取全局的 config.Runtime()。
// 单测注入固定快照，不必改动全局策略
type settingsSource func() *config.RuntimeSettings

func (f settingsSource) get() *config.RuntimeSettings {
	if f == nil {
		return config.Runtime()
	}
	return f()
}

// fixedSettings 固定返回给定快照（测试用）
func fixedSettings(s config.RuntimeSettings) settingsSource {
	return func() *config.RuntimeSettings { return &s }
}
