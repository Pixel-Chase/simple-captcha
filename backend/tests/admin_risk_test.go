package tests

import (
	"fmt"
	"net/http"
	"simple-captcha/config"
	"simple-captcha/services"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdmin_RiskTelemetryRequiresReadySession(t *testing.T) {
	h := newAdminHarness(t, 1)

	// 未登录
	w := h.do(http.MethodGet, "/risk/telemetry", nil, "", "")
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())

	// 仍在使用初始密码：只开放会话与改密，其余接口 403 / 2008
	w = h.doJSON(http.MethodPost, "/login", map[string]string{
		"username":   adminTestUser,
		"password":   h.password,
		"captchaKey": h.freshCaptchaKey(t),
	}, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decodeBody(t, w)
	require.Equal(t, true, body["mustChangePassword"])
	initialToken, _ := body["token"].(string)

	w = h.do(http.MethodGet, "/risk/telemetry", nil, initialToken, "")
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.EqualValues(t, 2008, decodeBody(t, w)["code"])
}

func TestAdmin_RiskTelemetryReturnsMaskedEntries(t *testing.T) {
	cfg := config.LoadConfig()
	if !cfg.Risk.Enabled || cfg.Risk.TelemetryLogSize <= 0 {
		t.Skip("风控评分或遥测日志已关闭")
	}
	h := newAdminHarness(t, 1)
	token := h.login(t)

	// 直接写两条带原始 IP / UA 的遥测记录，模拟出题评估与校验评分
	store := services.NewClientRiskStore()
	key := fmt.Sprintf("telemetry-test-%d", time.Now().UnixNano())
	store.LogTelemetry(map[string]interface{}{
		"kind": "issue", "time": time.Now().Format(time.RFC3339), "type": "slide-text", "key": key,
		"client": "8d42bc159691be7d5dd15580ed6e0f99", "ip": "198.51.100.23",
		"ua":    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
		"level": 4, "base": 3, "boost": 1, "envScore": 45, "reasons": []string{"webdriver(+45)", "env_boost(+1)"}, "viaPost": true,
	})
	store.LogTelemetry(map[string]interface{}{
		"kind": "verify", "time": time.Now().Format(time.RFC3339), "type": "slide-text", "key": key,
		"client": "8d42bc159691be7d5dd15580ed6e0f99", "ip": "2001:db8:1:2:3:4:5:6", "ua": "curl/8.5.0",
		"score": 70, "success": false, "reasons": []string{"non_browser_ua(+20)"},
	})

	w := h.do(http.MethodGet, "/risk/telemetry?limit=50", nil, token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "no-store, no-cache, must-revalidate", w.Header().Get("Cache-Control"))
	body := decodeBody(t, w)
	assert.EqualValues(t, 50, body["limit"])
	items, ok := body["items"].([]interface{})
	require.True(t, ok)
	require.LessOrEqual(t, len(items), 50)

	var issue, verify map[string]interface{}
	for _, raw := range items {
		entry := raw.(map[string]interface{})
		if entry["key"] != key {
			continue
		}
		switch entry["kind"] {
		case "issue":
			issue = entry
		case "verify":
			verify = entry
		}
	}
	require.NotNil(t, issue, "出题记录应在最近列表中")
	require.NotNil(t, verify, "校验记录应在最近列表中")

	assert.Equal(t, "198.51.*.*", issue["ip"])
	assert.Equal(t, "Chrome 130 / Windows", issue["ua"])
	assert.Equal(t, "8d42bc15", issue["client"])
	assert.EqualValues(t, 4, issue["level"])
	assert.Contains(t, fmt.Sprint(issue["reasons"]), "webdriver(+45)")

	assert.Equal(t, "2001:db8:1::*", verify["ip"])
	assert.Equal(t, "非浏览器", verify["ua"])
	assert.EqualValues(t, 70, verify["score"])

	// 整个响应里不得出现原始 IP 或完整 UA
	assert.NotContains(t, w.Body.String(), "198.51.100.23")
	assert.NotContains(t, w.Body.String(), "AppleWebKit")
}

func TestAdmin_RiskTelemetryLimitClampedAndDefaulted(t *testing.T) {
	h := newAdminHarness(t, 1)
	token := h.login(t)

	w := h.do(http.MethodGet, "/risk/telemetry?limit=1000", nil, token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, services.MaxRecentTelemetry, decodeBody(t, w)["limit"], "超过上限裁剪到 500")

	w = h.do(http.MethodGet, "/risk/telemetry", nil, token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 100, decodeBody(t, w)["limit"], "缺省 100")

	w = h.do(http.MethodGet, "/risk/telemetry?limit=0", nil, token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 100, decodeBody(t, w)["limit"], "0 视为缺省")

	w = h.do(http.MethodGet, "/risk/telemetry?limit=-5", nil, token, "")
	assert.Equal(t, http.StatusBadRequest, w.Code, "负数拒绝")
}
