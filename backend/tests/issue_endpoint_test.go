package tests

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"simple-captcha/cache"
	"simple-captcha/config"
	"simple-captcha/risk"
	"simple-captcha/services"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cleanIssueEnvFor 一份与 withBrowserHeaders 一致的干净桌面 Chrome 环境信号。
// 指纹由 UA 派生：每个测试客户端各有一份 fp，避免跨用例共用同一指纹触发重放计数
func cleanIssueEnvFor(ua string) *risk.Env {
	sum := md5.Sum([]byte(ua))
	return &risk.Env{
		UA:          ua,
		Platform:    "Win32",
		Languages:   []string{"zh-CN", "en"},
		Timezone:    "Asia/Shanghai",
		TzOffset:    -480,
		Screen:      [2]int{1920, 1080},
		Viewport:    [2]int{1536, 864},
		DPR:         2,
		Cores:       8,
		Memory:      8,
		Plugins:     5,
		WebGL:       "ANGLE (NVIDIA, NVIDIA GeForce RTX 3060 Direct3D11 vs_5_0 ps_5_0, D3D11)",
		Fingerprint: hex.EncodeToString(sum[:]),
	}
}

func uniqueUA(prefix string) string {
	return browserUA(fmt.Sprintf("%s/%d", prefix, time.Now().UnixNano()))
}

