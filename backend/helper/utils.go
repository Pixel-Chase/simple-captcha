package helper

import (
	"net"
	"simple-captcha/config"
	"simple-captcha/risk"
	"strings"

	"github.com/gin-gonic/gin"
)

// GetRealClientIP 获取客户端 IP。
//
// 代理头（默认 X-Forwarded-For / X-Real-IP，可通过 REMOTE_IP_HEADERS 扩展）
// 只有在直连来源命中 TrustedProxies 配置时才会被采信，这部分逻辑由 Gin 在
// router 初始化时通过 SetTrustedProxies + RemoteIPHeaders 统一处理。
// 旧实现无条件信任这些头，攻击者改头即可绕过限流与客户端绑定。
func GetRealClientIP(c *gin.Context) string {
	return c.ClientIP()
}

// IsTrustedProxyRequest 直连来源是否命中 TRUSTED_PROXIES（IP 或 CIDR）。
// 代理注入的 TLS / HTTP2 指纹头只有来自可信代理才采信，否则客户端自己就能伪造
func IsTrustedProxyRequest(c *gin.Context) bool {
	remote := net.ParseIP(strings.TrimSpace(c.RemoteIP()))
	if remote == nil {
		return false
	}
	for _, item := range config.LoadConfig().Security.TrustedProxies {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.Contains(item, "/") {
			if _, network, err := net.ParseCIDR(item); err == nil && network.Contains(remote) {
				return true
			}
			continue
		}
		if ip := net.ParseIP(item); ip != nil && ip.Equal(remote) {
			return true
		}
	}
	return false
}

// RequestSignalsFrom 从请求头提取供风控核验的信号；dpr 为出题请求携带的 devicePixelRatio，校验请求传 0。
// 浏览器网络栈自动填写的头直接读取；代理注入的 TLS / HTTP2 指纹头只在配置了头名且直连来源为可信代理时读取
func RequestSignalsFrom(c *gin.Context, dpr float64) *risk.RequestSignals {
	h := c.Request.Header
	signals := &risk.RequestSignals{
		UserAgent:       h.Get("User-Agent"),
		AcceptLanguage:  h.Get("Accept-Language"),
		SecFetchMode:    h.Get("Sec-Fetch-Mode"),
		SecFetchSite:    h.Get("Sec-Fetch-Site"),
		SecFetchDest:    h.Get("Sec-Fetch-Dest"),
		SecChUA:         h.Get("Sec-CH-UA"),
		SecChUAMobile:   h.Get("Sec-CH-UA-Mobile"),
		SecChUAPlatform: h.Get("Sec-CH-UA-Platform"),
		DPRParam:        dpr,
	}

	riskCfg := config.LoadConfig().Risk
	if (riskCfg.TLSFingerprintHeader != "" || riskCfg.HTTP2FingerprintHeader != "") && IsTrustedProxyRequest(c) {
		if name := riskCfg.TLSFingerprintHeader; name != "" {
			signals.TLSFingerprint = strings.TrimSpace(h.Get(name))
		}
		if name := riskCfg.HTTP2FingerprintHeader; name != "" {
			signals.HTTP2Fingerprint = strings.TrimSpace(h.Get(name))
		}
	}
	return signals
}

// ClientFingerprint 基于真实 IP 与 User-Agent 生成客户端哈希，
// 用于验证码会话绑定（出题与提交必须来自同一客户端）以及风控状态聚合。
func ClientFingerprint(c *gin.Context) string {
	return HashClient(GetRealClientIP(c), c.GetHeader("User-Agent"))
}
