// Package console 把管理后台前端（sdk/playground/console 的构建产物）内嵌进服务，与 API 同源提供。
//
// 构建产物由 sdk 中的 `pnpm build:console` 生成到 dist/，镜像构建时会自动生成；
// 本地未构建时 dist/ 里只有 .gitkeep，访问控制台返回一个说明如何构建的提示页。
package console

import (
	"bytes"
	"embed"
	"html"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

//go:embed all:dist
var embedded embed.FS

// BasePath 管理后台页面的挂载路径
const BasePath = "/console"

// apiPrefixPlaceholder 与前端约定的占位符（见 sdk/playground/console/index.html 与 api.ts）
const apiPrefixPlaceholder = "__CAPTCHA_API_PREFIX__"

// contentSecurityPolicy 控制台只加载同源脚本与样式：SDK 以 <style> 注入主题样式，需要放行内联样式；
// 验证码题图是 data: URI，背景图预览是 blob: URL
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; connect-src 'self'; font-src 'self' data:; object-src 'none'; " +
	"base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

const notBuiltPage = `<!doctype html>
<html lang="zh-CN"><head><meta charset="UTF-8"><title>管理后台尚未构建</title></head>
<body style="font-family:system-ui,sans-serif;max-width:640px;margin:64px auto;padding:0 16px;line-height:1.6">
<h1 style="font-size:20px">管理后台前端尚未构建</h1>
<p>当前二进制没有内嵌管理后台页面。镜像构建会自动生成；本地编译前先执行：</p>
<pre style="background:#f4f6fb;padding:12px;border-radius:8px">cd sdk &amp;&amp; pnpm build:console</pre>
<p>开发调试也可以直接使用演示站：<code>cd sdk &amp;&amp; pnpm start</code>，访问 http://localhost:8000/console/</p>
</body></html>`

// Assets 返回内嵌的构建产物
func Assets() fs.FS {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic(err) // dist 由 go:embed 保证存在
	}
	return sub
}

// Handler 管理后台的静态资源处理器
type Handler struct {
	assets fs.FS
	index  []byte // 已注入 API 前缀的 index.html；nil 表示未构建
}

// NewHandler 读取构建产物并把 API 前缀写入 index.html
func NewHandler(assets fs.FS, apiPrefix string) *Handler {
	index, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		return &Handler{assets: assets}
	}
	index = bytes.ReplaceAll(index, []byte(apiPrefixPlaceholder), []byte(html.EscapeString(apiPrefix)))
	return &Handler{assets: assets, index: index}
}

// Built 是否内嵌了构建产物
func (h *Handler) Built() bool {
	return h.index != nil
}

// Register 把管理后台挂到 BasePath：BasePath 重定向到 BasePath/，
// BasePath/ 返回 index.html，其余路径只提供构建产物中的文件。控制台用 hash 路由，不需要 SPA 回退
func Register(r gin.IRoutes, h *Handler) {
	redirect := func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, BasePath+"/")
	}
	r.GET(BasePath, redirect)
	r.HEAD(BasePath, redirect)
	r.GET(BasePath+"/*filepath", h.serve)
	r.HEAD(BasePath+"/*filepath", h.serve)
}

func (h *Handler) serve(c *gin.Context) {
	header := c.Writer.Header()
	header.Set("Content-Security-Policy", contentSecurityPolicy)
	header.Set("X-Robots-Tag", "noindex, nofollow")

	name := strings.TrimPrefix(c.Param("filepath"), "/")
	if name == "" || name == "index.html" {
		h.serveIndex(c)
		return
	}

	if !fs.ValidPath(name) {
		c.Status(http.StatusNotFound)
		return
	}
	file, err := h.assets.Open(name)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	content, seekable := file.(io.ReadSeeker)
	if err != nil || info.IsDir() || !seekable || strings.HasPrefix(info.Name(), ".") {
		c.Status(http.StatusNotFound)
		return
	}

	// assets/ 下的文件名带内容哈希，可以长期缓存；其余文件每次回源校验
	if strings.HasPrefix(name, "assets/") {
		header.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		header.Set("Cache-Control", "no-cache")
	}
	http.ServeContent(c.Writer, c.Request, info.Name(), time.Time{}, content)
}

func (h *Handler) serveIndex(c *gin.Context) {
	c.Header("Cache-Control", "no-cache")
	if h.index == nil {
		c.Data(http.StatusServiceUnavailable, "text/html; charset=utf-8", []byte(notBuiltPage))
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", h.index)
}
