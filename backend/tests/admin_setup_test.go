package tests

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"simple-captcha/config"
	"simple-captcha/helper"
	"simple-captcha/services"
	"simple-captcha/storage"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var setupCodeFormat = regexp.MustCompile(`^[A-HJ-NP-Z2-9]{4}-[A-HJ-NP-Z2-9]{4}-[A-HJ-NP-Z2-9]{4}$`)

func (h *adminHarness) setupStatus(t *testing.T) bool {
	t.Helper()
	w := h.do(http.MethodGet, "/setup", nil, "", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	initialized, ok := decodeBody(t, w)["initialized"].(bool)
	require.True(t, ok)
	return initialized
}

func TestSetup_UninitializedState(t *testing.T) {
	h := newAdminHarness(t, 1)

	assert.Regexp(t, setupCodeFormat, h.setupCode)
	assert.False(t, h.setupStatus(t))

	// 初始化之前登录返回 2012，且不消费验证码
	captchaKey := h.freshCaptchaKey(t)
	w := h.doJSON(http.MethodPost, "/login", map[string]string{
		"username": adminTestUser, "password": adminTestPassword, "captchaKey": captchaKey,
	}, "")
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Equal(t, helper.ErrCodeNotInitialized, errorCode(t, w))
	assert.True(t, h.risk.ConsumeSecondVerifyToken(captchaKey, services.ConsoleAppID), "未初始化时不应消费验证码")

	// 数据库里只有口令哈希，没有明文
	var hash string
	require.NoError(t, h.db.QueryRowContext(context.Background(), `SELECT code_hash FROM admin_setup WHERE id = 1`).Scan(&hash))
	assert.Len(t, hash, 64)
	assertFileLacks(t, h.dataDir, h.setupCode)
	assertFileLacks(t, h.dataDir, strings.ReplaceAll(h.setupCode, "-", ""))
}

func TestSetup_Validation(t *testing.T) {
	h := newAdminHarness(t, 1)

	cases := []struct {
		name     string
		code     string
		username string
		password string
		status   int
		errCode  int
	}{
		{"口令错误", "AAAA-BBBB-CCCC", adminTestUser, adminTestPassword, http.StatusForbidden, helper.ErrCodeSetupCodeInvalid},
		{"口令为空白", "   ", adminTestUser, adminTestPassword, http.StatusForbidden, helper.ErrCodeSetupCodeInvalid},
		{"缺少口令", "", adminTestUser, adminTestPassword, http.StatusBadRequest, helper.ErrCodeInvalidParam},
		{"用户名为空白", h.setupCode, "   ", adminTestPassword, http.StatusBadRequest, helper.ErrCodeInvalidParam},
		{"用户名过长", h.setupCode, strings.Repeat("名", 65), adminTestPassword, http.StatusBadRequest, helper.ErrCodeInvalidParam},
		{"用户名含控制字符", h.setupCode, "ad\x00min", adminTestPassword, http.StatusBadRequest, helper.ErrCodeInvalidParam},
		{"密码过短", h.setupCode, adminTestUser, "short", http.StatusBadRequest, helper.ErrCodeInvalidParam},
		{"密码超过 72 字节", h.setupCode, adminTestUser, strings.Repeat("a", 73), http.StatusBadRequest, helper.ErrCodeInvalidParam},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := h.doJSON(http.MethodPost, "/setup", map[string]string{
				"setupCode": tc.code, "username": tc.username, "password": tc.password,
			}, "")
			assert.Equal(t, tc.status, w.Code, w.Body.String())
			assert.Equal(t, tc.errCode, errorCode(t, w))
		})
	}

	// 全部失败后仍未初始化，原口令仍然有效
	assert.False(t, h.setupStatus(t))
	h.initialize(t)
}

