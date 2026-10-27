package middleware

import (
	"net/http"
	"net/http/httptest"
	"simple-captcha/services"
	"simple-captcha/storage"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func originRouter(t *testing.T) (*gin.Engine, *services.AppService) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	apps := services.NewAppService(storage.OpenTemp(t))
	r := gin.New()
	mw := AppOriginMiddleware(apps)
	r.OPTIONS("/captchas/:type", mw)
	r.POST("/captchas/:type", mw, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"app": GetRequestAppID(c)})
	})
	return r, apps
}

func send(r http.Handler, method, target string, headers map[string]string, remote string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if remote != "" {
		req.RemoteAddr = remote
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// 预检对任意来源（包括不存在的应用）都返回 204 并回显来源
func TestAppOrigin_PreflightAlwaysPasses(t *testing.T) {
	r, _ := originRouter(t)
	w := send(r, http.MethodOptions, "/captchas/auto?appId=app_000000000000", map[string]string{
		"Origin":                        "https://evil.example",
		"Access-Control-Request-Method": "POST",
	}, "")
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "https://evil.example", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "POST, OPTIONS", w.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Content-Type", w.Header().Get("Access-Control-Allow-Headers"))
	assert.Equal(t, "86400", w.Header().Get("Access-Control-Max-Age"))
	assert.Contains(t, w.Header().Values("Vary"), "Origin")
}

func TestAppOrigin_ConsoleAppSameOriginOnly(t *testing.T) {
	r, _ := originRouter(t)
	target := "http://captcha.local:3321/captchas/auto?appId=app_console"

	w := send(r, http.MethodPost, target, map[string]string{"Origin": "http://captcha.local:3321"}, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"app":"app_console"`)
	assert.Equal(t, "http://captcha.local:3321", w.Header().Get("Access-Control-Allow-Origin"))

	// 跨域与缺少 Origin 都拒绝；拒绝响应同样回显来源，SDK 才读得到错误码
	w = send(r, http.MethodPost, target, map[string]string{"Origin": "https://other.example"}, "")
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), `"code":1011`)
	assert.Equal(t, "https://other.example", w.Header().Get("Access-Control-Allow-Origin"))

	w = send(r, http.MethodPost, target, nil, "")
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), `"code":1011`)
}

// 只有直连来源是可信代理时才采信 X-Forwarded-Host / X-Forwarded-Proto
func TestAppOrigin_ForwardedHeadersOnlyFromTrustedProxy(t *testing.T) {
	r, _ := originRouter(t)
	target := "http://backend:3321/captchas/auto?appId=app_console"
	headers := map[string]string{
		"Origin":            "https://captcha.example.com",
		"X-Forwarded-Host":  "captcha.example.com",
		"X-Forwarded-Proto": "https",
	}

	// 默认 TRUSTED_PROXIES 含 10.0.0.0/8：经反向代理转发，按转发头判定为同源
	w := send(r, http.MethodPost, target, headers, "10.1.2.3:5000")
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// 公网直连伪造的转发头被忽略
	w = send(r, http.MethodPost, target, headers, "203.0.113.9:5000")
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), `"code":1011`)
}

func TestAppOrigin_UnknownOrDisabledApp(t *testing.T) {
	r, apps := originRouter(t)
	for _, appID := range []string{"", "nope", "app_000000000000"} {
		w := send(r, http.MethodPost, "/captchas/auto?appId="+appID, map[string]string{"Origin": "https://www.example.com"}, "")
		assert.Equal(t, http.StatusForbidden, w.Code, appID)
		assert.Contains(t, w.Body.String(), `"code":1010`, appID)
	}

	app, _, cerr := apps.Create("官网", []string{"https://www.example.com"})
	require.Nil(t, cerr)
	w := send(r, http.MethodPost, "/captchas/auto?appId="+app.ID, map[string]string{"Origin": "https://www.example.com"}, "")
	assert.Equal(t, http.StatusOK, w.Code)

	_, cerr = apps.Update(app.ID, app.Name, false, nil)
	require.Nil(t, cerr)
	w = send(r, http.MethodPost, "/captchas/auto?appId="+app.ID, map[string]string{"Origin": "https://www.example.com"}, "")
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), `"code":1010`)
	// 拒绝响应不透露名单内容
	assert.NotContains(t, w.Body.String(), "www.example.com")
}
