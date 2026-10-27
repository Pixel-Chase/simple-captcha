package services

import (
	"math/rand"
	"simple-captcha/config"
	"simple-captcha/risk"
	"testing"
)

// testPolicyConfig 单测使用的固定策略（与默认值一致），按值返回便于逐项修改
func testPolicyConfig() config.RuntimeSettings {
	s := config.DefaultRuntimeSettings()
	s.Captcha.DefaultDifficulty = 3
	s.Captcha.IPEscalatePerMinute = 30
	s.Risk.Enabled = true
	s.Risk.Mode = "enforce"
	s.Risk.EscalateThreshold = 55
	s.Risk.BlockThreshold = 85
	s.Risk.EnvScoreCap = 60
	s.Risk.BlockMinBehavior = 20
	s.Risk.FPReplayThreshold = 5
	s.Risk.FPReplayWindowSeconds = 600
	return s
}

func cleanEnv() *risk.Env {
	return &risk.Env{
		UA:        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
		Platform:  "Win32",
		Languages: []string{"zh-CN"},
		Timezone:  "Asia/Shanghai",
		TzOffset:  -480,
		Screen:    [2]int{1920, 1080},
		Viewport:  [2]int{1536, 864},
		DPR:       1.25,
		Cores:     8,
		Plugins:   5,
		WebGL:     "ANGLE (NVIDIA, NVIDIA GeForce RTX 3060 Direct3D11 vs_5_0 ps_5_0, D3D11)",
	}
}

func TestDecide_DefaultStaysForCleanEnv(t *testing.T) {
	policy := newChallengePolicyWithSettings(fixedSettings(testPolicyConfig()), nil)
	fresh := &ClientRisk{}

	if d := policy.Decide(IssueInput{Env: cleanEnv(), State: fresh}); d.Level != 3 || d.Boost != 0 || d.EnvScore != 0 {
		t.Errorf("clean env should stay at default 3, got %+v", d)
	}
	if d := policy.Decide(IssueInput{Env: cleanEnv(), State: &ClientRisk{Successes: 5}}); d.Level != 3 {
		t.Errorf("success history must not lower difficulty, got %+v", d)
	}
	if d := policy.Decide(IssueInput{State: fresh}); d.Level != 3 || len(d.Reasons) != 0 {
		t.Errorf("nil env (telemetry off) should stay at default without reasons, got %+v", d)
	}
}

func TestDecide_SoftwareRendererAloneDoesNotEscalate(t *testing.T) {
	policy := newChallengePolicyWithSettings(fixedSettings(testPolicyConfig()), nil)
	vdi := cleanEnv()
	vdi.WebGL = "llvmpipe (LLVM 15.0.7, 256 bits)"
	d := policy.Decide(IssueInput{Env: vdi, State: &ClientRisk{}})
	if d.Level != 3 || d.EnvScore != 25 || d.Boost != 0 {
		t.Errorf("VDI renderer alone (25) is below the boost threshold, got %+v", d)
	}
}

func TestDecide_AutomationEscalates(t *testing.T) {
	policy := newChallengePolicyWithSettings(fixedSettings(testPolicyConfig()), nil)

	webdriver := cleanEnv()
	webdriver.Webdriver = true
	if d := policy.Decide(IssueInput{Env: webdriver, State: &ClientRisk{}}); d.Level != 4 || d.Boost != 1 || d.EnvScore != 45 {
		t.Errorf("webdriver should add one level, got %+v", d)
	}

	both := cleanEnv()
	both.Webdriver = true
	both.Automation = []string{"__playwright"}
	d := policy.Decide(IssueInput{Env: both, State: &ClientRisk{}})
	if d.Level != 5 || d.Boost != 2 {
		t.Errorf("webdriver + automation should add two levels, got %+v", d)
	}
	if d.EnvScore != 60 {
		t.Errorf("env score should be capped to 60, got %d", d.EnvScore)
	}
}

func TestDecide_HistoryAndEnvClampToMax(t *testing.T) {
	policy := newChallengePolicyWithSettings(fixedSettings(testPolicyConfig()), nil)
	webdriver := cleanEnv()
	webdriver.Webdriver = true
	d := policy.Decide(IssueInput{Env: webdriver, State: &ClientRisk{Fails: 4, LastScore: 90}, IPRequests: 100})
	if d.Level != MaxDifficulty {
		t.Errorf("stacked escalations must clamp to %d, got %+v", MaxDifficulty, d)
	}
}

