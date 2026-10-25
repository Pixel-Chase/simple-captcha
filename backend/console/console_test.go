package console

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testIndex = `<!doctype html><html><head>` +
	`<meta name="captcha-api-prefix" content="__CAPTCHA_API_PREFIX__" />` +
	`<script type="module" src="./assets/index-abc123.js"></script></head><body></body></html>`

func builtAssets() fs.FS {
	return fstest.MapFS{
		"index.html":              {Data: []byte(testIndex)},
		"assets/index-abc123.js":  {Data: []byte("console.log('console')")},
		"assets/index-abc123.css": {Data: []byte("body{}")},
		"favicon.svg":             {Data: []byte("<svg/>")},
		".gitkeep":                {Data: nil},
	}
}

func newRouter(assets fs.FS, apiPrefix string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Register(r, "/console", NewHandler(assets, apiPrefix))
	return r
}

func get(r http.Handler, method, target string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, target, nil))
	return w
}

func TestConsole_RedirectsToTrailingSlash(t *testing.T) {
	r := newRouter(builtAssets(), "/api/v1")
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		w := get(r, method, "/console")
		assert.Equal(t, http.StatusMovedPermanently, w.Code, method)
		assert.Equal(t, "/console/", w.Header().Get("Location"))
	}
}

func TestConsole_IndexInjectsAPIPrefix(t *testing.T) {
	r := newRouter(builtAssets(), "/captcha/api")

	for _, target := range []string{"/console/", "/console/index.html"} {
		w := get(r, http.MethodGet, target)
		require.Equal(t, http.StatusOK, w.Code, target)
		body := w.Body.String()
		assert.Contains(t, body, `content="/captcha/api"`)
		assert.NotContains(t, body, apiPrefixPlaceholder)
		assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
		assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
		assert.Contains(t, w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'")
		assert.Equal(t, "noindex, nofollow", w.Header().Get("X-Robots-Tag"))
	}

	// API 挂在根路径时前缀为空串；前缀里的特殊字符按 HTML 属性转义
	w := get(newRouter(builtAssets(), ""), http.MethodGet, "/console/")
	assert.Contains(t, w.Body.String(), `name="captcha-api-prefix" content=""`)
	w = get(newRouter(builtAssets(), `/x"><script>`), http.MethodGet, "/console/")
	assert.NotContains(t, w.Body.String(), `"><script>`)
	assert.Contains(t, w.Body.String(), `content="/x&#34;&gt;&lt;script&gt;"`)
}

func TestConsole_ServesHashedAssetsWithLongCache(t *testing.T) {
	r := newRouter(builtAssets(), "/api/v1")

	w := get(r, http.MethodGet, "/console/assets/index-abc123.js")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "console.log('console')", w.Body.String())
	assert.Contains(t, w.Header().Get("Content-Type"), "javascript")
	assert.Equal(t, "public, max-age=31536000, immutable", w.Header().Get("Cache-Control"))
	assert.NotEmpty(t, w.Header().Get("Content-Security-Policy"))

	w = get(r, http.MethodGet, "/console/assets/index-abc123.css")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/css")

	// 不带哈希的文件每次回源校验
	w = get(r, http.MethodGet, "/console/favicon.svg")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))

	w = get(r, http.MethodHead, "/console/assets/index-abc123.js")
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestConsole_OnlyServesBuiltFiles(t *testing.T) {
	r := newRouter(builtAssets(), "/api/v1")

	for _, target := range []string{
		"/console/missing.js",
		"/console/assets",
		"/console/assets/",
		"/console/.gitkeep",
		"/console/apps", // hash 路由，不做 SPA 回退
		"/console/../go.mod",
		"/console/assets/..%2f..%2fconsole.go",
		"/console/%2e%2e/console.go",
	} {
		w := get(r, http.MethodGet, target)
		assert.Equal(t, http.StatusNotFound, w.Code, target)
		assert.False(t, strings.Contains(w.Body.String(), "package console"), target)
	}
}

func TestConsole_NotBuiltShowsHint(t *testing.T) {
	r := newRouter(fstest.MapFS{".gitkeep": {Data: nil}}, "/api/v1")
	assert.False(t, NewHandler(fstest.MapFS{}, "/api/v1").Built())

	w := get(r, http.MethodGet, "/console/")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "pnpm build:console")
	assert.NotEmpty(t, w.Header().Get("Content-Security-Policy"))
}

func TestConsole_EmbeddedAssetsAlwaysPresent(t *testing.T) {
	// 未构建时 dist 里也有 .gitkeep，go:embed 不会因为目录为空而编译失败
	_, err := fs.Stat(Assets(), ".gitkeep")
	assert.NoError(t, err)
}
