package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"simple-captcha/helper"
	"simple-captcha/services"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// boundApp 全局库中的接入应用及其密钥与来源
type boundApp struct {
	id, secret, origin string
}

func newBoundApp(t *testing.T, prefix, origin string) boundApp {
	t.Helper()
	apps := services.GetServiceManager().GetAppService()
	app, secret, cerr := apps.Create(uniqueAppName(prefix), []string{origin})
	require.Nil(t, cerr)
	t.Cleanup(func() { _ = apps.Delete(app.ID) })
	return boundApp{id: app.ID, secret: secret, origin: origin}
}

// consoleApp 内置系统应用：只接受同源（httptest 默认 Host 为 example.com）
var consoleApp = boundApp{id: services.ConsoleAppID, origin: "http://example.com"}

// asApp 以 app 的身份发起用户侧请求：appId 查询参数 + 该应用允许的 Origin，并补齐真实浏览器的请求头
func asApp(req *http.Request, app boundApp, ua string) *http.Request {
	q := req.URL.Query()
	q.Set("appId", app.id)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Origin", app.origin)
	req.RemoteAddr = remoteAddrFor(ua)
	return withBrowserHeaders(req, ua)
}

// issueAs 以 app 的身份出一道滑块题，返回 key 与正确答案
func issueAs(t *testing.T, app boundApp, ua string) (string, map[string]interface{}) {
	t.Helper()
	payload, _ := json.Marshal(map[string]interface{}{"dpr": 2, "env": cleanIssueEnvFor(ua)})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/captchas/slide-text", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := serve(asApp(req, app, ua))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	key := issueBody(t, w)["key"].(string)

	answer, meta := slideAnswer(t, key)
	attempt := map[string]interface{}{"point": answer, "env": cleanIssueEnvFor(ua)}
	if pow := solvePow(t, key, meta); pow != nil {
		attempt["pow"] = pow
	}
	return key, attempt
}

// submitAs 以 app 的身份提交答案，返回响应体
func submitAs(t *testing.T, app boundApp, ua, key string, attempt map[string]interface{}) map[string]interface{} {
	t.Helper()
	payload, _ := json.Marshal(attempt)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/captchas/slide-text/"+key+"/attempts", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := serve(asApp(req, app, ua))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

// passAs 以 app 的身份完整通过一次验证，返回 second_key
func passAs(t *testing.T, app boundApp, ua string) string {
	t.Helper()
	key, attempt := issueAs(t, app, ua)
	waitMinSolveTime()
	body := submitAs(t, app, ua, key, attempt)
	secondKey, _ := body["second_key"].(string)
	require.NotEmpty(t, secondKey, "应通过验证: %v", body)
	return secondKey
}

// queryStateAs 业务后端用应用密钥查询 second_key
func queryStateAs(t *testing.T, secret, secondKey string) bool {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/captchas/states/"+secondKey, nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	w := serve(req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	if body["valid"] == true {
		return true
	}
	assert.EqualValues(t, helper.ErrCodeExpired, body["code"], "无效 key 与其他应用的 key 响应相同")
	return false
}

func TestAppBinding_SecondKeyOnlyForIssuingApp(t *testing.T) {
	a := newBoundApp(t, "A", "https://a.example.com")
	b := newBoundApp(t, "B", "https://b.example.com")
	ua := uniqueUA("binding-s2s")

	secondKey := passAs(t, a, ua)

	// B 的密钥查询 A 的 key：无效，且不消费
	assert.False(t, queryStateAs(t, b.secret, secondKey))
	assert.False(t, queryStateAs(t, b.secret, secondKey))
	// A 的密钥：有效，只能消费一次
	assert.True(t, queryStateAs(t, a.secret, secondKey))
	assert.False(t, queryStateAs(t, a.secret, secondKey))
}

func TestAppBinding_SubmitMustMatchIssuingApp(t *testing.T) {
	a := newBoundApp(t, "A", "https://a.example.com")
	b := newBoundApp(t, "B", "https://b.example.com")
	ua := uniqueUA("binding-submit")

	key, attempt := issueAs(t, a, ua)
	waitMinSolveTime()

	// 以 B 的身份提交 A 的题：与题目不存在相同（1004），不计尝试次数也不作废题目
	body := submitAs(t, b, ua, key, attempt)
	assert.EqualValues(t, helper.ErrCodeExpired, body["code"])
	record, cerr := services.NewBaseService().GetCaptchaData(key)
	require.Nil(t, cerr)
	assert.EqualValues(t, 0, record["attempts"])

	// A 自己仍能正常通过
	body = submitAs(t, a, ua, key, attempt)
	assert.NotEmpty(t, body["second_key"], "%v", body)
}

func TestAppBinding_AdminLoginOnlyAcceptsConsoleKeys(t *testing.T) {
	a := newBoundApp(t, "A", "https://a.example.com")
	h := newAdminHarness(t, 1)
	h.initialize(t)
	login := func(captchaKey string) *httptest.ResponseRecorder {
		return h.doJSON(http.MethodPost, "/login", map[string]string{
			"username": adminTestUser, "password": h.password, "captchaKey": captchaKey,
		}, "")
	}

	// 接入应用里通过的验证码不能拿来登录管理后台，也不会因此被消费
	appKey := passAs(t, a, uniqueUA("binding-login-app"))
	w := login(appKey)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, helper.ErrCodeCaptchaRequired, errorCode(t, w))
	assert.True(t, queryStateAs(t, a.secret, appKey), "登录失败不应消费 A 的 key")

	// 管理后台系统应用的 key 不能通过 S2S 消费，只能用于登录
	consoleKey := passAs(t, consoleApp, uniqueUA("binding-login-console"))
	assert.False(t, queryStateAs(t, a.secret, consoleKey))
	w = login(consoleKey)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestAppBinding_TelemetryRecordsApp(t *testing.T) {
	a := newBoundApp(t, "A", "https://a.example.com")
	ua := uniqueUA("binding-telemetry")
	key, attempt := issueAs(t, a, ua)
	waitMinSolveTime()
	submitAs(t, a, ua, key, attempt)

	manager := services.GetServiceManager()
	manager.GetTelemetryStore().Flush()
	rows, err := manager.GetStorage().QueryContext(context.Background(),
		`SELECT kind, app_id FROM telemetry WHERE payload LIKE ? ORDER BY id`, "%"+key+"%")
	require.NoError(t, err)
	defer rows.Close()
	kinds := map[string]string{}
	for rows.Next() {
		var kind string
		var appID *string
		require.NoError(t, rows.Scan(&kind, &appID))
		require.NotNil(t, appID, kind)
		kinds[kind] = *appID
	}
	assert.Equal(t, map[string]string{"issue": a.id, "verify": a.id}, kinds)

	entry := telemetryEntryFor(t, key)
	assert.Equal(t, a.id, entry["app"], "管理接口返回的记录带有应用 ID")
}
