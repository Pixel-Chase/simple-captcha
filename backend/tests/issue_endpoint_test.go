package tests

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(method, config.APIPrefix+"/captchas/"+captchaType, bytes.NewReader(payload))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return asConsoleApp(req)
}

// asConsoleApp 以内置系统应用（管理后台，只接受同源）的身份访问用户侧接口：带 appId 查询参数与同源 Origin
func asConsoleApp(req *http.Request) *http.Request {
	q := req.URL.Query()
	q.Set("appId", services.ConsoleAppID)
	req.URL.RawQuery = q.Encode()
	if req.Header.Get("Origin") == "" {
		req.Header.Set("Origin", "http://"+req.Host)
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

// telemetryEntryFor 从风控观察记录里找出该题目的出题评估记录
func telemetryEntryFor(t *testing.T, key string) map[string]interface{} {
	items, err := services.NewClientRiskStore().RecentTelemetry(100)
	require.NoError(t, err)
	for _, entry := range items {
		if entry["key"] == key && entry["kind"] == "issue" {
			return entry
		}
	}
	require.FailNow(t, "出题评估应写入风控观察记录", "key=%s", key)
	return nil
}

// skipWithoutRisk 风控评分关闭时跳过用例，返回当前策略快照
func skipWithoutRisk(t *testing.T) *config.RuntimeSettings {
	rt := config.Runtime()
	if !rt.Risk.Enabled {
		t.Skip("策略配置中关闭了风控评分")
	}
	return rt
}

func TestIssueEndpoint_CleanEnvStaysDefaultBeforeAndAfterSuccess(t *testing.T) {
	rt := config.Runtime()
	base := services.ClampDifficulty(rt.Captcha.DefaultDifficulty)
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
	req := asConsoleApp(httptest.NewRequest(http.MethodPost, config.APIPrefix+"/captchas/slide-text/"+key+"/attempts", bytes.NewReader(payload)))
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
	rt := skipWithoutRisk(t)
	base := services.ClampDifficulty(rt.Captcha.DefaultDifficulty)
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

// 出题只接受 POST：不带请求体时按默认难度出题；旧版 GET 出题已删除，返回 404
func TestIssueEndpoint_EmptyBodyAndGetRemoved(t *testing.T) {
	rt := config.Runtime()
	base := services.ClampDifficulty(rt.Captcha.DefaultDifficulty)
	ua := uniqueUA("issue-empty")

	post := issueBody(t, issueRequest(t, http.MethodPost, "rotate", nil, ua))
	assert.Equal(t, base, int(post["difficulty"].(float64)), "不带请求体的 POST 按默认难度出题")
	for _, field := range []string{"key", "type", "image", "imageWidth", "imageHeight", "scale", "difficulty"} {
		assert.Contains(t, post, field)
	}

	w := serve(rawIssueRequest(t, http.MethodGet, "rotate", nil))
	assert.Equal(t, http.StatusNotFound, w.Code, "GET /api/v1/captchas/rotate 已删除")
}

func TestIssueEndpoint_RejectsOversizedBody(t *testing.T) {
	cfg := config.LoadConfig()
	limit := cfg.Security.VerifyMaxBodySize
	require.Greater(t, limit, int64(0))

	body := `{"dpr":1` + strings.Repeat(" ", int(limit)+1024) + `}`
	req := asConsoleApp(httptest.NewRequest(http.MethodPost, config.APIPrefix+"/captchas/slide-text", strings.NewReader(body)))
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
	rt := skipWithoutRisk(t)
	if rt.Risk.TelemetryRetention <= 0 {
		t.Skip("遥测日志已关闭")
	}
	ua := uniqueUA("issue-telemetry")
	env := cleanIssueEnvFor(ua)
	env.Webdriver = true
	env.Fingerprint = fmt.Sprintf("fp%030d", time.Now().UnixNano()%1_000_000_000)

	body := issueBody(t, issueRequest(t, http.MethodPost, "click-shape", map[string]interface{}{"dpr": 2, "env": env}, ua))
	found := telemetryEntryFor(t, body["key"].(string))

	assert.Equal(t, env.Fingerprint, found["fp"])
	assert.EqualValues(t, body["difficulty"], found["level"])
	assert.EqualValues(t, rt.Captcha.DefaultDifficulty, found["base"])
	assert.EqualValues(t, 1, found["boost"])
	assert.EqualValues(t, 45, found["envScore"])
	assert.Equal(t, "click-shape", found["type"])
	assert.Contains(t, fmt.Sprint(found["reasons"]), "webdriver(+45)")
	// 写入即脱敏：原始 IP 与完整 UA 不落库
	assert.Regexp(t, `^\d+\.\d+\.\*\.\*$`, found["ip"])
	assert.Equal(t, "Chrome 130 / Windows", found["ua"])
}

// 预检对任意来源都放行（拦截在真实请求上），否则 SDK 读不到错误码
func TestIssueEndpoint_CORSPreflightAllowsPost(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, config.APIPrefix+"/captchas/auto?appId=app_000000000000", nil)
	origin := "https://app.example.com"
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")
	w := serve(req)

	assert.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
	assert.Equal(t, origin, w.Header().Get("Access-Control-Allow-Origin"))
}

// ---------------------------------------------------------------------------
// 请求头核验
// ---------------------------------------------------------------------------

func TestIssueEndpoint_BrowserLikeHeadersStayDefault(t *testing.T) {
	rt := skipWithoutRisk(t)
	base := services.ClampDifficulty(rt.Captcha.DefaultDifficulty)
	ua := uniqueUA("issue-consistent")

	body := issueBody(t, issueRequest(t, http.MethodPost, "slide-text", map[string]interface{}{"dpr": 2, "env": cleanIssueEnvFor(ua)}, ua))
	assert.Equal(t, base, int(body["difficulty"].(float64)))

	if rt.Risk.TelemetryRetention > 0 {
		entry := telemetryEntryFor(t, body["key"].(string))
		assert.Equal(t, true, entry["consistent"], "请求头齐全且与 env 一致应记录 consistent=true")
		assert.EqualValues(t, 0, entry["envScore"])
	}
}

func TestIssueEndpoint_FakeChromeWithoutBrowserHeadersEscalates(t *testing.T) {
	rt := skipWithoutRisk(t)
	ua := uniqueUA("issue-fake-chrome")
	// 只有一个冒充 Chrome 的 UA：没有 Sec-Fetch-*、Client Hints、Accept-Language
	req := rawIssueRequest(t, http.MethodPost, "slide-text", map[string]interface{}{"dpr": 2, "env": cleanIssueEnvFor(ua)})
	req.Header.Set("User-Agent", ua)
	req.RemoteAddr = remoteAddrFor(ua)
	body := issueBody(t, serve(req))

	assert.Equal(t, services.MaxDifficulty, int(body["difficulty"].(float64)), "25 + 15 + 15 = 55 分应加 2 级")
	if rt.Risk.TelemetryRetention > 0 {
		reasons := fmt.Sprint(telemetryEntryFor(t, body["key"].(string))["reasons"])
		for _, want := range []string{"sec_fetch_missing", "client_hints_missing", "accept_language_missing"} {
			assert.Contains(t, reasons, want)
		}
		assert.NotContains(t, reasons, "non_browser_ua")
	}
}

func TestIssueEndpoint_NonBrowserUAEscalates(t *testing.T) {
	rt := skipWithoutRisk(t)
	base := services.ClampDifficulty(rt.Captcha.DefaultDifficulty)
	req := rawIssueRequest(t, http.MethodPost, "slide-text", nil)
	curlUA := fmt.Sprintf("curl/8.5.0 t%d", time.Now().UnixNano())
	req.Header.Set("User-Agent", curlUA)
	req.RemoteAddr = remoteAddrFor(curlUA)
	body := issueBody(t, serve(req))

	assert.Equal(t, services.ClampDifficulty(base+1), int(body["difficulty"].(float64)), "20 + 15 = 35 分应加 1 级")
	if rt.Risk.TelemetryRetention > 0 {
		entry := telemetryEntryFor(t, body["key"].(string))
		reasons := fmt.Sprint(entry["reasons"])
		assert.Contains(t, reasons, "non_browser_ua")
		assert.Contains(t, reasons, "accept_language_missing")
		assert.Equal(t, false, entry["consistent"])
	}
}

func TestIssueEndpoint_UAMismatchEscalates(t *testing.T) {
	rt := skipWithoutRisk(t)
	base := services.ClampDifficulty(rt.Captcha.DefaultDifficulty)
	ua := uniqueUA("issue-ua-mismatch")
	env := cleanIssueEnvFor(ua)
	env.UA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:131.0) Gecko/20100101 Firefox/131.0"

	body := issueBody(t, issueRequest(t, http.MethodPost, "slide-text", map[string]interface{}{"dpr": 2, "env": env}, ua))
	assert.Equal(t, services.ClampDifficulty(base+1), int(body["difficulty"].(float64)), "ua_mismatch 30 分加 1 级")
	if rt.Risk.TelemetryRetention > 0 {
		assert.Contains(t, fmt.Sprint(telemetryEntryFor(t, body["key"].(string))["reasons"]), "ua_mismatch")
	}
}
