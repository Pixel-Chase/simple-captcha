package services

import "testing"

func TestMaskIP(t *testing.T) {
	cases := map[string]string{
		"203.0.113.77":         "203.0.*.*",
		"10.1.2.3":             "10.1.*.*",
		"2001:db8:1:2:3:4:5:6": "2001:db8:1::*",
		"::1":                  "0:0:0::*",
		"::ffff:203.0.113.9":   "203.0.*.*",
		"":                     "",
		"not-an-ip":            "*",
	}
	for in, want := range cases {
		if got := MaskIP(in); got != want {
			t.Errorf("MaskIP(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSummarizeUA(t *testing.T) {
	cases := map[string]string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36":               "Chrome 130 / Windows",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0": "Edge 130 / Windows",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15":         "Safari 17 / macOS",
		"Mozilla/5.0 (X11; Linux x86_64; rv:131.0) Gecko/20100101 Firefox/131.0":                                                        "Firefox 131 / Linux",
		"curl/8.5.0": "非浏览器",
		"":           "",
	}
	for in, want := range cases {
		if got := SummarizeUA(in); got != want {
			t.Errorf("SummarizeUA(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMaskTelemetry(t *testing.T) {
	entry := map[string]interface{}{
		"kind":    "issue",
		"ip":      "198.51.100.23",
		"ua":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
		"client":  "8d42bc159691be7d5dd15580ed6e0f99",
		"level":   4,
		"reasons": []string{"webdriver(+45)"},
	}
	masked := MaskTelemetry(entry)
	if masked["ip"] != "198.51.*.*" || masked["ua"] != "Chrome 130 / Windows" || masked["client"] != "8d42bc15" {
		t.Errorf("masking wrong: %+v", masked)
	}
	if masked["level"] != 4 || masked["kind"] != "issue" {
		t.Error("other fields must be preserved")
	}
	if entry["ip"] != "198.51.100.23" {
		t.Error("input must not be mutated")
	}
	if MaskTelemetry(nil) != nil {
		t.Error("nil in, nil out")
	}
}

// TestMask_Idempotent 写入侧已脱敏的值在读取侧再脱敏一次必须保持不变
func TestMask_Idempotent(t *testing.T) {
	for _, v := range []string{"198.51.*.*", "2001:db8:1::*", "*"} {
		if got := MaskIP(v); got != v {
			t.Errorf("MaskIP(%q) should be idempotent, got %q", v, got)
		}
	}
	for _, v := range []string{"Chrome 130 / Windows", "Edge 130 / Windows", "Safari 17 / macOS", "非浏览器", "非浏览器 / Linux", "其它"} {
		if got := SummarizeUA(v); got != v {
			t.Errorf("SummarizeUA(%q) should be idempotent, got %q", v, got)
		}
	}
	once := MaskTelemetry(map[string]interface{}{"ip": "203.0.113.9", "ua": "Mozilla/5.0 (X11; Linux x86_64; rv:131.0) Gecko/20100101 Firefox/131.0"})
	twice := MaskTelemetry(once)
	if twice["ip"] != once["ip"] || twice["ua"] != once["ua"] {
		t.Errorf("double masking changed values: %v -> %v", once, twice)
	}
}
