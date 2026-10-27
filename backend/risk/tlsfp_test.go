package risk

import (
	"os"
	"path/filepath"
	"testing"
)

const testFingerprintList = `{
  "browser": {"T13D_CHROME_TEST": "Chrome", "t13d_firefox_test": "firefox"},
  "deny": ["t13d_curl_test", " "]
}`

func mustList(t *testing.T) *FingerprintList {
	list, err := ParseFingerprintList([]byte(testFingerprintList))
	if err != nil {
		t.Fatalf("parse list: %v", err)
	}
	return list
}

func TestParseFingerprintList(t *testing.T) {
	list := mustList(t)
	if got := list.Family("t13d_chrome_test"); got != "chrome" {
		t.Errorf("family lookup should be case-insensitive and normalized, got %q", got)
	}
	if !list.Denied("T13D_CURL_TEST") || list.Denied("t13d_chrome_test") || list.Denied("") {
		t.Error("deny lookup wrong")
	}
	var nilList *FingerprintList
	if nilList.Denied("x") || nilList.Family("x") != "" {
		t.Error("nil list must be neutral")
	}

	if _, err := ParseFingerprintList([]byte(`{bad json`)); err == nil {
		t.Error("invalid json must fail")
	}
	if _, err := ParseFingerprintList([]byte(`{"browser":{"abc":"  "}}`)); err == nil {
		t.Error("empty family must fail")
	}
}

func TestLoadFingerprintList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "list.json")
	if err := os.WriteFile(path, []byte(testFingerprintList), 0o600); err != nil {
		t.Fatal(err)
	}
	list, err := LoadFingerprintList(path)
	if err != nil || list.Family("t13d_firefox_test") != "firefox" {
		t.Fatalf("load failed: %v", err)
	}
	if _, err := LoadFingerprintList(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("missing file must fail")
	}
}

func TestUAFamily(t *testing.T) {
	cases := map[string]string{
		chromeWinUA:  "chrome",
		firefoxWinUA: "firefox",
		safariMacUA:  "safari",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0": "edge",
		curlUA: "",
		"":     "",
	}
	for ua, want := range cases {
		if got := uaFamily(ua); got != want {
			t.Errorf("uaFamily(%q) = %q, want %q", ua, got, want)
		}
	}
}

func TestEvaluateRequest_TLSRules(t *testing.T) {
	list := mustList(t)
	withFP := func(tls, h2 string, withList bool) *RequestSignals {
		sig := chromeSignals()
		sig.TLSFingerprint = tls
		sig.HTTP2Fingerprint = h2
		if withList {
			sig.Fingerprints = list
		}
		return sig
	}

	denied := EvaluateRequest(withFP("t13d_curl_test", "", true), nil)
	if denied.Score != 40 || !denied.HardMarker || !hasReason(denied.Reasons, "tls_denylisted") || denied.Consistent {
		t.Errorf("deny-listed fingerprint should be a 40-point hard marker, got %+v", denied)
	}
	if h2 := EvaluateRequest(withFP("", "t13d_curl_test", true), nil); !hasReason(h2.Reasons, "tls_denylisted") {
		t.Errorf("deny list applies to HTTP/2 fingerprint too, got %+v", h2)
	}

	mismatch := EvaluateRequest(withFP("t13d_firefox_test", "", true), nil)
	if mismatch.Score != 30 || mismatch.HardMarker || !hasReason(mismatch.Reasons, "tls_ua_family_mismatch") {
		t.Errorf("firefox fingerprint with Chrome UA should add 30 (not hard), got %+v", mismatch)
	}

	if ok := EvaluateRequest(withFP("t13d_chrome_test", "", true), nil); ok.Score != 0 || !ok.Consistent {
		t.Errorf("matching family must be neutral, got %+v", ok)
	}
	if unknown := EvaluateRequest(withFP("t13d_unknown", "", true), nil); unknown.Score != 0 || !unknown.Consistent {
		t.Errorf("unknown fingerprint must be neutral, got %+v", unknown)
	}
	if noList := EvaluateRequest(withFP("t13d_curl_test", "", false), nil); noList.Score != 0 || !noList.Consistent {
		t.Errorf("without a list fingerprints are only recorded, got %+v", noList)
	}

	// 硬性标记要透传到环境侧结果
	if env := EvaluateEnv(nil, withFP("t13d_curl_test", "", true)); !env.HardMarker || env.Score != 40 {
		t.Errorf("EvaluateEnv must carry the TLS hard marker, got %+v", env)
	}
}
