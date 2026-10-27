package tests

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"simple-captcha/config"
	"simple-captcha/helper"
	"simple-captcha/risk"
	"simple-captcha/services"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testJA4Header   = "X-JA4-Fingerprint"
	testDeniedFP    = "t13d_denied_test"
	testFirefoxFP   = "t13d_firefox_test"
	testChromeFP    = "t13d_chrome_test"
	testUnknownFP   = "t13d_unknown_test"
	testListPayload = `{"browser":{"t13d_firefox_test":"firefox","t13d_chrome_test":"chrome"},"deny":["t13d_denied_test"]}`
)

// withTLSFingerprintHeader 临时启用指纹头读取；配置是进程内单例，测试结束后还原
func withTLSFingerprintHeader(t *testing.T, header string) {
	cfg := config.LoadConfig()
	prev := cfg.Risk.TLSFingerprintHeader
	cfg.Risk.TLSFingerprintHeader = header
	t.Cleanup(func() { cfg.Risk.TLSFingerprintHeader = prev })
}

// installFingerprintList 注入一份测试用指纹参考列表，nil 表示只记录不计分
func installFingerprintList(t *testing.T, payload string) {
	manager := services.GetServiceManager()
	prev := manager.GetTLSFingerprintList()
	var list *risk.FingerprintList
	if payload != "" {
		parsed, err := risk.ParseFingerprintList([]byte(payload))
		require.NoError(t, err)
		list = parsed
	}
	manager.SetTLSFingerprintList(list)
	t.Cleanup(func() { manager.SetTLSFingerprintList(prev) })
}

// trustedRemoteAddr 从 TRUSTED_PROXIES 的第一个网段里取一个地址，host 用于区分用例避免单 IP 计数串扰
func trustedRemoteAddr(t *testing.T, host byte) string {
	for _, item := range config.LoadConfig().Security.TrustedProxies {
		if _, network, err := net.ParseCIDR(item); err == nil && network.IP.To4() != nil {
			ip := network.IP.To4()
			return fmt.Sprintf("%d.%d.%d.%d:4321", ip[0], ip[1], ip[2]|1, host)
		}
		if ip := net.ParseIP(item); ip != nil {
			return ip.String() + ":4321"
		}
	}
	t.Skip("TRUSTED_PROXIES 为空，无法构造可信代理来源")
	return ""
}

func issueWithFingerprintHeader(t *testing.T, remoteAddr, ua, fp string) map[string]interface{} {
	req := withBrowserHeaders(rawIssueRequest(t, "slide-text", map[string]interface{}{"dpr": 2, "env": cleanIssueEnvFor(ua)}), ua)
	req.RemoteAddr = remoteAddr
	req.Header.Set(testJA4Header, fp)
	return issueBody(t, serve(req))
}

func TestIsTrustedProxyRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	check := func(remote string) bool {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.Request.RemoteAddr = remote
		return helper.IsTrustedProxyRequest(c)
	}
	assert.True(t, check(trustedRemoteAddr(t, 3)), "可信网段内的直连来源")
	assert.False(t, check("198.18.7.7:4321"), "基准测试网段不在默认可信列表")
	assert.False(t, check("not-an-ip"), "非法地址不可信")
}

func TestTLSFingerprint_IgnoredFromUntrustedRemote(t *testing.T) {
	rt := config.Runtime()
	withTLSFingerprintHeader(t, testJA4Header)
	installFingerprintList(t, testListPayload)
	base := services.ClampDifficulty(rt.Captcha.DefaultDifficulty)

	ua := uniqueUA("tls-untrusted")
	body := issueWithFingerprintHeader(t, remoteAddrFor(ua), ua, testDeniedFP)
	assert.Equal(t, base, int(body["difficulty"].(float64)), "非可信来源携带的指纹头必须被忽略")
	entry := telemetryEntryFor(t, body["key"].(string))
	_, hasTLS := entry["tls"]
	assert.False(t, hasTLS, "被忽略的指纹不应写入遥测")
}

func TestTLSFingerprint_DeniedFromTrustedProxyEscalates(t *testing.T) {
	rt := config.Runtime()
	withTLSFingerprintHeader(t, testJA4Header)
	installFingerprintList(t, testListPayload)
	base := services.ClampDifficulty(rt.Captcha.DefaultDifficulty)

	ua := uniqueUA("tls-denied")
	body := issueWithFingerprintHeader(t, trustedRemoteAddr(t, 21), ua, testDeniedFP)
	assert.Equal(t, services.ClampDifficulty(base+1), int(body["difficulty"].(float64)), "拒绝列表 40 分应加 1 级")
	entry := telemetryEntryFor(t, body["key"].(string))
	assert.Equal(t, testDeniedFP, entry["tls"])
	assert.Contains(t, fmt.Sprint(entry["reasons"]), "tls_denylisted(+40)")
}

func TestTLSFingerprint_FamilyMismatchEscalates(t *testing.T) {
	rt := config.Runtime()
	withTLSFingerprintHeader(t, testJA4Header)
	installFingerprintList(t, testListPayload)
	base := services.ClampDifficulty(rt.Captcha.DefaultDifficulty)

	ua := uniqueUA("tls-family")
	body := issueWithFingerprintHeader(t, trustedRemoteAddr(t, 22), ua, testFirefoxFP)
	assert.Equal(t, services.ClampDifficulty(base+1), int(body["difficulty"].(float64)), "Firefox 指纹配 Chrome UA 应加 1 级")
	assert.Contains(t, fmt.Sprint(telemetryEntryFor(t, body["key"].(string))["reasons"]), "tls_ua_family_mismatch(+30)")

	ua2 := uniqueUA("tls-match")
	body = issueWithFingerprintHeader(t, trustedRemoteAddr(t, 23), ua2, testChromeFP)
	assert.Equal(t, base, int(body["difficulty"].(float64)), "家族一致不加难")
}

func TestTLSFingerprint_RecordedWithoutList(t *testing.T) {
	rt := config.Runtime()
	withTLSFingerprintHeader(t, testJA4Header)
	installFingerprintList(t, "")
	base := services.ClampDifficulty(rt.Captcha.DefaultDifficulty)

	ua := uniqueUA("tls-nolist")
	body := issueWithFingerprintHeader(t, trustedRemoteAddr(t, 24), ua, testUnknownFP)
	assert.Equal(t, base, int(body["difficulty"].(float64)), "没有列表时指纹只记录不计分")
	assert.Equal(t, testUnknownFP, telemetryEntryFor(t, body["key"].(string))["tls"], "指纹原值应写入遥测供整理列表")
}

func TestTLSFingerprint_HeaderDisabledByDefault(t *testing.T) {
	rt := config.Runtime()
	withTLSFingerprintHeader(t, "")
	installFingerprintList(t, testListPayload)

	ua := uniqueUA("tls-disabled")
	body := issueWithFingerprintHeader(t, trustedRemoteAddr(t, 25), ua, testDeniedFP)
	assert.Equal(t, services.ClampDifficulty(rt.Captcha.DefaultDifficulty), int(body["difficulty"].(float64)), "未配置头名时即使来自可信代理也不读取")
	_, hasTLS := telemetryEntryFor(t, body["key"].(string))["tls"]
	assert.False(t, hasTLS)
}
