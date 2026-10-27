package risk

import (
	"reflect"
	"sort"
	"testing"
)

func sortedReasons(r []string) []string {
	out := append([]string(nil), r...)
	sort.Strings(out)
	return out
}

func TestEvaluateEnv_CleanEnvScoresZero(t *testing.T) {
	clean := EvaluateEnv(humanEnv(), nil)
	if clean.Score != 0 || clean.HardMarker || len(clean.Reasons) != 0 {
		t.Errorf("clean env should score 0 without reasons, got %+v", clean)
	}
	empty := EvaluateEnv(nil, nil)
	if empty.Score != 0 || empty.HardMarker || len(empty.Reasons) != 0 {
		t.Errorf("nil env must not add env_missing at issuance, got %+v", empty)
	}
}

func TestEvaluateEnv_HardMarkers(t *testing.T) {
	webdriver := humanEnv()
	webdriver.Webdriver = true
	if r := EvaluateEnv(webdriver, nil); r.Score != 45 || !r.HardMarker {
		t.Errorf("webdriver should be a 45-point hard marker, got %+v", r)
	}

	automation := humanEnv()
	automation.Automation = []string{"__playwright"}
	if r := EvaluateEnv(automation, nil); r.Score != 35 || !r.HardMarker {
		t.Errorf("automation markers should be a 35-point hard marker, got %+v", r)
	}

	// 软件渲染器常见于 VDI / 虚拟机，只是疑点，不是硬性标记
	vdi := humanEnv()
	vdi.WebGL = "llvmpipe (LLVM 15.0.7, 256 bits)"
	if r := EvaluateEnv(vdi, nil); r.Score != 25 || r.HardMarker {
		t.Errorf("software renderer should be 25 points and not hard, got %+v", r)
	}
}

func TestEvaluate_EnvScoreCapped(t *testing.T) {
	// webdriver 45 + automation 35 + software_renderer 25 + no_languages 10 = 115
	env := &Env{Webdriver: true, Automation: []string{"x"}, WebGL: "SwiftShader"}
	base := Input{Type: "slide-text", Trace: humanLikeTrace(), Env: env, PowExpected: true, PowProvided: true}

	uncapped := Evaluate(base)
	if uncapped.EnvScore != 115 {
		t.Fatalf("uncapped env score should be 115, got %d (%v)", uncapped.EnvScore, uncapped.Reasons)
	}
	if uncapped.BehaviorScore >= 0 {
		t.Fatalf("human-like trace should have negative behavior score, got %d", uncapped.BehaviorScore)
	}

	capped := base
	capped.EnvScoreCap = 60
	result := Evaluate(capped)
	if result.EnvScore != 60 {
		t.Errorf("env score should be capped to 60, got %d", result.EnvScore)
	}
	if want := 60 + result.BehaviorScore; result.Score != want {
		t.Errorf("total should be capped env + behavior = %d, got %d", want, result.Score)
	}
	if !result.HardMarker {
		t.Error("hard marker must survive the cap")
	}
	if result.Score >= 85 {
		t.Errorf("env-only evidence with human behavior must stay below block threshold, got %d", result.Score)
	}
}

func TestEvaluate_EnvMissingOnlyAtVerify(t *testing.T) {
	result := Evaluate(Input{Type: "click-text", Trace: &Trace{Clicks: [][3]float64{{1, 1, 500}, {2, 2, 900}}, Points: [][3]float64{{0, 0, 100}, {1, 1, 400}, {2, 2, 800}}, StartedAt: 100, SubmittedAt: 1000}, PowExpected: true, PowProvided: true})
	found := false
	for _, r := range result.Reasons {
		if r == "env_missing(+10)" {
			found = true
		}
	}
	if !found || result.EnvScore != 10 {
		t.Errorf("verify-time evaluation must flag missing env, got envScore=%d reasons=%v", result.EnvScore, result.Reasons)
	}
}

func TestEvaluate_SplitScoresAddUp(t *testing.T) {
	points := make([][3]float64, 0, 40)
	for i := 0; i < 40; i++ {
		points = append(points, [3]float64{float64(i * 3), 0, float64(i * 4)})
	}
	bot := humanEnv()
	bot.Webdriver = true
	inputs := []Input{
		{Type: "slide-text", Trace: humanLikeTrace(), Env: humanEnv(), PowExpected: true, PowProvided: true},
		{Type: "slide-text", Trace: &Trace{Points: points, PointerType: "mouse", StartedAt: 10, SubmittedAt: 170, Untrusted: 40}, Env: bot, PowExpected: true},
		{Type: "click-text", PowExpected: true},
		{Type: "rotate", Env: &Env{Webdriver: true, Automation: []string{"x"}, WebGL: "SwiftShader"}, Trace: &Trace{Untrusted: 3}, PowExpected: true, EnvScoreCap: 60},
	}
	for i, in := range inputs {
		r := Evaluate(in)
		want := r.EnvScore + r.BehaviorScore
		if want < 0 {
			want = 0
		}
		if want > 100 {
			want = 100
		}
		if r.Score != want {
			t.Errorf("case %d: score %d != clamp(env %d + behavior %d)", i, r.Score, r.EnvScore, r.BehaviorScore)
		}
	}
}

