package services

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"simple-captcha/config"
	"simple-captcha/helper"
	"simple-captcha/storage"
	"sync"
	"time"
)

// SettingsService 验证码与风控策略的持久化：库中只存当前值，保存后立即替换全局快照
type SettingsService struct {
	db *storage.DB
	mu sync.Mutex // 串行化"读当前值 + 合并请求 + 写库 + 替换快照"
}

// NewSettingsService 创建策略服务
func NewSettingsService(db *storage.DB) *SettingsService {
	return &SettingsService{db: db}
}

// Load 启动时加载：在默认值之上叠加库中的 JSON（未知字段忽略、缺失字段取默认值），
// 已不满足当前校验规则的字段恢复默认值并告警，然后替换全局快照
func (s *SettingsService) Load() error {
	settings := config.DefaultRuntimeSettings()
	var data string
	err := s.db.QueryRowContext(context.Background(), `SELECT data FROM settings WHERE id = 1`).Scan(&data)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// 尚未保存过：全部取默认值
	case err != nil:
		return fmt.Errorf("读取策略配置失败: %w", err)
	default:
		if err := json.Unmarshal([]byte(data), &settings); err != nil {
			log.Printf("警告: 策略配置无法解析，全部使用默认值: error=%v", err)
			settings = config.DefaultRuntimeSettings()
		}
		if reset := settings.Sanitize(); len(reset) > 0 {
			log.Printf("警告: 以下策略配置已不满足当前取值范围，已恢复默认值: %v", reset)
		}
	}
	return config.ApplyRuntime(settings)
}

// Current 当前生效的策略与默认值
func (s *SettingsService) Current() (config.RuntimeSettings, config.RuntimeSettings) {
	return *config.Runtime(), config.DefaultRuntimeSettings()
}

// Update 以当前值为底合并请求体（请求中缺少的字段保持不变，未知字段拒绝），校验后写库并替换快照
func (s *SettingsService) Update(body []byte) (*config.RuntimeSettings, *helper.CaptchaError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := *config.Runtime()
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&next); err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeInvalidParam, "策略配置格式错误", err.Error())
	}
	if err := next.Validate(); err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeInvalidParam, err.Error())
	}

	data, err := json.Marshal(next)
	if err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeInternalError, "序列化策略配置失败", err.Error())
	}
	_, err = s.db.ExecContext(context.Background(),
		`INSERT INTO settings (id, data, updated_at) VALUES (1, ?, ?)
		 ON CONFLICT (id) DO UPDATE SET data = excluded.data, updated_at = excluded.updated_at`,
		string(data), time.Now().Unix())
	if err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeInternalError, "保存策略配置失败", err.Error())
	}
	if err := config.ApplyRuntime(next); err != nil {
		return nil, helper.NewCaptchaError(helper.ErrCodeInternalError, "应用策略配置失败", err.Error())
	}
	return config.Runtime(), nil
}
