package risk

import (
	"math"
	"strconv"
	"strings"
	"time"

	// 运行时镜像（alpine）没有系统 tzdata，内嵌一份供时区偏移校验使用
	_ "time/tzdata"
)

// RequestSignals 服务端从 HTTP 请求头提取的信号，与客户端自报的 Env 相互印证。
// 这些值由浏览器的网络栈自动填写，脚本要伪装成浏览器必须逐个补齐且彼此一致，比自报 JSON 可信得多。
type RequestSignals struct {
	UserAgent       string
	AcceptLanguage  string
	SecFetchMode    string
	SecFetchSite    string
	SecFetchDest    string
	SecChUA         string
	SecChUAMobile   string
	SecChUAPlatform string
	// DPRParam 出题请求携带的 dpr，0 表示未携带（校验请求不带）
	DPRParam float64
	// TLSFingerprint / HTTP2Fingerprint 由可信代理注入（只在直连来源命中 TRUSTED_PROXIES 时采信），为空表示未启用
	TLSFingerprint   string
	HTTP2Fingerprint string
	// Fingerprints 指纹参考列表，nil 表示只记录指纹不计分
	Fingerprints *FingerprintList
}

// RequestResult 请求侧评分
type RequestResult struct {
	Score   int
	Reasons []string
	// HardMarker 命中硬性标记（指纹在拒绝列表中）
	HardMarker bool
	// Consistent 请求头齐全且与自报环境一致。仅用于记录与后续分析，不参与难度决策
	Consistent bool
}

// minClientHintsChromium 从该主版本起 Chromium 默认在所有请求上发送低熵 Client Hints
const minClientHintsChromium = 89

// EvaluateRequest 核验请求头是否像真实浏览器，并与自报环境做跨层比对。sig 为 nil 时返回零值。
// 规则只对明确声明 Chrome / Firefox 的 UA 要求 Sec-Fetch-*，只对 Chromium 89+ 要求 Client Hints，
// Safari、WebView 与未知 UA 不因缺少这些头扣分。
func EvaluateRequest(sig *RequestSignals, env *Env) RequestResult {
	if sig == nil {
		return RequestResult{}
	}
	s := &scorer{}
	ua := sig.UserAgent
	lowerUA := strings.ToLower(ua)

	browserUA := strings.Contains(ua, "Mozilla/")
	expectSecFetch := strings.Contains(ua, "Chrome/") || strings.Contains(ua, "Firefox/")
	expectClientHints := chromiumMajor(ua) >= minClientHintsChromium

	if !browserUA {
		// curl、Python requests、Go http 等默认 UA 都不含 Mozilla/
		s.add(20, "non_browser_ua")
	}
	if expectSecFetch && sig.SecFetchMode == "" {
		s.add(25, "sec_fetch_missing")
	}
	if expectClientHints && sig.SecChUA == "" {
		// Brave 等隐私浏览器可能不发送，权重保守
		s.add(15, "client_hints_missing")
	}
	if mismatch := clientHintsMismatchUA(sig, lowerUA); mismatch {
		s.add(25, "client_hints_mismatch")
	}
	if sig.AcceptLanguage == "" {
		s.add(15, "accept_language_missing")
	}
	evaluateTLS(s, sig)

	if env != nil {
		if env.UA != "" && env.UA != ua {
			s.add(30, "ua_mismatch")
		}
		if len(env.Languages) > 0 && sig.AcceptLanguage != "" && !acceptsLanguage(sig.AcceptLanguage, env.Languages[0]) {
			s.add(10, "language_mismatch")
		}
		if envP, chP := envPlatform(env, lowerUA), normalizePlatform(sig.SecChUAPlatform); envP != "" && chP != "" && envP != chP {
			s.add(20, "platform_mismatch")
		}
		if env.Mobile != nil && sig.SecChUAMobile != "" && *env.Mobile != (sig.SecChUAMobile == "?1") {
			s.add(20, "mobile_mismatch")
		}
		if env.Timezone != "" {
			if known, matches := timezoneMatches(env.Timezone, env.TzOffset); known && !matches {
				s.add(15, "tz_offset_mismatch")
			}
		}
		if sig.DPRParam > 0 && env.DPR > 0 && clampScale(sig.DPRParam) != clampScale(env.DPR) {
			s.add(10, "dpr_mismatch")
		}
	}

	consistent := len(s.reasons) == 0 &&
		sig.AcceptLanguage != "" &&
		(!expectSecFetch || sig.SecFetchMode != "") &&
		(!expectClientHints || sig.SecChUA != "")

	return RequestResult{Score: s.score, Reasons: s.reasons, HardMarker: s.hard, Consistent: consistent}
}

