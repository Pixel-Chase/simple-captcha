package risk

import (
	"strings"
	"testing"
	"time"
)

const (
	chromeWinUA   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"
	firefoxWinUA  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:131.0) Gecko/20100101 Firefox/131.0"
	safariMacUA   = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15"
	chromeMobile  = "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36"
	curlUA        = "curl/8.5.0"
	acceptZH      = "zh-CN,zh;q=0.9,en;q=0.8"
	secChUAChrome = `"Chromium";v="130", "Google Chrome";v="130", "Not?A_Brand";v="99"`
)

// chromeSignals 一份真实桌面 Chrome 发出的请求头
func chromeSignals() *RequestSignals {
	return &RequestSignals{
		UserAgent:       chromeWinUA,
		AcceptLanguage:  acceptZH,
		SecFetchMode:    "cors",
		SecFetchSite:    "cross-site",
		SecFetchDest:    "empty",
		SecChUA:         secChUAChrome,
		SecChUAMobile:   "?0",
		SecChUAPlatform: `"Windows"`,
		DPRParam:        2,
	}
}

// chromeEnv 与 chromeSignals 一致的自报环境
func chromeEnv() *Env {
	return &Env{
		UA:        chromeWinUA,
		Platform:  "Win32",
		Languages: []string{"zh-CN", "en"},
		Timezone:  "Asia/Shanghai",
		TzOffset:  -480,
		DPR:       2,
		Cores:     8,
		Plugins:   5,
	}
}

func hasReason(reasons []string, name string) bool {
	for _, r := range reasons {
		if strings.HasPrefix(r, name+"(") {
			return true
		}
	}
	return false
}

func TestEvaluateRequest_BrowserIsConsistent(t *testing.T) {
	r := EvaluateRequest(chromeSignals(), chromeEnv())
	if r.Score != 0 || !r.Consistent || len(r.Reasons) != 0 {
		t.Fatalf("real browser headers with matching env should be consistent, got %+v", r)
	}
	if r := EvaluateRequest(chromeSignals(), nil); r.Score != 0 || !r.Consistent {
		t.Errorf("browser headers without env (telemetry off) should be consistent, got %+v", r)
	}
	if r := EvaluateRequest(nil, chromeEnv()); r.Score != 0 || r.Consistent {
		t.Errorf("nil signals must be neutral and not consistent, got %+v", r)
	}
}

func TestEvaluateRequest_HeaderRules(t *testing.T) {
	cases := []struct {
		name     string
		sig      *RequestSignals
		want     []string
		dontWant []string
		score    int
	}{
		{
			name:     "curl default headers",
			sig:      &RequestSignals{UserAgent: curlUA},
			want:     []string{"non_browser_ua", "accept_language_missing"},
			dontWant: []string{"sec_fetch_missing", "client_hints_missing"},
			score:    35,
		},
		{
			name:  "empty UA",
			sig:   &RequestSignals{AcceptLanguage: acceptZH},
			want:  []string{"non_browser_ua"},
			score: 20,
		},
		{
			name:     "fake Chrome UA without browser headers",
			sig:      &RequestSignals{UserAgent: chromeWinUA},
			want:     []string{"sec_fetch_missing", "client_hints_missing", "accept_language_missing"},
			dontWant: []string{"non_browser_ua"},
			score:    55,
		},
		{
			name:     "Firefox needs Sec-Fetch but not Client Hints",
			sig:      &RequestSignals{UserAgent: firefoxWinUA, AcceptLanguage: acceptZH},
			want:     []string{"sec_fetch_missing"},
			dontWant: []string{"client_hints_missing"},
			score:    25,
		},
		{
			name:     "Safari without Client Hints or Sec-Fetch is fine",
			sig:      &RequestSignals{UserAgent: safariMacUA, AcceptLanguage: acceptZH},
			dontWant: []string{"client_hints_missing", "sec_fetch_missing", "non_browser_ua"},
			score:    0,
		},
		{
			name: "mobile hint contradicts desktop UA",
			sig: func() *RequestSignals {
				s := chromeSignals()
				s.SecChUAMobile = "?1"
				return s
			}(),
			want:  []string{"client_hints_mismatch"},
			score: 25,
		},
		{
			name: "platform hint contradicts UA",
			sig: func() *RequestSignals {
				s := chromeSignals()
				s.SecChUAPlatform = `"macOS"`
				return s
			}(),
			want:  []string{"client_hints_mismatch"},
			score: 25,
		},
		{
			name: "android mobile chrome consistent",
			sig: &RequestSignals{
				UserAgent: chromeMobile, AcceptLanguage: acceptZH, SecFetchMode: "cors",
				SecChUA: secChUAChrome, SecChUAMobile: "?1", SecChUAPlatform: `"Android"`,
			},
			dontWant: []string{"client_hints_mismatch"},
			score:    0,
		},
	}
	for _, c := range cases {
		r := EvaluateRequest(c.sig, nil)
		if r.Score != c.score {
			t.Errorf("%s: score %d, want %d (%v)", c.name, r.Score, c.score, r.Reasons)
		}
		for _, w := range c.want {
			if !hasReason(r.Reasons, w) {
				t.Errorf("%s: expected reason %s in %v", c.name, w, r.Reasons)
			}
		}
		for _, d := range c.dontWant {
			if hasReason(r.Reasons, d) {
				t.Errorf("%s: unexpected reason %s in %v", c.name, d, r.Reasons)
			}
		}
		if c.score > 0 && r.Consistent {
			t.Errorf("%s: flagged request must not be consistent", c.name)
		}
	}
}

