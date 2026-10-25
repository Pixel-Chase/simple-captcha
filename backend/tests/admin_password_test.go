package tests

import (
	"net/http"
	"os"
	"path/filepath"
	"simple-captcha/helper"
	"simple-captcha/services"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loginInitial 用随机初始密码登录，返回仍处于"必须改密"状态的会话令牌
func (h *adminHarness) loginInitial(t *testing.T) string {
	t.Helper()
	w := h.doJSON(http.MethodPost, "/login", map[string]string{
		"username": adminTestUser, "password": h.password, "captchaKey": h.freshCaptchaKey(t),
	}, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decodeBody(t, w)
	require.Equal(t, true, body["mustChangePassword"])
	return body["token"].(string)
}

func TestAdmin_InitialCredentialIsRandomAndHashed(t *testing.T) {
	dir := t.TempDir()

	first, err := services.EnsureAdminCredential(dir)
	require.NoError(t, err)
	assert.Len(t, first, 20)
	assert.NotContains(t, first, "0", "初始密码不含易混字符")
	assert.NotContains(t, first, "O")

	// 文件只保存哈希，权限仅属主可读写
	path := filepath.Join(dir, "admin.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), first)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	// 已存在时不再生成，也取不回明文
	again, err := services.EnsureAdminCredential(dir)
	require.NoError(t, err)
	assert.Empty(t, again)

	// 另一个数据目录得到不同的密码
	other, err := services.EnsureAdminCredential(t.TempDir())
	require.NoError(t, err)
	assert.NotEqual(t, first, other)
}

func TestAdmin_CorruptCredentialFailsFast(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "admin.json"), []byte(`{"passwordHash":"plain"}`), 0o600))

	_, err := services.EnsureAdminCredential(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reset-admin-password")
}

func TestAdmin_MustChangePasswordGatesConsole(t *testing.T) {
	h := newAdminHarness(t, 2)
	token := h.loginInitial(t)

	// 会话查询可用，并提示需要改密
	w := h.do(http.MethodGet, "/session", nil, token, "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, true, decodeBody(t, w)["mustChangePassword"])

	// 背景图与应用管理一律 403
	for _, req := range []struct{ method, path string }{
		{http.MethodGet, "/backgrounds"},
		{http.MethodDelete, "/backgrounds/1.jpeg"},
		{http.MethodGet, "/apps"},
		{http.MethodPost, "/apps"},
	} {
		w := h.doJSON(req.method, req.path, map[string]string{"name": "x"}, token)
		assert.Equal(t, http.StatusForbidden, w.Code, "%s %s", req.method, req.path)
		assert.Equal(t, helper.ErrCodePasswordChangeRequired, errorCode(t, w))
	}

	// 注销仍然可用
	w = h.do(http.MethodDelete, "/session", nil, token, "")
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestAdmin_ChangePasswordValidation(t *testing.T) {
	h := newAdminHarness(t, 1)
	token := h.loginInitial(t)

	cases := []struct {
		name    string
		current string
		next    string
		status  int
		code    int
	}{
		{"当前密码错误", "wrong-password", adminTestPassword, http.StatusBadRequest, helper.ErrCodeWrongPassword},
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

	// 全部失败后会话仍然有效，状态不变
	w := h.do(http.MethodGet, "/session", nil, token, "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, true, decodeBody(t, w)["mustChangePassword"])

	// 未登录不能改密
	w = h.doJSON(http.MethodPut, "/password", map[string]string{
		"currentPassword": h.password, "newPassword": adminTestPassword,
	}, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAdmin_ChangePasswordRevokesAllSessions(t *testing.T) {
	h := newAdminHarness(t, 2)
	initial := h.password
	first := h.loginInitial(t)
	second := h.loginInitial(t)

	w := h.doJSON(http.MethodPut, "/password", map[string]string{
		"currentPassword": initial, "newPassword": adminTestPassword,
	}, first)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decodeBody(t, w)
	fresh := body["token"].(string)
	assert.Equal(t, false, body["mustChangePassword"])
	assert.NotEqual(t, first, fresh)
	h.password = adminTestPassword

	// 改密前签发的会话（包括发起改密的那个）全部失效，新会话可直接管理素材
	for _, token := range []string{first, second} {
		w = h.do(http.MethodGet, "/session", nil, token, "")
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	}
	w = h.do(http.MethodGet, "/backgrounds", nil, fresh, "")
	assert.Equal(t, http.StatusOK, w.Code)

	// 旧密码不能再登录，新密码登录后不再要求改密
	w = h.doJSON(http.MethodPost, "/login", map[string]string{
		"username": adminTestUser, "password": initial, "captchaKey": h.freshCaptchaKey(t),
	}, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, helper.ErrCodeLoginFailed, errorCode(t, w))

	w = h.doJSON(http.MethodPost, "/login", map[string]string{
		"username": adminTestUser, "password": adminTestPassword, "captchaKey": h.freshCaptchaKey(t),
	}, "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, false, decodeBody(t, w)["mustChangePassword"])
}

// reset-admin-password 子命令在服务运行期间替换凭据文件：无需重启即生效，已有会话全部失效
func TestAdmin_ResetPasswordTakesEffectWithoutRestart(t *testing.T) {
	h := newAdminHarness(t, 2)
	token := h.login(t)

	w := h.do(http.MethodGet, "/backgrounds", nil, token, "")
	require.Equal(t, http.StatusOK, w.Code)

	reset, err := services.ResetAdminPassword(h.dataDir)
	require.NoError(t, err)

	w = h.do(http.MethodGet, "/backgrounds", nil, token, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	w = h.doJSON(http.MethodPost, "/login", map[string]string{
		"username": adminTestUser, "password": adminTestPassword, "captchaKey": h.freshCaptchaKey(t),
	}, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code, "重置前的密码已失效")

	h.password = reset
	token = h.loginInitial(t)
	w = h.do(http.MethodGet, "/backgrounds", nil, token, "")
	assert.Equal(t, http.StatusForbidden, w.Code, "重置后的密码同样要求先改密")
}
