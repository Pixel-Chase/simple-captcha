package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"simple-captcha/helper"
	"simple-captcha/services"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// globalApp 在全局服务管理器（完整路由使用的那份库）中创建一个应用，测试结束时删除
func globalApp(t *testing.T, name string, origins ...string) *services.AppInfo {
	t.Helper()
	apps := services.GetServiceManager().GetAppService()
	app, _, cerr := apps.Create(name, origins)
	require.Nil(t, cerr)
	t.Cleanup(func() { _ = apps.Delete(app.ID) })
	return app
}

// uniqueAppName 全局库在多个用例间共用，名称加时间戳避免重名
func uniqueAppName(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()%1_000_000_000)
}

// userRequest 发往完整路由的用户侧请求；origin 为空时不带 Origin 头
func userRequest(method, path, appID, origin string, headers map[string]string, remote string) *httptest.ResponseRecorder {
	if appID != "" {
		path += "?appId=" + appID
	}
	req := httptest.NewRequest(method, path, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if remote != "" {
		req.RemoteAddr = remote
	}
	w := httptest.NewRecorder()
	router().ServeHTTP(w, req)
	return w
}

func TestOrigins_Matrix(t *testing.T) {
	app := globalApp(t, uniqueAppName("官网"), "https://www.example.com", "http://localhost:8000")
	const issue = "/api/v1/captchas/slide-text"

	t.Run("名单内的 Origin 出题成功并回显来源", func(t *testing.T) {
		w := userRequest(http.MethodPost, issue, app.ID, "https://www.example.com", nil, "")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "https://www.example.com", w.Header().Get("Access-Control-Allow-Origin"))
		key := decodeBody(t, w)["key"].(string)

		// 提交同样经过来源校验：空答案在来源校验之后才被参数校验拒绝（400），而不是 403
		w = userRequest(http.MethodPost, issue+"/"+key+"/attempts", app.ID, "http://localhost:8000", nil, "")
		assert.NotEqual(t, http.StatusForbidden, w.Code, w.Body.String())
		assert.Equal(t, "http://localhost:8000", w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("名单外或缺少 Origin 返回 403 / 1011", func(t *testing.T) {
		for _, origin := range []string{"https://evil.example", "https://www.example.com:8443", ""} {
			w := userRequest(http.MethodPost, issue, app.ID, origin, nil, "")
			assert.Equal(t, http.StatusForbidden, w.Code, origin)
			assert.Equal(t, helper.ErrCodeOriginNotAllowed, errorCode(t, w), origin)
			assert.NotContains(t, w.Body.String(), "www.example.com", "拒绝响应不透露名单")
		}
		// 提交接口同样拦截
		w := userRequest(http.MethodPost, issue+"/"+strings.Repeat("a", 32)+"/attempts", app.ID, "https://evil.example", nil, "")
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("应用不存在或已停用返回 1010", func(t *testing.T) {
		for _, appID := range []string{"", "app_000000000000", "bogus"} {
			w := userRequest(http.MethodPost, issue, appID, "https://www.example.com", nil, "")
			assert.Equal(t, http.StatusForbidden, w.Code, appID)
			assert.Equal(t, helper.ErrCodeAppInvalid, errorCode(t, w), appID)
		}
		disabled := globalApp(t, uniqueAppName("停用"), "https://www.example.com")
		_, cerr := services.GetServiceManager().GetAppService().Update(disabled.ID, disabled.Name, false, nil)
		require.Nil(t, cerr)
		w := userRequest(http.MethodPost, issue, disabled.ID, "https://www.example.com", nil, "")
		assert.Equal(t, helper.ErrCodeAppInvalid, errorCode(t, w))
	})

	t.Run("系统应用只接受同源", func(t *testing.T) {
		// httptest 默认 Host 为 example.com
		w := userRequest(http.MethodPost, issue, services.ConsoleAppID, "http://example.com", nil, "")
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
		w = userRequest(http.MethodPost, issue, services.ConsoleAppID, "https://www.example.com", nil, "")
		assert.Equal(t, helper.ErrCodeOriginNotAllowed, errorCode(t, w))
	})

	t.Run("不可信来源携带的 X-Forwarded-Host 被忽略", func(t *testing.T) {
		forged := map[string]string{"X-Forwarded-Host": "captcha.example.com", "X-Forwarded-Proto": "https"}
		w := userRequest(http.MethodPost, issue, services.ConsoleAppID, "https://captcha.example.com", forged, "203.0.113.7:4000")
		assert.Equal(t, http.StatusForbidden, w.Code)
		// 来自可信代理（默认 TRUSTED_PROXIES 含 10.0.0.0/8）时采信
		w = userRequest(http.MethodPost, issue, services.ConsoleAppID, "https://captcha.example.com", forged, "10.9.8.7:4000")
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})

	t.Run("预检对任意 Origin 返回 204", func(t *testing.T) {
		for _, path := range []string{issue, issue + "/" + strings.Repeat("a", 32) + "/attempts"} {
			w := userRequest(http.MethodOptions, path, "app_000000000000", "https://anything.example",
				map[string]string{"Access-Control-Request-Method": "POST"}, "")
			assert.Equal(t, http.StatusNoContent, w.Code, path)
			assert.Equal(t, "https://anything.example", w.Header().Get("Access-Control-Allow-Origin"))
		}
	})
}

func TestOrigins_AdminManagesOrigins(t *testing.T) {
	h := newAdminHarness(t, 1)
	token := h.login(t)

	w := h.doJSON(http.MethodPost, "/apps", map[string]interface{}{
		"name":    "订单系统",
		"origins": []string{"HTTPS://Shop.Example.com/", "https://shop.example.com:443", "http://localhost:8000"},
	}, token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	app := decodeBody(t, w)["app"].(map[string]interface{})
	assert.Equal(t, []interface{}{"https://shop.example.com", "http://localhost:8000"}, app["origins"], "规范化并去重")
	assert.Equal(t, false, app["isSystem"])
	id := app["id"].(string)

	// 修改来源：提供时整体替换，缺省时保持不变
	w = h.doJSON(http.MethodPut, "/apps/"+id, map[string]interface{}{"name": "订单系统", "enabled": true, "origins": []string{"https://m.example.com"}}, token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, []interface{}{"https://m.example.com"}, decodeBody(t, w)["origins"])
	w = h.doJSON(http.MethodPut, "/apps/"+id, map[string]interface{}{"name": "订单系统 2", "enabled": true}, token)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []interface{}{"https://m.example.com"}, decodeBody(t, w)["origins"])

	// 非法来源与超出数量返回 400，提示中文原因
	for _, bad := range [][]string{{"https://*.example.com"}, {"https://example.com/path"}, {"ftp://example.com"}} {
		w = h.doJSON(http.MethodPut, "/apps/"+id, map[string]interface{}{"name": "订单系统 2", "enabled": true, "origins": bad}, token)
		assert.Equal(t, http.StatusBadRequest, w.Code, bad)
		assert.Equal(t, helper.ErrCodeInvalidParam, errorCode(t, w))
	}
	many := make([]string, 21)
	for i := range many {
		many[i] = "https://s" + strings.Repeat("x", i+1) + ".example.com"
	}
	w = h.doJSON(http.MethodPost, "/apps", map[string]interface{}{"name": "太多来源", "origins": many}, token)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 列表带 origins 与 isSystem，系统应用排在最前
	all := h.listAllApps(t, token)
	require.Len(t, all, 2)
	system := all[0].(map[string]interface{})
	assert.Equal(t, services.ConsoleAppID, system["id"])
	assert.Equal(t, true, system["isSystem"])
	assert.Equal(t, []interface{}{}, system["origins"])
	assert.Equal(t, []interface{}{"https://m.example.com"}, all[1].(map[string]interface{})["origins"])

	// 删除应用时来源一并删除
	w = h.do(http.MethodDelete, "/apps/"+id, nil, token, "")
	require.Equal(t, http.StatusNoContent, w.Code)
	var left int
	require.NoError(t, h.db.QueryRow(`SELECT COUNT(1) FROM app_origins WHERE app_id = ?`, id).Scan(&left))
	assert.Zero(t, left)
}

func TestOrigins_SystemAppReadonly(t *testing.T) {
	h := newAdminHarness(t, 1)
	token := h.login(t)
	id := services.ConsoleAppID

	cases := []struct {
		name string
		w    *httptest.ResponseRecorder
	}{
		{"删除", h.do(http.MethodDelete, "/apps/"+id, nil, token, "")},
		{"停用", h.doJSON(http.MethodPut, "/apps/"+id, map[string]interface{}{"name": "管理后台", "enabled": false}, token)},
		{"改名", h.doJSON(http.MethodPut, "/apps/"+id, map[string]interface{}{"name": "后台", "enabled": true}, token)},
		{"修改来源", h.doJSON(http.MethodPut, "/apps/"+id, map[string]interface{}{"name": "管理后台", "enabled": true, "origins": []string{"https://a.com"}}, token)},
		{"轮换密钥", h.do(http.MethodPost, "/apps/"+id+"/secret", nil, token, "")},
	}
	for _, tc := range cases {
		assert.Equal(t, http.StatusBadRequest, tc.w.Code, tc.name)
		assert.Equal(t, helper.ErrCodeSystemAppReadonly, errorCode(t, tc.w), tc.name)
	}

	// 系统应用没有密钥，空串或伪造的密钥都不能通过 S2S 鉴权
	_, ok := h.apps.Authenticate("")
	assert.False(t, ok)
	w := h.queryState(t, "sk_"+strings.Repeat("0", 48), strings.Repeat("a", 32))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// 管理接口与 S2S 状态查询不返回任何跨域响应头
func TestOrigins_AdminAndStateHaveNoCORSHeaders(t *testing.T) {
	for _, path := range []string{"/api/v1/admin/setup", "/api/v1/admin/apps", "/api/v1/captchas/states/" + strings.Repeat("a", 32)} {
		for _, method := range []string{http.MethodGet, http.MethodOptions} {
			req := httptest.NewRequest(method, path, nil)
			req.Header.Set("Origin", "https://evil.example")
			req.Header.Set("Access-Control-Request-Method", "GET")
			w := httptest.NewRecorder()
			router().ServeHTTP(w, req)
			assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"), "%s %s", method, path)
		}
	}
}
