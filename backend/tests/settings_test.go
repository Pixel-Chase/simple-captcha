package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"simple-captcha/config"
	"simple-captcha/helper"
	"simple-captcha/services"
	"simple-captcha/storage"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// restoreRuntime 用例结束后恢复默认策略，避免影响其他用例
func restoreRuntime(t *testing.T) {
	t.Helper()
	before := *config.Runtime()
	t.Cleanup(func() { require.NoError(t, config.ApplyRuntime(before)) })
}

func (h *adminHarness) getSettings(t *testing.T, token string) (config.RuntimeSettings, config.RuntimeSettings) {
	t.Helper()
	w := h.do(http.MethodGet, "/settings", nil, token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body struct {
		Settings config.RuntimeSettings `json:"settings"`
		Defaults config.RuntimeSettings `json:"defaults"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Settings, body.Defaults
}

func (h *adminHarness) putSettings(t *testing.T, token string, settings interface{}) *httptest.ResponseRecorder {
	t.Helper()
	return h.doJSON(http.MethodPut, "/settings", settings, token)
}

func TestSettings_RequiresSession(t *testing.T) {
	h := newAdminHarness(t, 1)
	assert.Equal(t, http.StatusUnauthorized, h.do(http.MethodGet, "/settings", nil, "", "").Code)
	assert.Equal(t, http.StatusUnauthorized, h.doJSON(http.MethodPut, "/settings", map[string]int{}, "").Code)
}

func TestSettings_GetReturnsCurrentAndDefaults(t *testing.T) {
	restoreRuntime(t)
	h := newAdminHarness(t, 1)
	token := h.login(t)

	current, defaults := h.getSettings(t, token)
	assert.Equal(t, *config.Runtime(), current)
	assert.Equal(t, config.DefaultRuntimeSettings(), defaults)
}

func TestSettings_ValidationErrorsNameTheField(t *testing.T) {
	restoreRuntime(t)
	h := newAdminHarness(t, 1)
	token := h.login(t)
	before := *config.Runtime()

	bad := config.DefaultRuntimeSettings()
	bad.Captcha.DefaultDifficulty = 6
	bad.Risk.Mode = "block"
	w := h.putSettings(t, token, bad)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Equal(t, helper.ErrCodeInvalidParam, errorCode(t, w))
	assert.Contains(t, w.Body.String(), "captcha.defaultDifficulty")
	assert.Contains(t, w.Body.String(), "risk.mode")

	// 未知字段（拼写错误）同样拒绝
	w = h.doJSON(http.MethodPut, "/settings", map[string]interface{}{"captcha": map[string]int{"defaultDiffculty": 4}}, token)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 失败不改变当前值
	assert.Equal(t, before, *config.Runtime())
}

// 热生效：保存后下一次出题即用新难度；改为 log 模式后高风险提交不再被拦截；全程不重启
func TestSettings_HotReload(t *testing.T) {
	restoreRuntime(t)
	h := newAdminHarness(t, 1)
	token := h.login(t)

	next := *config.Runtime()
	next.Captcha.DefaultDifficulty = 4
	next.Captcha.IPEscalatePerMinute = 0 // 排除出口 IP 加难对断言的影响
	w := h.putSettings(t, token, next)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	ua := uniqueUA("settings-hot-reload")
	body := issueBody(t, issueRequest(t, http.MethodPost, "slide-text", map[string]interface{}{"dpr": 2, "env": cleanIssueEnvFor(ua)}, ua))
	assert.Equal(t, 4, int(body["difficulty"].(float64)), "下一次出题使用新的默认难度")

	// enforce 模式下机器特征 + 正确答案被拦截；切到 log 后同样的提交通过
	if !config.Runtime().Risk.Enabled {
		t.Skip("风控评分关闭")
	}
	submitBot := func() int {
		service, _ := services.GetServiceManager().GetCaptchaService("slide-text")
		data, err := service.GenerateCaptcha(&services.ChallengeRequest{Type: "slide-text", AppID: services.ConsoleAppID})
		require.Nil(t, err)
		answer, meta := slideAnswer(t, data.Key)
		pow := solvePow(t, data.Key, meta)
		env, trace := botTelemetry()
		waitMinSolveTime()
		payload, _ := json.Marshal(map[string]interface{}{"point": answer, "pow": pow, "env": env, "trace": trace})
		req := asConsoleApp(httptest.NewRequest(http.MethodPost, "/api/v1/captchas/slide-text/"+data.Key+"/attempts", bytes.NewReader(payload)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router().ServeHTTP(rec, req)
		var resp struct {
			Code      int    `json:"code"`
			SecondKey string `json:"second_key"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		if resp.SecondKey != "" {
			return 0
		}
		return resp.Code
	}

	next.Risk.Mode = "enforce"
	require.Equal(t, http.StatusOK, h.putSettings(t, token, next).Code)
	assert.Equal(t, helper.ErrCodeVerifyFailed, submitBot(), "enforce 模式拦截高风险提交")

	next.Risk.Mode = "log"
	require.Equal(t, http.StatusOK, h.putSettings(t, token, next).Code)
	assert.Equal(t, 0, submitBot(), "log 模式只记录不拦截")
}

// 持久化：保存后重新打开库并加载，值一致；库中没有记录时取默认值
func TestSettings_PersistAcrossRestart(t *testing.T) {
	restoreRuntime(t)
	h := newAdminHarness(t, 1)
	token := h.login(t)

	next := config.DefaultRuntimeSettings()
	next.Captcha.MaxAttempts = 7
	next.Pow.Required = true
	next.Risk.TelemetryRetention = 2000
	require.Equal(t, http.StatusOK, h.putSettings(t, token, next).Code)

	// 模拟重启：先把内存快照改回默认，再从库加载
	require.NoError(t, config.ApplyRuntime(config.DefaultRuntimeSettings()))
	db, err := storage.Open(h.dataDir)
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, services.NewSettingsService(db).Load())
	assert.Equal(t, next, *config.Runtime())

	// 空库：全部默认值
	require.NoError(t, services.NewSettingsService(storage.OpenTemp(t)).Load())
	assert.Equal(t, config.DefaultRuntimeSettings(), *config.Runtime())
}

