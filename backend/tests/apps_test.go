package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"simple-captcha/controllers"
	"simple-captcha/helper"
	"simple-captcha/middleware"
	"simple-captcha/services"
	"simple-captcha/storage"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	appIDFormat     = regexp.MustCompile(`^app_[0-9a-f]{12}$`)
	appSecretFormat = regexp.MustCompile(`^sk_[0-9a-f]{48}$`)
)

// createApp 通过管理接口新建应用，返回应用信息与一次性明文密钥
func (h *adminHarness) createApp(t *testing.T, token, name string) (map[string]interface{}, string) {
	t.Helper()
	w := h.doJSON(http.MethodPost, "/apps", map[string]string{"name": name}, token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decodeBody(t, w)
	return body["app"].(map[string]interface{}), body["secret"].(string)
}

// listApps 列出接入应用，不含内置的系统应用（见 listAllApps）
func (h *adminHarness) listApps(t *testing.T, token string) []interface{} {
	t.Helper()
	items := make([]interface{}, 0)
	for _, item := range h.listAllApps(t, token) {
		if item.(map[string]interface{})["isSystem"] != true {
			items = append(items, item)
		}
	}
	return items
}

func (h *adminHarness) listAllApps(t *testing.T, token string) []interface{} {
	t.Helper()
	w := h.do(http.MethodGet, "/apps", nil, token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	return decodeBody(t, w)["items"].([]interface{})
}

// stateRouter 只挂 S2S 状态查询接口，鉴权使用 harness 的应用服务
func (h *adminHarness) stateRouter() *gin.Engine {
	r := gin.New()
	r.GET("/api/v1/captchas/states/:key",
		middleware.AppAuthMiddleware(h.apps),
		middleware.ValidateStateRequest(),
		controllers.ValidateCaptchaHandler)
	return r
}

func (h *adminHarness) queryState(t *testing.T, secret, key string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/captchas/states/"+key, nil)
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	w := httptest.NewRecorder()
	h.stateRouter().ServeHTTP(w, req)
	return w
}

func TestApps_CreateAndList(t *testing.T) {
	h := newAdminHarness(t, 1)
	token := h.login(t)

	assert.Empty(t, h.listApps(t, token))

	app, secret := h.createApp(t, token, "  订单系统  ")
	assert.Regexp(t, appIDFormat, app["id"])
	assert.Regexp(t, appSecretFormat, secret)
	assert.Equal(t, "订单系统", app["name"], "名称去掉首尾空白")
	assert.Equal(t, true, app["enabled"])
	assert.Equal(t, secret[:10], app["secretHint"])
	assert.Nil(t, app["lastUsedAt"])
	assert.NotContains(t, app, "secretHash")

	// 列表不返回明文与哈希
	items := h.listApps(t, token)
	require.Len(t, items, 1)
	listed := items[0].(map[string]interface{})
	assert.Equal(t, app["id"], listed["id"])
	assert.NotContains(t, listed, "secret")
	assert.NotContains(t, listed, "secretHash")

	// 库中只有哈希，数据库文件权限仅属主可读写；不再生成 apps.json
	assertFileLacks(t, h.dataDir, secret)
	info, err := os.Stat(filepath.Join(h.dataDir, storage.FileName))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	_, err = os.Stat(filepath.Join(h.dataDir, "apps.json"))
	assert.True(t, os.IsNotExist(err))

	// 另开一个连接读取同一个库：应用仍在，原密钥仍可鉴权
	db, err := storage.Open(h.dataDir)
	require.NoError(t, err)
	defer db.Close()
	reloaded := services.NewAppService(db)
	assert.Equal(t, 1, reloaded.Count())
	_, ok := reloaded.Authenticate(secret)
	assert.True(t, ok)
}

func TestApps_CreateValidation(t *testing.T) {
	h := newAdminHarness(t, 1)
	token := h.login(t)
	h.createApp(t, token, "Order")

	cases := []struct {
		name    string
		payload interface{}
		status  int
		code    int
	}{
		{"缺少名称", map[string]string{}, http.StatusBadRequest, helper.ErrCodeInvalidParam},
		{"全是空白", map[string]string{"name": "   "}, http.StatusBadRequest, helper.ErrCodeInvalidParam},
		{"超过 40 个字符", map[string]string{"name": strings.Repeat("应", 41)}, http.StatusBadRequest, helper.ErrCodeInvalidParam},
		{"控制字符", map[string]string{"name": "a\nb"}, http.StatusBadRequest, helper.ErrCodeInvalidParam},
		{"重名（不区分大小写）", map[string]string{"name": "order"}, http.StatusConflict, helper.ErrCodeAppNameConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := h.doJSON(http.MethodPost, "/apps", tc.payload, token)
			assert.Equal(t, tc.status, w.Code, w.Body.String())
			assert.Equal(t, tc.code, errorCode(t, w))
		})
	}
	assert.Len(t, h.listApps(t, token), 1)
}

func TestApps_UpdateRotateDelete(t *testing.T) {
	h := newAdminHarness(t, 1)
	token := h.login(t)
	app, secret := h.createApp(t, token, "会员中心")
	h.createApp(t, token, "订单系统")
	id := app["id"].(string)

	// 改名与停用
	w := h.doJSON(http.MethodPut, "/apps/"+id, map[string]interface{}{"name": "会员服务", "enabled": false}, token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	updated := decodeBody(t, w)
	assert.Equal(t, "会员服务", updated["name"])
	assert.Equal(t, false, updated["enabled"])
	_, ok := h.apps.Authenticate(secret)
	assert.False(t, ok, "停用后密钥立即失效")

	// 不能改成其他应用的名字；enabled 必填
	w = h.doJSON(http.MethodPut, "/apps/"+id, map[string]interface{}{"name": "订单系统", "enabled": true}, token)
	assert.Equal(t, http.StatusConflict, w.Code)
	w = h.doJSON(http.MethodPut, "/apps/"+id, map[string]interface{}{"name": "会员服务"}, token)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 重新启用后原密钥恢复可用
	w = h.doJSON(http.MethodPut, "/apps/"+id, map[string]interface{}{"name": "会员服务", "enabled": true}, token)
	require.Equal(t, http.StatusOK, w.Code)
	_, ok = h.apps.Authenticate(secret)
	assert.True(t, ok)

	// 重置密钥：旧密钥立即失效，新密钥可用
	w = h.do(http.MethodPost, "/apps/"+id+"/secret", nil, token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decodeBody(t, w)
	rotated := body["secret"].(string)
	assert.Regexp(t, appSecretFormat, rotated)
	assert.NotEqual(t, secret, rotated)
	assert.Equal(t, rotated[:10], body["app"].(map[string]interface{})["secretHint"])
	_, ok = h.apps.Authenticate(secret)
	assert.False(t, ok)
	_, ok = h.apps.Authenticate(rotated)
	assert.True(t, ok)

	// 删除后密钥失效，再删返回 404
	w = h.do(http.MethodDelete, "/apps/"+id, nil, token, "")
	assert.Equal(t, http.StatusNoContent, w.Code)
	_, ok = h.apps.Authenticate(rotated)
	assert.False(t, ok)
	assert.Len(t, h.listApps(t, token), 1)

	w = h.do(http.MethodDelete, "/apps/"+id, nil, token, "")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, helper.ErrCodeAppNotFound, errorCode(t, w))

	// 非法 ID 一律按不存在处理
	for _, bad := range []string{"app_123", "../apps", "APP_0123456789AB"} {
		w = h.do(http.MethodPost, "/apps/"+bad+"/secret", nil, token, "")
		assert.Contains(t, []int{http.StatusNotFound, http.StatusBadRequest}, w.Code, "id=%q", bad)
	}
}

func TestApps_RequireAdminSession(t *testing.T) {
	h := newAdminHarness(t, 1)
	for _, req := range []struct{ method, path string }{
		{http.MethodGet, "/apps"},
		{http.MethodPost, "/apps"},
		{http.MethodPut, "/apps/app_0123456789ab"},
		{http.MethodPost, "/apps/app_0123456789ab/secret"},
		{http.MethodDelete, "/apps/app_0123456789ab"},
	} {
		w := h.doJSON(req.method, req.path, map[string]string{"name": "x"}, "")
		assert.Equal(t, http.StatusUnauthorized, w.Code, "%s %s", req.method, req.path)
	}
}

// S2S 状态查询：只认已启用应用的密钥；没有任何应用时一律拒绝
func TestApps_StateQueryRequiresAppSecret(t *testing.T) {
	h := newAdminHarness(t, 1)

	expectUnauthorized := func(t *testing.T, secret string) {
		t.Helper()
		key, cerr := h.risk.GenerateSecondVerifyToken("app_aaaaaaaaaaaa")
		require.Nil(t, cerr)
		w := h.queryState(t, secret, key)
		assert.Equal(t, http.StatusUnauthorized, w.Code, "secret=%q", secret)
		assert.Equal(t, helper.ErrCodeAppUnauthorized, errorCode(t, w))
		assert.Equal(t, "Bearer", w.Header().Get("WWW-Authenticate"))
	}

	// 尚未创建任何应用：不再有"未配置则放行"
	require.Equal(t, 0, h.apps.Count())
	expectUnauthorized(t, "")
	expectUnauthorized(t, "sk_"+strings.Repeat("0", 48))

	token := h.login(t)
	app, secret := h.createApp(t, token, "订单系统")

	for _, bad := range []string{"", "garbage", secret[:len(secret)-1], secret + "0", strings.ToUpper(secret), token} {
		expectUnauthorized(t, bad)
	}

	// 正确密钥：鉴权通过，本应用的 second_key 一次性有效
	key, cerr := h.risk.GenerateSecondVerifyToken(app["id"].(string))
	require.Nil(t, cerr)
	w := h.queryState(t, secret, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, true, body["valid"])

	w = h.queryState(t, secret, key)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.EqualValues(t, helper.ErrCodeExpired, errorCode(t, w))

	// 调用后列表里能看到最近使用时间
	items := h.listApps(t, token)
	require.Len(t, items, 1)
	assert.NotNil(t, items[0].(map[string]interface{})["lastUsedAt"])

	// 停用后同一密钥被拒绝
	w = h.doJSON(http.MethodPut, "/apps/"+app["id"].(string), map[string]interface{}{"name": "订单系统", "enabled": false}, token)
	require.Equal(t, http.StatusOK, w.Code)
	expectUnauthorized(t, secret)
}

// 存储不可用时（库已关闭）应用管理返回 500，不会 panic
func TestApps_StorageFailureReturns500(t *testing.T) {
	h := newAdminHarness(t, 1)
	token := h.login(t)
	h.createApp(t, token, "first")

	// 鉴权本身也查库：直接调用服务层，绕开会话校验
	require.NoError(t, h.db.Close())
	_, _, cerr := h.apps.Create("second", nil)
	require.NotNil(t, cerr)
	assert.Equal(t, helper.ErrCodeInternalError, cerr.Code)
	_, cerr = h.apps.List()
	require.NotNil(t, cerr)
	assert.Equal(t, helper.ErrCodeInternalError, cerr.Code)
	_, ok := h.apps.Authenticate("sk_" + strings.Repeat("0", 48))
	assert.False(t, ok)
}

func TestApps_MaxApps(t *testing.T) {
	apps := services.NewAppService(storage.OpenTemp(t))
	for i := 0; i < 100; i++ {
		_, _, cerr := apps.Create(fmt.Sprintf("app-%d", i), nil)
		require.Nil(t, cerr)
	}
	_, _, cerr := apps.Create("one-too-many", nil)
	require.NotNil(t, cerr)
	assert.Equal(t, helper.ErrCodeInvalidParam, cerr.Code)
	assert.Equal(t, 100, apps.Count())
}
