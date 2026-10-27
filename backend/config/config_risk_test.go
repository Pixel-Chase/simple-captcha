package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