// rawIssueRequest 构造出题请求但不补浏览器请求头，由调用方自行设置
func rawIssueRequest(t *testing.T, method, captchaType string, body interface{}) *http.Request {
	cfg := config.LoadConfig()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(method, cfg.Server.APIPrefix+"/captchas/"+captchaType, bytes.NewReader(payload))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func serve(req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	router().ServeHTTP(w, req)
	return w
}

// issueRequest 以真实浏览器的请求头出题：method 为 GET 或 POST，body 为 nil 时不带请求体；出口 IP 由 UA 派生
func issueRequest(t *testing.T, method, captchaType string, body interface{}, userAgent string) *httptest.ResponseRecorder {
	req := withBrowserHeaders(rawIssueRequest(t, method, captchaType, body), userAgent)
	req.RemoteAddr = remoteAddrFor(userAgent)
	return serve(req)
}

func issueBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

func difficultyOf(t *testing.T, w *httptest.ResponseRecorder) int {
	return int(issueBody(t, w)["difficulty"].(float64))
}

// telemetryEntryFor 从 Redis 遥测列表里找出该题目的出题评估记录
func telemetryEntryFor(t *testing.T, key string) map[string]interface{} {
	raw, err := cache.GetRedisClient().LRange(context.Background(), "captcha:telemetry", 0, 100).Result()
	require.NoError(t, err)
	for _, item := range raw {
		var entry map[string]interface{}
		if json.Unmarshal([]byte(item), &entry) == nil && entry["key"] == key && entry["kind"] == "issue" {
			return entry
		}
	}
	require.FailNow(t, "出题评估应写入 captcha:telemetry", "key=%s", key)
	return nil
}

func skipWithoutRisk(t *testing.T) *config.Config {
	cfg := config.LoadConfig()
	if !cfg.Risk.Enabled {
		t.Skip("RISK_ENABLED=false")
	}
	return cfg
}

func TestIssueEndpoint_CleanEnvStaysDefaultBeforeAndAfterSuccess(t *testing.T) {
	cfg := config.LoadConfig()
	base := services.ClampDifficulty(cfg.Captcha.DefaultDifficulty)
	ua := uniqueUA("issue-clean")
	env := cleanIssueEnvFor(ua)

	first := issueBody(t, issueRequest(t, http.MethodPost, "slide-text", map[string]interface{}{"dpr": 2, "env": env}, ua))
	assert.Equal(t, base, int(first["difficulty"].(float64)), "干净环境保持默认难度")

	// 用正确答案通过一次校验：提交客户端必须与出题客户端一致（同 IP + UA）
	key := first["key"].(string)
	answer, meta := slideAnswer(t, key)
	pow := solvePow(t, key, meta)
	waitMinSolveTime()
	attempt := map[string]interface{}{"point": answer, "env": env}
	if pow != nil {
		attempt["pow"] = pow
	}
	payload, _ := json.Marshal(attempt)
	req := httptest.NewRequest(http.MethodPost, cfg.Server.APIPrefix+"/captchas/slide-text/"+key+"/attempts", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddrFor(ua)
	w := serve(withBrowserHeaders(req, ua))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "second_key")

	// 通过一次之后再出题：没有降级，仍是默认难度
	second := issueRequest(t, http.MethodPost, "slide-text", map[string]interface{}{"dpr": 2, "env": env}, ua)
	assert.Equal(t, base, difficultyOf(t, second), "成功历史不降低难度")
}

func TestIssueEndpoint_WebdriverEscalates(t *testing.T) {
	cfg := skipWithoutRisk(t)
	base := services.ClampDifficulty(cfg.Captcha.DefaultDifficulty)
	ua := uniqueUA("issue-webdriver")
	env := cleanIssueEnvFor(ua)
	env.Webdriver = true

	w := issueRequest(t, http.MethodPost, "click-text", map[string]interface{}{"dpr": 2, "env": env}, ua)
	assert.GreaterOrEqual(t, difficultyOf(t, w), services.ClampDifficulty(base+1), "webdriver 应至少加 1 级")

	ua2 := uniqueUA("issue-automation")
	env = cleanIssueEnvFor(ua2)
	env.Webdriver = true
	env.Automation = []string{"__playwright"}
	w = issueRequest(t, http.MethodPost, "click-text", map[string]interface{}{"dpr": 2, "env": env}, ua2)
	assert.Equal(t, services.ClampDifficulty(base+2), difficultyOf(t, w), "webdriver + automation 加 2 级")
}

func TestIssueEndpoint_EmptyBodyMatchesGet(t *testing.T) {
	cfg := config.LoadConfig()
	base := services.ClampDifficulty(cfg.Captcha.DefaultDifficulty)
	ua := uniqueUA("issue-empty")

	post := issueBody(t, issueRequest(t, http.MethodPost, "rotate", nil, ua))
	get := issueBody(t, issueRequest(t, http.MethodGet, "rotate", nil, ua))

	assert.Equal(t, base, int(post["difficulty"].(float64)), "不带请求体的 POST 与 GET 等价")
	assert.Equal(t, base, int(get["difficulty"].(float64)), "真实浏览器里的旧版 SDK 走 GET 仍是默认难度")

	keysOf := func(m map[string]interface{}) []string {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		return keys
	}
	assert.ElementsMatch(t, keysOf(get), keysOf(post), "POST 与 GET 的响应字段集合一致")
	for _, field := range []string{"key", "type", "image", "imageWidth", "imageHeight", "scale", "difficulty"} {
		assert.Contains(t, post, field)
	}
}

func TestIssueEndpoint_RejectsOversizedBody(t *testing.T) {
	cfg := config.LoadConfig()
	limit := cfg.Security.VerifyMaxBodySize
	require.Greater(t, limit, int64(0))

	body := `{"dpr":1` + strings.Repeat(" ", int(limit)+1024) + `}`
	req := httptest.NewRequest(http.MethodPost, cfg.Server.APIPrefix+"/captchas/slide-text", strings.NewReader(body))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/json")
	rec := serve(req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestIssueEndpoint_RejectsOversizedEnvList(t *testing.T) {
	ua := uniqueUA("issue-oversized")
	env := cleanIssueEnvFor(ua)
	env.Languages = make([]string, 33)
	for i := range env.Languages {
		env.Languages[i] = "zh"
	}
	w := issueRequest(t, http.MethodPost, "slide-text", map[string]interface{}{"env": env}, ua)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.EqualValues(t, 400, body["code"])
}

func TestIssueEndpoint_TelemetryLogged(t *testing.T) {
	cfg := skipWithoutRisk(t)
	if cfg.Risk.TelemetryLogSize <= 0 {
		t.Skip("遥测日志已关闭")
	}
	ua := uniqueUA("issue-telemetry")
	env := cleanIssueEnvFor(ua)
	env.Webdriver = true
	env.Fingerprint = fmt.Sprintf("fp%030d", time.Now().UnixNano()%1_000_000_000)

	body := issueBody(t, issueRequest(t, http.MethodPost, "click-shape", map[string]interface{}{"dpr": 2, "env": env}, ua))
	found := telemetryEntryFor(t, body["key"].(string))

	assert.Equal(t, true, found["viaPost"])
	assert.Equal(t, env.Fingerprint, found["fp"])
	assert.EqualValues(t, body["difficulty"], found["level"])
	assert.EqualValues(t, cfg.Captcha.DefaultDifficulty, found["base"])
	assert.EqualValues(t, 1, found["boost"])
	assert.EqualValues(t, 45, found["envScore"])
	assert.Equal(t, "click-shape", found["type"])
	assert.Contains(t, fmt.Sprint(found["reasons"]), "webdriver(+45)")
	// 写入即脱敏：原始 IP 与完整 UA 不落 Redis
	assert.Regexp(t, `^\d+\.\d+\.\*\.\*$`, found["ip"])
	assert.Equal(t, "Chrome 130 / Windows", found["ua"])
}

func TestIssueEndpoint_CORSPreflightAllowsPost(t *testing.T) {
	cfg := config.LoadConfig()
	if !cfg.CORS.Enabled {
		t.Skip("CORS_ENABLED=false")
	}
	req := httptest.NewRequest(http.MethodOptions, cfg.Server.APIPrefix+"/captchas/auto", nil)
	origin := "https://app.example.com"
	if len(cfg.CORS.AllowOrigins) > 0 && cfg.CORS.AllowOrigins[0] != "*" {
		origin = cfg.CORS.AllowOrigins[0]
	}
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")
	w := serve(req)

	assert.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
}

// ---------------------------------------------------------------------------
// 请求头核验
// ---------------------------------------------------------------------------

func TestIssueEndpoint_BrowserLikeHeadersStayDefault(t *testing.T) {
	cfg := skipWithoutRisk(t)
	base := services.ClampDifficulty(cfg.Captcha.DefaultDifficulty)
	ua := uniqueUA("issue-consistent")

	body := issueBody(t, issueRequest(t, http.MethodPost, "slide-text", map[string]interface{}{"dpr": 2, "env": cleanIssueEnvFor(ua)}, ua))
	assert.Equal(t, base, int(body["difficulty"].(float64)))

	if cfg.Risk.TelemetryLogSize > 0 {
		entry := telemetryEntryFor(t, body["key"].(string))
		assert.Equal(t, true, entry["consistent"], "请求头齐全且与 env 一致应记录 consistent=true")
		assert.EqualValues(t, 0, entry["envScore"])
	}
}

func TestIssueEndpoint_FakeChromeWithoutBrowserHeadersEscalates(t *testing.T) {
	cfg := skipWithoutRisk(t)
	ua := uniqueUA("issue-fake-chrome")
	// 只有一个冒充 Chrome 的 UA：没有 Sec-Fetch-*、Client Hints、Accept-Language
	req := rawIssueRequest(t, http.MethodPost, "slide-text", map[string]interface{}{"dpr": 2, "env": cleanIssueEnvFor(ua)})
	req.Header.Set("User-Agent", ua)
	req.RemoteAddr = remoteAddrFor(ua)
	body := issueBody(t, serve(req))

	assert.Equal(t, services.MaxDifficulty, int(body["difficulty"].(float64)), "25 + 15 + 15 = 55 分应加 2 级")
	if cfg.Risk.TelemetryLogSize > 0 {
		reasons := fmt.Sprint(telemetryEntryFor(t, body["key"].(string))["reasons"])
		for _, want := range []string{"sec_fetch_missing", "client_hints_missing", "accept_language_missing"} {
			assert.Contains(t, reasons, want)
		}
		assert.NotContains(t, reasons, "non_browser_ua")
	}
}

func TestGetEndpoint_NonBrowserUAEscalates(t *testing.T) {
	cfg := skipWithoutRisk(t)
	base := services.ClampDifficulty(cfg.Captcha.DefaultDifficulty)
	req := rawIssueRequest(t, http.MethodGet, "slide-text", nil)
	curlUA := fmt.Sprintf("curl/8.5.0 t%d", time.Now().UnixNano())
	req.Header.Set("User-Agent", curlUA)
	req.RemoteAddr = remoteAddrFor(curlUA)
	body := issueBody(t, serve(req))

	assert.Equal(t, services.ClampDifficulty(base+1), int(body["difficulty"].(float64)), "20 + 15 = 35 分应加 1 级")
	if cfg.Risk.TelemetryLogSize > 0 {
		entry := telemetryEntryFor(t, body["key"].(string))
		reasons := fmt.Sprint(entry["reasons"])
		assert.Contains(t, reasons, "non_browser_ua")
		assert.Contains(t, reasons, "accept_language_missing")
		assert.Equal(t, false, entry["consistent"])
		assert.Equal(t, false, entry["viaPost"])
	}
}

func TestIssueEndpoint_UAMismatchEscalates(t *testing.T) {
	cfg := skipWithoutRisk(t)
	base := services.ClampDifficulty(cfg.Captcha.DefaultDifficulty)
	ua := uniqueUA("issue-ua-mismatch")
	env := cleanIssueEnvFor(ua)
	env.UA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:131.0) Gecko/20100101 Firefox/131.0"

	body := issueBody(t, issueRequest(t, http.MethodPost, "slide-text", map[string]interface{}{"dpr": 2, "env": env}, ua))
	assert.Equal(t, services.ClampDifficulty(base+1), int(body["difficulty"].(float64)), "ua_mismatch 30 分加 1 级")
	if cfg.Risk.TelemetryLogSize > 0 {
		assert.Contains(t, fmt.Sprint(telemetryEntryFor(t, body["key"].(string))["reasons"]), "ua_mismatch")
	}
}