// 只提交部分字段时其余字段保持当前值；库中残留的非法值启动时恢复默认值
func TestSettings_PartialUpdateAndSanitizeOnLoad(t *testing.T) {
	restoreRuntime(t)
	h := newAdminHarness(t, 1)
	token := h.login(t)

	w := h.doJSON(http.MethodPut, "/settings", map[string]interface{}{"captcha": map[string]int{"maxAttempts": 9}}, token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 9, config.Runtime().Captcha.MaxAttempts)
	assert.Equal(t, config.DefaultRuntimeSettings().Captcha.DefaultDifficulty, config.Runtime().Captcha.DefaultDifficulty)

	// 直接往库里写一个越界值与一个未知字段，模拟规则收紧后的旧数据
	raw := `{"captcha":{"maxAttempts":99,"defaultDifficulty":2},"future":{"x":1}}`
	_, err := h.db.Exec(`UPDATE settings SET data = ? WHERE id = 1`, raw)
	require.NoError(t, err)
	require.NoError(t, services.NewSettingsService(h.db).Load())
	assert.Equal(t, config.DefaultRuntimeSettings().Captcha.MaxAttempts, config.Runtime().Captcha.MaxAttempts, "越界值恢复默认")
	assert.Equal(t, 2, config.Runtime().Captcha.DefaultDifficulty, "合法值保留")

	// 无法解析时整体取默认值
	_, err = h.db.Exec(`UPDATE settings SET data = ? WHERE id = 1`, strings.Repeat("{", 3))
	require.NoError(t, err)
	require.NoError(t, services.NewSettingsService(h.db).Load())
	assert.Equal(t, config.DefaultRuntimeSettings(), *config.Runtime())
}
