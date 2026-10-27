package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"simple-captcha/config"
	"simple-captcha/helper"
	"simple-captcha/routes"
	"simple-captcha/services"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	adminTestUser = "admin"
	// adminTestPassword 首次登录强制改密后使用的密码；初始密码由服务随机生成，见 adminHarness.password
	adminTestPassword = "s3cret-demo-pass"
)

var (
	adminHarnessSeq  atomic.Int32
	savedNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*\.(jpg|png)$`)
)

// adminHarness 用临时素材目录、临时数据目录与独立的服务实例搭建管理路由，
// 每个实例使用不同的来源 IP，避免登录接口的 IP 限流在用例之间串扰
type adminHarness struct {
	router     *gin.Engine
	store      *services.BackgroundStore
	admin      *services.AdminService
	apps       *services.AppService
	risk       *services.RiskControlServiceImpl
	dir        string
	dataDir    string
	remoteAddr string
	// password 当前有效的管理员密码：起初是随机初始密码，login 完成强制改密后变为 adminTestPassword
	password string
}

func newAdminHarness(t *testing.T, imageCount int, mutate ...func(*config.AdminConfig)) *adminHarness {
	t.Helper()

	dir := t.TempDir()
	copySampleImages(t, dir, imageCount)

	store, err := services.NewBackgroundStore(dir)
	require.NoError(t, err)

	cfg := config.AdminConfig{
		Username:       adminTestUser,
		DataDir:        t.TempDir(),
		SessionTTL:     time.Hour,
		MaxUploadSize:  2 << 20,
		MaxImagePixels: 24_000_000,
	}
	for _, fn := range mutate {
		fn(&cfg)
	}

	// 先行生成凭据以拿到初始密码；NewAdminService 发现凭据已存在便不会重新生成
	initialPassword, err := services.EnsureAdminCredential(cfg.DataDir)
	require.NoError(t, err)
	require.NotEmpty(t, initialPassword)

	risk, ok := services.GetServiceManager().GetRiskControlService().(*services.RiskControlServiceImpl)
	require.True(t, ok)

	admin, err := services.NewAdminService(cfg, store, risk)
	require.NoError(t, err)
	apps, err := services.NewAppService(cfg.DataDir)
	require.NoError(t, err)
	// 删除应用会顺带清掉 Redis 里不过期的最近使用时间，避免每次跑测试都残留键
	t.Cleanup(func() {
		for _, app := range apps.List() {
			_ = apps.Delete(app.ID)
		}
	})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	routes.RegisterAdminRoutes(r.Group("/api/v1/admin"), admin, apps, risk)

	seq := adminHarnessSeq.Add(1)
	return &adminHarness{
		router:     r,
		store:      store,
		admin:      admin,
		apps:       apps,
		risk:       risk,
		dir:        dir,
		dataDir:    cfg.DataDir,
		remoteAddr: fmt.Sprintf("198.51.100.%d:4000", seq),
		password:   initialPassword,
	}
}

// copySampleImages 从仓库素材目录拷贝前 n 张图片到临时目录
func copySampleImages(t *testing.T, dir string, n int) {
	t.Helper()

	// controllers_test.go 的 init 会把工作目录切到项目根目录，这里按与服务相同的方式解析素材目录
	sampleDir := helper.ResolveImageDir("images")
	entries, err := os.ReadDir(sampleDir)
	require.NoError(t, err)

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && helper.IsSupportedImageFile(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	require.GreaterOrEqual(t, len(names), n, "仓库素材不足")

	for _, name := range names[:n] {
		data, err := os.ReadFile(filepath.Join(sampleDir, name))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o644))
	}
}

func (h *adminHarness) do(method, path string, body io.Reader, token, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/v1/admin"+path, body)
	req.RemoteAddr = h.remoteAddr
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, req)
	return w
}

func (h *adminHarness) doJSON(method, path string, payload interface{}, token string) *httptest.ResponseRecorder {
	data, _ := json.Marshal(payload)
	return h.do(method, path, bytes.NewReader(data), token, "application/json")
}

// freshCaptchaKey 直接通过风控服务签发一个二次验证令牌，等价于前端完成一次行为验证
func (h *adminHarness) freshCaptchaKey(t *testing.T) string {
	t.Helper()
	token, err := h.risk.GenerateSecondVerifyToken("admin-login-test")
	require.Nil(t, err)
	return token
}

// login 以当前密码登录；仍处于初始密码状态时顺带完成强制改密，返回可访问全部管理接口的会话令牌
func (h *adminHarness) login(t *testing.T) string {
	t.Helper()
	w := h.doJSON(http.MethodPost, "/login", map[string]string{
		"username":   adminTestUser,
		"password":   h.password,
		"captchaKey": h.freshCaptchaKey(t),
	}, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	body := decodeBody(t, w)
	token, _ := body["token"].(string)
	require.Len(t, token, 64)
	if body["mustChangePassword"] != true {
		return token
	}

	w = h.doJSON(http.MethodPut, "/password", map[string]string{
		"currentPassword": h.password,
		"newPassword":     adminTestPassword,
	}, token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	h.password = adminTestPassword

	body = decodeBody(t, w)
	require.Equal(t, false, body["mustChangePassword"])
	token, _ = body["token"].(string)
	require.Len(t, token, 64)
	return token
}

// upload 以 multipart 形式上传一个文件
func (h *adminHarness) upload(t *testing.T, token, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return h.do(http.MethodPost, "/backgrounds", &buf, token, writer.FormDataContentType())
}

func (h *adminHarness) fileCount(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir(h.dir)
	require.NoError(t, err)
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && helper.IsSupportedImageFile(entry.Name()) {
			count++
		}
	}
	return count
}

// encodePNG / encodeJPEG 生成指定尺寸的测试图片
func encodePNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func encodeJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: uint8(x), B: uint8(y), A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}))
	return buf.Bytes()
}

func errorCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	body := decodeBody(t, w)
	code, _ := body["code"].(float64)
	return int(code)
}

// ---------------------------------------------------------------------------
// 登录与会话
// ---------------------------------------------------------------------------

func TestAdmin_LoginFlow(t *testing.T) {
	h := newAdminHarness(t, 3)

	captchaKey := h.freshCaptchaKey(t)
	w := h.doJSON(http.MethodPost, "/login", map[string]string{
		"username": adminTestUser, "password": h.password, "captchaKey": captchaKey,
	}, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decodeBody(t, w)
	token := body["token"].(string)
	assert.Len(t, token, 64)
	assert.Equal(t, adminTestUser, body["username"])
	assert.Greater(t, body["expiresAt"].(float64), float64(time.Now().Unix()))
	assert.Equal(t, true, body["mustChangePassword"], "随机初始密码登录后必须改密")

	// 会话可用
	w = h.do(http.MethodGet, "/session", nil, token, "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, adminTestUser, decodeBody(t, w)["username"])

	// 同一个验证码 key 已被消费，不能再次用于登录
	w = h.doJSON(http.MethodPost, "/login", map[string]string{
		"username": adminTestUser, "password": h.password, "captchaKey": captchaKey,
	}, "")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, helper.ErrCodeCaptchaRequired, errorCode(t, w))

	// 退出后令牌立即失效
	w = h.do(http.MethodDelete, "/session", nil, token, "")
	assert.Equal(t, http.StatusNoContent, w.Code)
	w = h.do(http.MethodGet, "/session", nil, token, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, helper.ErrCodeUnauthorized, errorCode(t, w))
}

func TestAdmin_LoginRejectsBadCredentials(t *testing.T) {
	h := newAdminHarness(t, 2)

	cases := []struct {
		name     string
		username string
		password string
	}{
		{"密码错误", adminTestUser, "wrong-password"},
		{"用户名错误", "root", h.password},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := h.doJSON(http.MethodPost, "/login", map[string]string{
				"username": tc.username, "password": tc.password, "captchaKey": h.freshCaptchaKey(t),
			}, "")
			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Equal(t, helper.ErrCodeLoginFailed, errorCode(t, w))
			assert.Empty(t, decodeBody(t, w)["token"])
		})
	}

	// 密码错误也会消费掉验证码：下一次尝试必须重新完成行为验证
	captchaKey := h.freshCaptchaKey(t)
	w := h.doJSON(http.MethodPost, "/login", map[string]string{
		"username": adminTestUser, "password": "wrong", "captchaKey": captchaKey,
	}, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	w = h.doJSON(http.MethodPost, "/login", map[string]string{
		"username": adminTestUser, "password": h.password, "captchaKey": captchaKey,
	}, "")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, helper.ErrCodeCaptchaRequired, errorCode(t, w))

	// 缺少验证码或验证码无效
	w = h.doJSON(http.MethodPost, "/login", map[string]string{
		"username": adminTestUser, "password": h.password,
	}, "")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, helper.ErrCodeInvalidParam, errorCode(t, w))

	w = h.doJSON(http.MethodPost, "/login", map[string]string{
		"username": adminTestUser, "password": h.password, "captchaKey": "not-a-real-key",
	}, "")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, helper.ErrCodeCaptchaRequired, errorCode(t, w))
}

func TestAdmin_RequiresBearerToken(t *testing.T) {
	h := newAdminHarness(t, 2)

	for _, token := range []string{"", "garbage", "deadbeef", h.freshCaptchaKey(t)} {
		w := h.do(http.MethodGet, "/backgrounds", nil, token, "")
		assert.Equal(t, http.StatusUnauthorized, w.Code, "token=%q", token)
		assert.Equal(t, helper.ErrCodeUnauthorized, errorCode(t, w))
		assert.Equal(t, "Bearer", w.Header().Get("WWW-Authenticate"))
	}

	// 其余受保护接口同样拒绝
	w := h.upload(t, "", "x.png", encodePNG(t, 960, 640))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	w = h.do(http.MethodDelete, "/backgrounds/1.jpeg", nil, "", "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ---------------------------------------------------------------------------
// 背景图管理
// ---------------------------------------------------------------------------

func TestAdmin_ListBackgrounds(t *testing.T) {
	h := newAdminHarness(t, 3)
	token := h.login(t)

	w := h.do(http.MethodGet, "/backgrounds", nil, token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "no-store, no-cache, must-revalidate", w.Header().Get("Cache-Control"))

	body := decodeBody(t, w)
	assert.Equal(t, float64(3), body["total"])
	items := body["items"].([]interface{})
	require.Len(t, items, 3)
	first := items[0].(map[string]interface{})
	for _, field := range []string{"name", "size", "width", "height", "format", "modifiedAt"} {
		assert.Contains(t, first, field)
	}
	// 宽高与文件实际尺寸一致；从文件读取期望值，不绑定仓库素材的规格
	data, err := os.ReadFile(filepath.Join(h.dir, first["name"].(string)))
	require.NoError(t, err)
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, float64(cfg.Width), first["width"])
	assert.Equal(t, float64(cfg.Height), first["height"])

	constraints := body["constraints"].(map[string]interface{})
	assert.Equal(t, float64(900), constraints["minWidth"])
	assert.Equal(t, float64(600), constraints["minHeight"])
	assert.Equal(t, float64(1800), constraints["recommendedWidth"])
	assert.Equal(t, float64(1200), constraints["recommendedHeight"])
	assert.Equal(t, float64(2<<20), constraints["maxUploadSize"])
	assert.ElementsMatch(t, []interface{}{"jpeg", "png"}, constraints["allowedFormats"])
}

func TestAdmin_UploadHotReloadsBackgrounds(t *testing.T) {
	h := newAdminHarness(t, 2)
	token := h.login(t)

	reloads := 0
	h.store.OnReload(func() { reloads++ })
	versionBefore := h.store.Version()
	require.Equal(t, 2, h.store.Count())

	// PNG：文件名带路径与中文，落盘名应被清洗并带随机后缀
	w := h.upload(t, token, "../相册/My Photo (1).PNG", encodePNG(t, 960, 640))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	saved := decodeBody(t, w)
	name := saved["name"].(string)
	assert.Regexp(t, savedNamePattern, name)
	assert.True(t, strings.HasPrefix(name, "My-Photo-1-"), "name=%s", name)
	assert.Equal(t, "png", saved["format"])
	assert.Equal(t, float64(960), saved["width"])
	assert.Equal(t, float64(640), saved["height"])
	assert.FileExists(t, filepath.Join(h.dir, name))

	// 仓库已热重载：数量 +1、版本递增、回调被触发
	assert.Equal(t, 3, h.store.Count())
	assert.Equal(t, versionBefore+1, h.store.Version())
	assert.Equal(t, 1, reloads)

	// JPEG：扩展名由真实格式决定，而非上传方声明
	w = h.upload(t, token, "fake-name.png", encodeJPEG(t, 900, 660))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	saved = decodeBody(t, w)
	assert.Equal(t, "jpeg", saved["format"])
	assert.Regexp(t, `\.jpg$`, saved["name"])
	assert.Equal(t, 4, h.store.Count())

	// 列表按修改时间倒序，最新上传排在最前
	w = h.do(http.MethodGet, "/backgrounds", nil, token, "")
	require.Equal(t, http.StatusOK, w.Code)
	items := decodeBody(t, w)["items"].([]interface{})
	require.Len(t, items, 4)
	assert.Equal(t, saved["name"], items[0].(map[string]interface{})["name"])

	// 没有残留的临时文件
	entries, err := os.ReadDir(h.dir)
	require.NoError(t, err)
	for _, entry := range entries {
		assert.NotContains(t, entry.Name(), ".upload-")
	}
}

func TestAdmin_UploadRejectsInvalidFiles(t *testing.T) {
	h := newAdminHarness(t, 2, func(cfg *config.AdminConfig) {
		cfg.MaxUploadSize = 64 << 10
	})
	token := h.login(t)
	versionBefore := h.store.Version()

	cases := []struct {
		name     string
		filename string
		content  []byte
		status   int
		code     int
	}{
		{"尺寸过小", "small.png", encodePNG(t, 100, 100), http.StatusBadRequest, helper.ErrCodeInvalidImage},
		{"宽度不足", "narrow.png", encodePNG(t, 200, 400), http.StatusBadRequest, helper.ErrCodeInvalidImage},
		{"低于 3 倍率下限", "lowres.png", encodePNG(t, 600, 400), http.StatusBadRequest, helper.ErrCodeInvalidImage},
		{"非图片内容", "notes.png", []byte("hello, this is not an image"), http.StatusBadRequest, helper.ErrCodeInvalidImage},
		{"空文件", "empty.png", []byte{}, http.StatusBadRequest, helper.ErrCodeInvalidImage},
		{"体积超限", "huge.png", bytes.Repeat([]byte{0x89}, 100<<10), http.StatusRequestEntityTooLarge, helper.ErrCodeImageTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := h.upload(t, token, tc.filename, tc.content)
			assert.Equal(t, tc.status, w.Code, w.Body.String())
			assert.Equal(t, tc.code, errorCode(t, w))
		})
	}

	// 缺少 file 字段
	w := h.do(http.MethodPost, "/backgrounds", bytes.NewReader([]byte("{}")), token, "application/json")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, helper.ErrCodeInvalidParam, errorCode(t, w))

	// 全部被拒：磁盘与仓库均未变化
	assert.Equal(t, 2, h.fileCount(t))
	assert.Equal(t, 2, h.store.Count())
	assert.Equal(t, versionBefore, h.store.Version())
}

func TestAdmin_DeleteBackground(t *testing.T) {
	h := newAdminHarness(t, 2)
	token := h.login(t)

	w := h.upload(t, token, "extra.png", encodePNG(t, 960, 640))
	require.Equal(t, http.StatusOK, w.Code)
	name := decodeBody(t, w)["name"].(string)
	require.Equal(t, 3, h.store.Count())

	// 预览可读
	w = h.do(http.MethodGet, "/backgrounds/"+name, nil, token, "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/png", w.Header().Get("Content-Type"))
	assert.Equal(t, "private, max-age=300", w.Header().Get("Cache-Control"))
	assert.Empty(t, w.Header().Get("Pragma"))

	// 删除后磁盘与仓库同步
	w = h.do(http.MethodDelete, "/backgrounds/"+name, nil, token, "")
	assert.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	assert.NoFileExists(t, filepath.Join(h.dir, name))
	assert.Equal(t, 2, h.store.Count())

	// 再删一次 / 预览一个不存在的文件
	w = h.do(http.MethodDelete, "/backgrounds/"+name, nil, token, "")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, helper.ErrCodeBackgroundNotFound, errorCode(t, w))
	w = h.do(http.MethodGet, "/backgrounds/"+name, nil, token, "")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdmin_RejectsUnsafeNames(t *testing.T) {
	h := newAdminHarness(t, 2)

	// 在目录之外放一个诱饵文件，确认任何路径成分都无法触达它
	decoy := filepath.Join(filepath.Dir(h.dir), "decoy.png")
	require.NoError(t, os.WriteFile(decoy, encodePNG(t, 960, 640), 0o644))
	t.Cleanup(func() { _ = os.Remove(decoy) })

	unsafe := []string{
		"../decoy.png",
		"..%2Fdecoy.png",
		"/etc/passwd",
		".hidden.png",
		"a..png",
		"name with space.png",
		"script.png.sh",
		"noext",
		"",
	}
	for _, name := range unsafe {
		err := h.admin.DeleteBackground(name)
		require.NotNil(t, err, "name=%q", name)
		assert.Equal(t, helper.ErrCodeInvalidParam, err.Code, "name=%q", name)

		_, err = h.admin.ResolveBackgroundPath(name)
		require.NotNil(t, err, "name=%q", name)
		assert.Equal(t, helper.ErrCodeInvalidParam, err.Code, "name=%q", name)
	}
	assert.FileExists(t, decoy)
	assert.Equal(t, 2, h.fileCount(t))
}

func TestAdmin_RefusesDeletingLastBackground(t *testing.T) {
	h := newAdminHarness(t, 1)
	token := h.login(t)

	w := h.do(http.MethodGet, "/backgrounds", nil, token, "")
	require.Equal(t, http.StatusOK, w.Code)
	items := decodeBody(t, w)["items"].([]interface{})
	require.Len(t, items, 1)
	name := items[0].(map[string]interface{})["name"].(string)

	w = h.do(http.MethodDelete, "/backgrounds/"+name, nil, token, "")
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, helper.ErrCodeLastBackground, errorCode(t, w))
	assert.FileExists(t, filepath.Join(h.dir, name))
	assert.Equal(t, 1, h.store.Count())
}

// ---------------------------------------------------------------------------
// 背景图仓库
// ---------------------------------------------------------------------------

func TestBackgroundStore_ReloadKeepsOldImagesOnFailure(t *testing.T) {
	dir := t.TempDir()
	copySampleImages(t, dir, 2)

	store, err := services.NewBackgroundStore(dir)
	require.NoError(t, err)
	require.Equal(t, 2, store.Count())
	versionBefore := store.Version()

	// 目录被清空：重载失败，但仍保留原有素材可供出题
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, entry := range entries {
		require.NoError(t, os.Remove(filepath.Join(dir, entry.Name())))
	}
	err = store.Reload()
	require.Error(t, err)
	assert.Equal(t, 2, store.Count())
	assert.Equal(t, versionBefore, store.Version())
	// 每张素材按 110% / 125% 两个放大档各一份
	assert.Len(t, store.Get(600, 400), 4)
}

// imageSizes 列出图片集合的尺寸，便于与期望集合比较
func imageSizes(images []image.Image) [][2]int {
	sizes := make([][2]int, 0, len(images))
	for _, img := range images {
		sizes = append(sizes, [2]int{img.Bounds().Dx(), img.Bounds().Dy()})
	}
	return sizes
}

func TestBackgroundStore_GetCoversRequestedSize(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "square.png"), encodePNG(t, 420, 420), 0o644))

	store, err := services.NewBackgroundStore(dir)
	require.NoError(t, err)

	// 420x420 方图（与出图比例不同）：每个出图尺寸按 110% / 125% 两档各缩放到恰好覆盖，受限的一边与之相等
	cases := []struct {
		w, h int
		want [][2]int
	}{
		{900, 600, [][2]int{{990, 990}, {1125, 1125}}}, // 3x 主图
		{600, 400, [][2]int{{660, 660}, {750, 750}}},   // 2x 主图
		{256, 256, [][2]int{{282, 282}, {320, 320}}},   // 2x 旋转
	}
	for _, c := range cases {
		assert.ElementsMatch(t, c.want, imageSizes(store.Get(c.w, c.h)), "%dx%d", c.w, c.h)
	}
}

func TestBackgroundStore_DownscalesLargeOriginals(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "large.png"), encodePNG(t, 1800, 1200), 0o644))

	store, err := services.NewBackgroundStore(dir)
	require.NoError(t, err)

	// 3:2 高清原图整张缩小，保留完整构图，而不是从原图里随机裁一小块
	assert.ElementsMatch(t, [][2]int{{660, 440}, {750, 500}}, imageSizes(store.Get(600, 400)))
	assert.ElementsMatch(t, [][2]int{{990, 660}, {1125, 750}}, imageSizes(store.Get(900, 600)))
}

// 题型服务在素材热重载后仍能正常出题（生成器实例被重置并用新素材重建）
func TestCaptchaService_GeneratesAfterBackgroundReload(t *testing.T) {
	h := newAdminHarness(t, 2)
	token := h.login(t)

	svc := services.NewRotateCaptchaService(h.store)
	req := &services.ChallengeRequest{Type: "rotate", Mode: "light", Scale: 1, Difficulty: 3}

	first, cerr := svc.GenerateCaptcha(req)
	require.Nil(t, cerr)
	require.NotEmpty(t, first.Image)

	w := h.upload(t, token, "fresh.png", encodePNG(t, 960, 640))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	second, cerr := svc.GenerateCaptcha(req)
	require.Nil(t, cerr)
	require.NotEmpty(t, second.Image)
	assert.NotEqual(t, first.Key, second.Key)
}