func TestDecide_IPPressureStillApplies(t *testing.T) {
	policy := newChallengePolicyWithSettings(fixedSettings(testPolicyConfig()), nil)
	if d := policy.Decide(IssueInput{Env: cleanEnv(), State: &ClientRisk{}, IPRequests: 31}); d.Level != 4 {
		t.Errorf("ip pressure above threshold should add one level, got %+v", d)
	}
}

func TestDecide_FingerprintReplay(t *testing.T) {
	policy := newChallengePolicyWithSettings(fixedSettings(testPolicyConfig()), nil)
	threshold := testPolicyConfig().Risk.FPReplayThreshold
	if d := policy.Decide(IssueInput{Env: cleanEnv(), State: &ClientRisk{}, FPReplays: threshold}); d.Level != 3 {
		t.Errorf("replays at threshold should not escalate, got %+v", d)
	}
	d := policy.Decide(IssueInput{Env: cleanEnv(), State: &ClientRisk{}, FPReplays: threshold + 1})
	if d.Level != 4 || d.EnvScore != fpReplayPoints {
		t.Errorf("replayed fingerprint alone should add one level, got %+v", d)
	}

	disabled := testPolicyConfig()
	disabled.Risk.FPReplayThreshold = 0
	if d := newChallengePolicyWithSettings(fixedSettings(disabled), nil).Decide(IssueInput{Env: cleanEnv(), State: &ClientRisk{}, FPReplays: 99}); d.Level != 3 {
		t.Errorf("threshold 0 disables replay escalation, got %+v", d)
	}
}

func TestDecide_RiskDisabledIgnoresEnv(t *testing.T) {
	cfg := testPolicyConfig()
	cfg.Risk.Enabled = false
	policy := newChallengePolicyWithSettings(fixedSettings(cfg), nil)
	webdriver := cleanEnv()
	webdriver.Webdriver = true
	if d := policy.Decide(IssueInput{Env: webdriver, State: &ClientRisk{}}); d.Level != 3 || d.Boost != 0 {
		t.Errorf("RISK_ENABLED=false must ignore env signals, got %+v", d)
	}
}

func TestDecide_NilPolicyFallsBackToDefault(t *testing.T) {
	var policy *ChallengePolicy
	if d := policy.Decide(IssueInput{}); d.Level != 3 {
		t.Errorf("nil policy should fall back to 3, got %+v", d)
	}
}

// TestDecide_NeverBelowDefault 不变量：任何输入下难度都不低于默认值，也不超过上限
func TestDecide_NeverBelowDefault(t *testing.T) {
	rnd := rand.New(rand.NewSource(7))
	for base := MinDifficulty; base <= MaxDifficulty; base++ {
		cfg := testPolicyConfig()
		cfg.Captcha.DefaultDifficulty = base
		policy := newChallengePolicyWithSettings(fixedSettings(cfg), nil)
		for i := 0; i < 200; i++ {
			var env *risk.Env
			if rnd.Intn(3) > 0 {
				env = cleanEnv()
				env.Webdriver = rnd.Intn(4) == 0
				if rnd.Intn(4) == 0 {
					env.Automation = []string{"x"}
				}
				if rnd.Intn(3) == 0 {
					env.WebGL = "SwiftShader"
				}
				if rnd.Intn(3) == 0 {
					env.Languages = nil
				}
				if rnd.Intn(3) == 0 {
					env.Plugins = 0
				}
			}
			in := IssueInput{
				Env:        env,
				IPRequests: rnd.Intn(200),
				FPReplays:  rnd.Intn(10),
				State:      &ClientRisk{Fails: rnd.Intn(6), Successes: rnd.Intn(6), LastScore: rnd.Intn(101)},
			}
			d := policy.Decide(in)
			if d.Level < base || d.Level > MaxDifficulty {
				t.Fatalf("base %d: level %d out of [%d, %d] for %+v", base, d.Level, base, MaxDifficulty, in)
			}
			if d.Base != base {
				t.Fatalf("decision base %d != configured %d", d.Base, base)
			}
		}
	}
}