// TestEvaluate_RegressionSnapshot 锁定拆分前的分值与原因集合（不限上限时必须逐一相同）
func TestEvaluate_RegressionSnapshot(t *testing.T) {
	points := make([][3]float64, 0, 40)
	for i := 0; i < 40; i++ {
		points = append(points, [3]float64{float64(i * 3), 0, float64(i * 4)})
	}
	botEnv := humanEnv()
	botEnv.Webdriver = true

	cases := []struct {
		name    string
		in      Input
		score   int
		reasons []string
	}{
		{
			name:    "human_slide",
			in:      Input{Type: "slide-text", Trace: humanLikeTrace(), Env: humanEnv(), PowExpected: true, PowProvided: true},
			score:   0,
			reasons: []string{"overshoot_correction(-10)", "pauses(-5)", "variable_speed(-5)", "direction_changes(-5)"},
		},
		{
			name:    "missing_click",
			in:      Input{Type: "click-text", PowExpected: true, PowProvided: false},
			score:   45,
			reasons: []string{"env_missing(+10)", "pow_missing(+15)", "trace_missing(+20)"},
		},
		{
			name:    "clamped",
			in:      Input{Type: "rotate", Env: &Env{Webdriver: true, Automation: []string{"x"}, WebGL: "SwiftShader"}, Trace: &Trace{Untrusted: 3}, PowExpected: true},
			score:   100,
			reasons: []string{"webdriver(+45)", "automation_markers(+35)", "software_renderer(+25)", "no_languages(+10)", "pow_missing(+15)", "untrusted_events(+35)", "too_few_points(+25)"},
		},
		{
			name:    "linear_bot",
			in:      Input{Type: "slide-text", Trace: &Trace{Points: points, PointerType: "mouse", StartedAt: 10, SubmittedAt: 170, Untrusted: 40}, Env: botEnv, PowExpected: true, PowProvided: false},
			score:   100,
			reasons: []string{"webdriver(+45)", "pow_missing(+15)", "untrusted_events(+35)", "perfectly_straight(+25)", "constant_speed(+20)", "uniform_sampling(+15)", "no_hesitation(+10)", "no_reaction_time(+10)"},
		},
		{
			name:    "uniform_click",
			in:      Input{Type: "click-text", Trace: &Trace{Points: [][3]float64{{10, 10, 100}, {50, 60, 400}, {120, 80, 700}}, Clicks: [][3]float64{{10, 10, 300}, {50, 60, 600}, {120, 80, 900}}, PointerType: "mouse", StartedAt: 100, SubmittedAt: 1000}, Env: humanEnv(), PowExpected: true, PowProvided: true},
			score:   25,
			reasons: []string{"uniform_click_intervals(+25)"},
		},
	}
	for _, c := range cases {
		r := Evaluate(c.in)
		if r.Score != c.score {
			t.Errorf("%s: score %d, want %d (%v)", c.name, r.Score, c.score, r.Reasons)
		}
		if got, want := sortedReasons(r.Reasons), sortedReasons(c.reasons); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: reasons %v, want %v", c.name, got, want)
		}
	}
}

func TestEvaluateEnv_AnomalyWeights(t *testing.T) {
	cases := []struct {
		name      string
		anomalies []string
		score     int
		hard      bool
	}{
		{"zero outer size", []string{"zero_outer_size"}, 20, false},
		{"permission mismatch", []string{"permission_mismatch"}, 15, false},
		{"no media devices", []string{"no_media_devices"}, 10, false},
		{"chrome object missing", []string{"chrome_object_missing"}, 10, false},
		{"worker ua mismatch is hard", []string{"worker_ua_mismatch"}, 30, true},
		{"getter patched counted once", []string{"getter_patched:webdriver", "getter_patched:plugins", "getter_patched:languages"}, 30, true},
		{"duplicates counted once", []string{"zero_outer_size", "zero_outer_size"}, 20, false},
		{"unknown markers ignored", []string{"something_new", "another"}, 0, false},
	}
	for _, c := range cases {
		env := humanEnv()
		env.Anomalies = c.anomalies
		r := EvaluateEnv(env, nil)
		if r.Score != c.score || r.HardMarker != c.hard {
			t.Errorf("%s: got score %d hard %v (%v), want %d / %v", c.name, r.Score, r.HardMarker, r.Reasons, c.score, c.hard)
		}
	}

	// VDI / 虚拟机：软件渲染器 + 没有媒体设备指向同一个正常原因，后者不再叠加，环境分停在 25 不触发加难
	vdi := humanEnv()
	vdi.WebGL = "llvmpipe (LLVM 15.0.7, 256 bits)"
	vdi.Anomalies = []string{"no_media_devices"}
	if r := EvaluateEnv(vdi, nil); r.Score != 25 || hasReason(r.Reasons, "no_media_devices") {
		t.Errorf("software renderer should absorb no_media_devices, got %d %v", r.Score, r.Reasons)
	}
	// 没有软件渲染器时 no_media_devices 正常计分
	headless := humanEnv()
	headless.Anomalies = []string{"no_media_devices", "zero_outer_size"}
	if r := EvaluateEnv(headless, nil); r.Score != 30 {
		t.Errorf("no_media_devices + zero_outer_size should be 30, got %d %v", r.Score, r.Reasons)
	}
}

func TestEvaluate_AnomaliesStayUnderCap(t *testing.T) {
	env := humanEnv()
	env.Anomalies = []string{"zero_outer_size", "permission_mismatch", "no_media_devices", "chrome_object_missing", "worker_ua_mismatch", "getter_patched:webdriver"}
	// 20 + 15 + 10 + 10 + 30 + 30 = 115，裁剪到 60
	r := Evaluate(Input{Type: "slide-text", Trace: humanLikeTrace(), Env: env, PowExpected: true, PowProvided: true, EnvScoreCap: 60})
	if r.EnvScore != 60 {
		t.Errorf("six anomalies should be capped to 60, got %d (%v)", r.EnvScore, r.Reasons)
	}
	if !r.HardMarker {
		t.Error("worker / getter anomalies must keep the hard marker")
	}
}