func TestSetup_SuccessIssuesSessionAndLocks(t *testing.T) {
	h := newAdminHarness(t, 1)

	// 口令忽略大小写、空白与连字符；用户名去掉首尾空白
	loose := " " + strings.ToLower(strings.ReplaceAll(h.setupCode, "-", " ")) + " "
	w := h.doJSON(http.MethodPost, "/setup", map[string]string{
		"setupCode": loose, "username": "  运维 ops  ", "password": adminTestPassword,
	}, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decodeBody(t, w)
	token := body["token"].(string)
	assert.Len(t, token, 64)
	assert.Equal(t, "运维 ops", body["username"])
	assert.Greater(t, body["expiresAt"].(float64), float64(time.Now().Unix()))

	// 返回的会话可直接访问管理接口
	w = h.do(http.MethodGet, "/apps", nil, token, "")
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.True(t, h.setupStatus(t))

	// 口令已作废，再次初始化返回 409 / 2013
	w = h.doJSON(http.MethodPost, "/setup", map[string]string{
		"setupCode": h.setupCode, "username": "other", "password": adminTestPassword,
	}, "")
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, helper.ErrCodeAlreadyInitialized, errorCode(t, w))
	var rows int
	require.NoError(t, h.db.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM admin_setup`).Scan(&rows))
	assert.Zero(t, rows)

	// 用初始化时设置的用户名与密码登录
	w = h.doJSON(http.MethodPost, "/login", map[string]string{
		"username": "运维 ops", "password": adminTestPassword, "captchaKey": h.freshCaptchaKey(t),
	}, "")
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestSetup_ConcurrentOnlyOneSucceeds(t *testing.T) {
	h := newAdminHarness(t, 1)

	const workers = 6
	codes := make([]int, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := h.doJSON(http.MethodPost, "/setup", map[string]string{
				"setupCode": h.setupCode, "username": adminTestUser, "password": adminTestPassword,
			}, "")
			codes[i] = w.Code
		}(i)
	}
	wg.Wait()

	succeeded := 0
	for _, code := range codes {
		if code == http.StatusOK {
			succeeded++
		} else {
			assert.Contains(t, []int{http.StatusConflict, http.StatusForbidden}, code)
		}
	}
	assert.Equal(t, 1, succeeded, "codes=%v", codes)
}

func TestSetup_RestartRotatesCodeUntilInitialized(t *testing.T) {
	h := newAdminHarness(t, 1)
	ctx := context.Background()

	// 未初始化时重启（再次 PrepareSetup）换新口令，旧口令失效
	next, err := services.PrepareSetup(ctx, h.db)
	require.NoError(t, err)
	assert.Regexp(t, setupCodeFormat, next)
	assert.NotEqual(t, h.setupCode, next)

	w := h.doJSON(http.MethodPost, "/setup", map[string]string{
		"setupCode": h.setupCode, "username": adminTestUser, "password": adminTestPassword,
	}, "")
	assert.Equal(t, http.StatusForbidden, w.Code)
	h.setupCode = next
	h.initialize(t)

	// 已初始化后重启不再生成口令
	again, err := services.PrepareSetup(ctx, h.db)
	require.NoError(t, err)
	assert.Empty(t, again)
}

// 关闭并重新打开存储（模拟重启）：账号、会话、应用与最近使用时间都保留
func TestSetup_StatePersistsAcrossRestart(t *testing.T) {
	h := newAdminHarness(t, 1)
	token := h.login(t)
	app, secret := h.createApp(t, token, "订单系统")
	_, ok := h.apps.Authenticate(secret)
	require.True(t, ok)
	require.NoError(t, h.db.Close())

	db, err := storage.Open(h.dataDir)
	require.NoError(t, err)
	defer db.Close()
	code, err := services.PrepareSetup(context.Background(), db)
	require.NoError(t, err)
	assert.Empty(t, code, "已初始化，重启后不再生成口令")

	cfg := config.AdminConfig{DataDir: h.dataDir, SessionTTL: time.Hour}
	admin := services.NewAdminService(cfg, db, h.store, h.risk)
	session, ok := admin.Authenticate(token)
	require.True(t, ok, "会话在有效期内仍可用")
	assert.Equal(t, adminTestUser, session.Username)

	apps := services.NewAppService(db)
	items, cerr := apps.List()
	require.Nil(t, cerr)
	require.Len(t, items, 2, "内置系统应用 + 新建的应用")
	assert.True(t, items[0].IsSystem)
	assert.Equal(t, app["id"], items[1].ID)
	require.NotNil(t, items[1].LastUsedAt, "最近使用时间已落库")
	_, ok = apps.Authenticate(secret)
	assert.True(t, ok)
}

// assertFileLacks 数据目录下的数据库文件（含 WAL）都不包含给定明文
func assertFileLacks(t *testing.T, dir, needle string) {
	t.Helper()
	for _, name := range []string{storage.FileName, storage.FileName + "-wal"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if os.IsNotExist(err) {
			continue
		}
		require.NoError(t, err)
		assert.NotContains(t, string(data), needle, name)
	}
}
