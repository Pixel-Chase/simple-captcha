package services

import (
	"simple-captcha/risk"
	"testing"
)

func TestShouldBlock_RequiresBehaviorEvidence(t *testing.T) {
	engine := &RiskEngine{settings: fixedSettings(testPolicyConfig())}

	cases := []struct {
		name   string
		result *risk.Result
		want   bool
	}{
		{"nil result", nil, false},
		{"below threshold even with hard marker", &risk.Result{Score: 84, BehaviorScore: 50, HardMarker: true}, false},
		{"env only, no behavior", &risk.Result{Score: 85, EnvScore: 85, BehaviorScore: 0}, false},
		{"env plus weak behavior", &risk.Result{Score: 85, EnvScore: 60, BehaviorScore: 19}, false},
		{"env plus behavior evidence", &risk.Result{Score: 85, EnvScore: 60, BehaviorScore: 25}, true},
		{"hard marker with little behavior", &risk.Result{Score: 85, EnvScore: 60, BehaviorScore: 5, HardMarker: true}, true},
	}
	for _, c := range cases {
		if got := engine.ShouldBlock(c.result); got != c.want {
			t.Errorf("%s: ShouldBlock = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestShouldBlock_ModeAndEnabled(t *testing.T) {
	blockable := &risk.Result{Score: 95, BehaviorScore: 50, HardMarker: true}

	logMode := testPolicyConfig()
	logMode.Risk.Mode = "log"
	if (&RiskEngine{settings: fixedSettings(logMode)}).ShouldBlock(blockable) {
		t.Error("log mode must never block")
	}

	disabled := testPolicyConfig()
	disabled.Risk.Enabled = false
	if (&RiskEngine{settings: fixedSettings(disabled)}).ShouldBlock(blockable) {
		t.Error("disabled risk must never block")
	}
}

// TestShouldBlock_VDIUserWithHumanTraceNotBlocked 软件渲染器 + 无插件 + 无语言列表的桌面环境，配合真人轨迹不得被拦
func TestShouldBlock_VDIUserWithHumanTraceNotBlocked(t *testing.T) {
	cfg := testPolicyConfig()
	engine := &RiskEngine{settings: fixedSettings(cfg)}
	env := &risk.Env{
		UA:       "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
		WebGL:    "llvmpipe (LLVM 15.0.7, 256 bits)",
		Screen:   [2]int{1280, 720},
		Viewport: [2]int{1900, 900}, // viewport_larger_than_screen
	}
	points := make([][3]float64, 0, 30)
	x := 0.0
	for i := 0; i < 30; i++ {
		x += 3 + float64(i%5)
		points = append(points, [3]float64{x, float64(i % 3), float64(200 + i*20 + (i%4)*7)})
	}
	points = append(points, [3]float64{x + 6, 0, 900}, [3]float64{x + 1, 0, 980})
	result := risk.Evaluate(risk.Input{
		Type:        "slide-text",
		Env:         env,
		Trace:       &risk.Trace{Points: points, PointerType: "mouse", StartedAt: 200, SubmittedAt: 1200},
		PowExpected: true,
		PowProvided: true,
		EnvScoreCap: cfg.Risk.EnvScoreCap,
	})
	if result.HardMarker {
		t.Fatalf("VDI signals must not be hard markers: %v", result.Reasons)
	}
	if engine.ShouldBlock(&result) {
		t.Errorf("VDI user with human trace must not be blocked: score=%d env=%d behavior=%d reasons=%v",
			result.Score, result.EnvScore, result.BehaviorScore, result.Reasons)
	}
}
