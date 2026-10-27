package tests

import (
	"context"
	"net/http"
	"simple-captcha/helper"
	"simple-captcha/services"
	"simple-captcha/storage"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdmin_ChangePasswordValidation(t *testing.T) {
	h := newAdminHarness(t, 1)
	token := h.login(t)

	cases := []struct {
		name    string
		current string
		next    string
		status  int
		code    int
	}{
		{"当前密码错误", "wrong-password", "another-pass-123", http.StatusBadRequest, helper.ErrCodeWrongPassword},
		{"新密码过短", h.password, "short", http.StatusBadRequest, helper.ErrCodeInvalidParam},
		{"新密码全是空白", h.password, "            ", http.StatusBadRequest, helper.ErrCodeInvalidParam},
		{"新密码超过 72 字节", h.password, strings.Repeat("a", 73), http.StatusBadRequest, helper.ErrCodeInvalidParam},
		{"新旧密码相同", h.password, h.password, http.StatusBadRequest, helper.ErrCodeInvalidParam},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := h.doJSON(http.MethodPut, "/password", map[string]string{
				"currentPassword": tc.current, "newPassword": tc.next,
			}, token)
			assert.Equal(t, tc.status, w.Code, w.Body.String())
			assert.Equal(t, tc.code, errorCode(t, w))
		})
	}

	// 全部失败后会话仍然有效
	w := h.do(http.MethodGet, "/session", nil, token, "")
	require.Equal(t, http.StatusOK, w.Code)
	assertSessionKeys(t, decodeBody(t, w))

	// 未登录不能改密
	w = h.doJSON(http.MethodPut, "/password", map[string]string{
		"currentPassword": h.password, "newPassword": "another-pass-123",
	}, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAdmin_ChangePasswordRevokesAllSessions(t *testing.T) {
	h := newAdminHarness(t, 2)
	first := h.login(t)
	second := h.login(t)
	const next = "brand-new-pass-456"

	w := h.doJSON(http.MethodPut, "/password", map[string]string{
		"currentPassword": h.password, "newPassword": next,
	}, first)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decodeBody(t, w)
	fresh := body["token"].(string)
	assert.Equal(t, adminTestUser, body["username"])
	assert.NotEqual(t, first, fresh)
	old := h.password
	h.password = next

	// 改密前签发的会话（包括发起改密的那个）全部失效，新会话可直接管理素材
	for _, token := range []string{first, second} {
		w = h.do(http.MethodGet, "/session", nil, token, "")
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	}
	w = h.do(http.MethodGet, "/backgrounds", nil, fresh, "")
	assert.Equal(t, http.StatusOK, w.Code)

	// 旧密码不能再登录，新密码可以
	w = h.doJSON(http.MethodPost, "/login", map[string]string{
		"username": adminTestUser, "password": old, "captchaKey": h.freshCaptchaKey(t),
	}, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, helper.ErrCodeLoginFailed, errorCode(t, w))

	w = h.doJSON(http.MethodPost, "/login", map[string]string{
		"username": adminTestUser, "password": next, "captchaKey": h.freshCaptchaKey(t),
	}, "")
	require.Equal(t, http.StatusOK, w.Code)
}

// reset-admin 子命令在服务运行期间修改数据库：无需重启即生效，已有会话全部失效，服务回到未初始化状态，
// 新口令可以重新初始化，接入应用保留
func TestAdmin_ResetTakesEffectWithoutRestart(t *testing.T) {
	h := newAdminHarness(t, 2)
	token := h.login(t)
	app, secret := h.createApp(t, token, "订单系统")

	// 与子命令一样另开一个连接执行重置（子命令是独立进程，与运行中的服务共用库文件）
	cli, err := storage.Open(h.dataDir)
	require.NoError(t, err)
	code, err := services.ResetAdmin(context.Background(), cli)
	require.NoError(t, err)
	require.NoError(t, cli.Close())
	require.NotEmpty(t, code)

	w := h.do(http.MethodGet, "/backgrounds", nil, token, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code, "重置前的会话已失效")

	w = h.do(http.MethodGet, "/setup", nil, "", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, false, decodeBody(t, w)["initialized"])

	w = h.doJSON(http.MethodPost, "/login", map[string]string{
		"username": adminTestUser, "password": h.password, "captchaKey": h.freshCaptchaKey(t),
	}, "")
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, helper.ErrCodeNotInitialized, errorCode(t, w))

	// 启动时的旧口令已被替换，新口令可以完成初始化
	w = h.doJSON(http.MethodPost, "/setup", map[string]string{
		"setupCode": h.setupCode, "username": "ops", "password": adminTestPassword,
	}, "")
	assert.Equal(t, http.StatusForbidden, w.Code)
	h.setupCode = code
	token = h.initialize(t)

	// 接入应用不受影响，原密钥仍可鉴权
	items := h.listApps(t, token)
	require.Len(t, items, 1)
	assert.Equal(t, app["id"], items[0].(map[string]interface{})["id"])
	_, ok := h.apps.Authenticate(secret)
	assert.True(t, ok)
}