func TestEvaluateRequest_CrossLayerRules(t *testing.T) {
	mutate := func(fn func(env *Env)) *Env {
		env := chromeEnv()
		fn(env)
		return env
	}
	mobileTrue := true
	cases := []struct {
		name  string
		env   *Env
		want  string
		score int
	}{
		{"ua differs", mutate(func(e *Env) { e.UA = firefoxWinUA }), "ua_mismatch", 30},
		{"language not accepted", mutate(func(e *Env) { e.Languages = []string{"fr-FR"} }), "language_mismatch", 10},
		{"platform differs", mutate(func(e *Env) { e.Platform = "MacIntel" }), "platform_mismatch", 20},
		{"mobile differs", mutate(func(e *Env) { e.Mobile = &mobileTrue }), "mobile_mismatch", 20},
		{"tz offset differs", mutate(func(e *Env) { e.TzOffset = 0 }), "tz_offset_mismatch", 15},
		{"dpr differs", mutate(func(e *Env) { e.DPR = 1 }), "dpr_mismatch", 10},
	}
	for _, c := range cases {
		r := EvaluateRequest(chromeSignals(), c.env)
		if r.Score != c.score || !hasReason(r.Reasons, c.want) {
			t.Errorf("%s: want %s(+%d), got score %d reasons %v", c.name, c.want, c.score, r.Score, r.Reasons)
		}
		if r.Consistent {
			t.Errorf("%s: mismatching env must not be consistent", c.name)
		}
	}

	// 不应误报的情况
	fine := []struct {
		name string
		env  *Env
	}{
		{"zh vs zh-CN primary subtag", mutate(func(e *Env) { e.Languages = []string{"zh"} })},
		{"UTC with zero offset (resistFingerprinting)", mutate(func(e *Env) { e.Timezone = "UTC"; e.TzOffset = 0 })},
		{"unknown timezone name skipped", mutate(func(e *Env) { e.Timezone = "Mars/Olympus"; e.TzOffset = 0 })},
		{"unknown platform skipped", mutate(func(e *Env) { e.Platform = "FreeBSD amd64" })},
		{"dpr 1.75 rounds to 2", mutate(func(e *Env) { e.DPR = 1.75 })},
		{"dpr 3.5 clamps to 3", func() *Env {
			e := chromeEnv()
			e.DPR = 3.5
			return e
		}()},
		{"empty env ua skipped", mutate(func(e *Env) { e.UA = "" })},
	}
	for _, c := range fine {
		sig := chromeSignals()
		if c.name == "dpr 3.5 clamps to 3" {
			sig.DPRParam = 3
		}
		r := EvaluateRequest(sig, c.env)
		if r.Score != 0 || !r.Consistent {
			t.Errorf("%s: should be consistent, got %+v", c.name, r)
		}
	}

	// Android：navigator.platform 为 Linux armv8l，但 Client Hints 平台为 Android，不算矛盾
	android := &RequestSignals{UserAgent: chromeMobile, AcceptLanguage: acceptZH, SecFetchMode: "cors", SecChUA: secChUAChrome, SecChUAMobile: "?1", SecChUAPlatform: `"Android"`}
	androidEnv := &Env{UA: chromeMobile, Platform: "Linux armv8l", Languages: []string{"zh-CN"}, Mobile: &mobileTrue}
	if r := EvaluateRequest(android, androidEnv); r.Score != 0 || !r.Consistent {
		t.Errorf("android linux platform should match Android hint, got %+v", r)
	}
}

func TestTimezoneMatches_UsesEmbeddedTzdata(t *testing.T) {
	if _, err := time.LoadLocation("Asia/Shanghai"); err != nil {
		t.Fatalf("Asia/Shanghai must load (time/tzdata embedded): %v", err)
	}
	if known, ok := timezoneMatches("Asia/Shanghai", -480); !known || !ok {
		t.Errorf("Asia/Shanghai with JS offset -480 should match, got known=%v ok=%v", known, ok)
	}
	if known, ok := timezoneMatches("Asia/Shanghai", 0); !known || ok {
		t.Errorf("Asia/Shanghai with offset 0 should mismatch, got known=%v ok=%v", known, ok)
	}
	if known, _ := timezoneMatches("Not/AZone", 0); known {
		t.Error("unknown zone must be reported as not known")
	}
}

func TestEvaluateEnv_MergesRequestReasons(t *testing.T) {
	env := chromeEnv()
	env.Webdriver = true
	sig := &RequestSignals{UserAgent: chromeWinUA} // 冒充 Chrome 却没有浏览器请求头
	r := EvaluateEnv(env, sig)
	if r.Score != 45+55 {
		t.Errorf("env + request scores should add up to 100, got %d (%v)", r.Score, r.Reasons)
	}
	if !r.HardMarker || r.Consistent {
		t.Errorf("hard marker must be kept and request must be inconsistent, got %+v", r)
	}
	if len(r.RequestReasons) != 3 {
		t.Errorf("request reasons should be reported separately, got %v", r.RequestReasons)
	}
}