// clientHintsMismatchUA Client Hints 与 UA 自身矛盾：移动标记或平台对不上
func clientHintsMismatchUA(sig *RequestSignals, lowerUA string) bool {
	if sig.SecChUAMobile != "" {
		chMobile := sig.SecChUAMobile == "?1"
		uaMobile := strings.Contains(lowerUA, "mobile")
		if chMobile != uaMobile {
			return true
		}
	}
	if chP := normalizePlatform(sig.SecChUAPlatform); chP != "" {
		if uaP := uaPlatform(lowerUA); uaP != "" && uaP != chP {
			return true
		}
	}
	return false
}

// chromiumMajor 解析 UA 中 Chrome/NNN 的主版本号，没有则返回 0
func chromiumMajor(ua string) int {
	idx := strings.Index(ua, "Chrome/")
	if idx < 0 {
		return 0
	}
	rest := ua[idx+len("Chrome/"):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	major, err := strconv.Atoi(rest[:end])
	if err != nil {
		return 0
	}
	return major
}

// uaPlatform 从小写 UA 推断操作系统，归一化为与 normalizePlatform 相同的取值
func uaPlatform(lowerUA string) string {
	switch {
	case strings.Contains(lowerUA, "windows"):
		return "windows"
	case strings.Contains(lowerUA, "android"):
		return "android"
	case strings.Contains(lowerUA, "iphone"), strings.Contains(lowerUA, "ipad"), strings.Contains(lowerUA, "ipod"):
		return "ios"
	case strings.Contains(lowerUA, "cros"):
		return "chrome os"
	case strings.Contains(lowerUA, "macintosh"), strings.Contains(lowerUA, "mac os x"):
		return "macos"
	case strings.Contains(lowerUA, "linux"):
		return "linux"
	}
	return ""
}

// normalizePlatform 把 Sec-CH-UA-Platform（带引号）、navigator.platform、userAgentData.platform 归一化；未知返回空
func normalizePlatform(value string) string {
	v := strings.ToLower(strings.TrimSpace(strings.Trim(strings.TrimSpace(value), `"`)))
	switch {
	case v == "":
		return ""
	case strings.HasPrefix(v, "win"):
		return "windows"
	case strings.HasPrefix(v, "mac"):
		return "macos"
	case v == "android":
		return "android"
	case v == "ios", strings.HasPrefix(v, "iphone"), strings.HasPrefix(v, "ipad"), strings.HasPrefix(v, "ipod"):
		return "ios"
	case v == "chrome os", v == "chromium os", v == "cros":
		return "chrome os"
	case strings.HasPrefix(v, "linux"):
		return "linux"
	}
	return ""
}

// envPlatform 自报平台归一化；Android 上 navigator.platform 是 Linux armv8l，结合 UA 纠正
func envPlatform(env *Env, lowerUA string) string {
	p := normalizePlatform(env.Platform)
	if p == "linux" && strings.Contains(lowerUA, "android") {
		return "android"
	}
	return p
}

// acceptsLanguage Accept-Language 中是否包含该语言的主语言子标签（zh-CN 与 zh 视为一致），* 视为匹配
func acceptsLanguage(header, lang string) bool {
	want := primarySubtag(lang)
	if want == "" {
		return true
	}
	for _, item := range strings.Split(header, ",") {
		tag := strings.TrimSpace(item)
		if i := strings.Index(tag, ";"); i >= 0 {
			tag = strings.TrimSpace(tag[:i])
		}
		if tag == "*" || primarySubtag(tag) == want {
			return true
		}
	}
	return false
}

func primarySubtag(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if i := strings.IndexAny(tag, "-_"); i >= 0 {
		tag = tag[:i]
	}
	return tag
}

// timezoneMatches 时区名能解析时，比较其当前 UTC 偏移与 JS getTimezoneOffset（分钟，符号相反）。
// 第一个返回值表示时区名是否可识别，无法识别时不参与评分
func timezoneMatches(name string, jsOffsetMinutes int) (known bool, matches bool) {
	loc, err := time.LoadLocation(name)
	if err != nil || loc == nil {
		return false, false
	}
	_, offsetSeconds := time.Now().In(loc).Zone()
	return true, offsetSeconds/60 == -jsOffsetMinutes
}

// clampScale 与 SDK 的 getDevicePixelRatio 相同：四舍五入后裁剪到 1-3
func clampScale(dpr float64) int {
	scale := int(math.Round(dpr))
	if scale < 1 {
		return 1
	}
	if scale > 3 {
		return 3
	}
	return scale
}
