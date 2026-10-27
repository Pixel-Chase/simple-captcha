package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFromEnv_RiskCapDefaults(t *testing.T) {
	cfg := loadFromEnv()
	assert.Equal(t, 60, cfg.Risk.EnvScoreCap)
	assert.Equal(t, 20, cfg.Risk.BlockMinBehavior)
	assert.NoError(t, validateConfig(cfg))

	t.Setenv("RISK_ENV_SCORE_CAP", "0")
	t.Setenv("RISK_BLOCK_MIN_BEHAVIOR", "0")
	cfg = loadFromEnv()
	assert.Equal(t, 0, cfg.Risk.EnvScoreCap, "0 关闭环境分上限")
	assert.Equal(t, 0, cfg.Risk.BlockMinBehavior)
	assert.NoError(t, validateConfig(cfg))
}

func TestValidateConfig_RiskCapRange(t *testing.T) {
	cfg := loadFromEnv()
	cfg.Risk.EnvScoreCap = 101
	assert.ErrorContains(t, validateConfig(cfg), "RISK_ENV_SCORE_CAP")

	cfg = loadFromEnv()
	cfg.Risk.BlockMinBehavior = -1
	assert.ErrorContains(t, validateConfig(cfg), "RISK_BLOCK_MIN_BEHAVIOR")
}

func TestLoadFromEnv_FingerprintReplayDefaults(t *testing.T) {
	cfg := loadFromEnv()
	assert.Equal(t, 5, cfg.Risk.FPReplayThreshold)
	assert.Equal(t, 10*time.Minute, cfg.Risk.FPReplayWindow)
	assert.NoError(t, validateConfig(cfg))

	t.Setenv("RISK_FP_REPLAY_THRESHOLD", "0")
	cfg = loadFromEnv()
	assert.Equal(t, 0, cfg.Risk.FPReplayThreshold, "0 关闭指纹重放计数")
	assert.NoError(t, validateConfig(cfg))

	cfg = loadFromEnv()
	cfg.Risk.FPReplayThreshold = -1
	assert.ErrorContains(t, validateConfig(cfg), "RISK_FP_REPLAY_THRESHOLD")

	cfg = loadFromEnv()
	cfg.Risk.FPReplayThreshold = 5
	cfg.Risk.FPReplayWindow = 0
	assert.ErrorContains(t, validateConfig(cfg), "RISK_FP_REPLAY_WINDOW")
}

func TestLoadFromEnv_TLSFingerprintDefaultsAndValidation(t *testing.T) {
	cfg := loadFromEnv()
	assert.Empty(t, cfg.Risk.TLSFingerprintHeader, "默认不读取指纹头")
	assert.Empty(t, cfg.Risk.HTTP2FingerprintHeader)
	assert.Empty(t, cfg.Risk.TLSFingerprintList)
	assert.NoError(t, validateConfig(cfg))

	dir := t.TempDir()
	valid := filepath.Join(dir, "list.json")
	require.NoError(t, os.WriteFile(valid, []byte(`{"browser":{"abc":"chrome"},"deny":["def"]}`), 0o600))
	invalid := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(invalid, []byte(`{nope`), 0o600))

	t.Setenv("RISK_TLS_FINGERPRINT_HEADER", "X-JA4-Fingerprint")
	t.Setenv("RISK_TLS_FINGERPRINT_LIST", valid)
	cfg = loadFromEnv()
	assert.Equal(t, "X-JA4-Fingerprint", cfg.Risk.TLSFingerprintHeader)
	assert.NoError(t, validateConfig(cfg))

	t.Setenv("RISK_TLS_FINGERPRINT_LIST", invalid)
	assert.ErrorContains(t, validateConfig(loadFromEnv()), "RISK_TLS_FINGERPRINT_LIST")

	t.Setenv("RISK_TLS_FINGERPRINT_LIST", filepath.Join(dir, "missing.json"))
	assert.ErrorContains(t, validateConfig(loadFromEnv()), "RISK_TLS_FINGERPRINT_LIST")

	t.Setenv("RISK_TLS_FINGERPRINT_HEADER", "")
	t.Setenv("RISK_TLS_FINGERPRINT_LIST", valid)
	assert.ErrorContains(t, validateConfig(loadFromEnv()), "requires RISK_TLS_FINGERPRINT_HEADER")
}
