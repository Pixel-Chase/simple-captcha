package services

import (
	"simple-captcha/config"
	"testing"
)

func TestClampDifficulty(t *testing.T) {
	cases := map[int]int{-3: 3, 0: 3, 1: 1, 3: 3, 5: 5, 9: 5}
	for in, want := range cases {
		if got := ClampDifficulty(in); got != want {
			t.Errorf("ClampDifficulty(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestClampScale(t *testing.T) {
	if got := ClampScale(0, 3); got != MinRenderScale {
		t.Errorf("scale 0 should clamp to %d, got %d", MinRenderScale, got)
	}
	if got := ClampScale(1, 3); got != MinRenderScale {
		t.Errorf("1x screens should still get %dx images, got %d", MinRenderScale, got)
	}
	if got := ClampScale(2, 1); got != 1 {
		t.Errorf("max 1 configured by operator should win over the render floor, got %d", got)
	}
	if got := ClampScale(5, 3); got != 3 {
		t.Errorf("scale 5 should clamp to 3, got %d", got)
	}
	if got := ClampScale(3, 2); got != 2 {
		t.Errorf("scale 3 with max 2 should clamp to 2, got %d", got)
	}
	if got := ClampScale(2, 99); got != 2 {
		t.Errorf("invalid max should fall back to global max, got %d", got)
	}
}

func TestDifficultyProfiles_MonotonicTolerance(t *testing.T) {
	prev := GetDifficultyProfile(1)
	for level := 2; level <= MaxDifficulty; level++ {
		cur := GetDifficultyProfile(level)
		if cur.ClickTolerance > prev.ClickTolerance || cur.SlideTolerance > prev.SlideTolerance || cur.RotateTolerance > prev.RotateTolerance {
			t.Errorf("level %d tolerance should not exceed level %d", level, level-1)
		}
		if cur.ClickLen.Min < prev.ClickLen.Min || cur.PowExtraBits < prev.PowExtraBits {
			t.Errorf("level %d should be at least as hard as level %d", level, level-1)
		}
		prev = cur
	}
}

func TestToleranceScalesWithImage(t *testing.T) {
	meta1 := &ChallengeMeta{Difficulty: 3, Scale: 1}
	meta2 := &ChallengeMeta{Difficulty: 3, Scale: 2}
	if ClickToleranceFor(meta2) != ClickToleranceFor(meta1)*2 {
		t.Error("click tolerance should scale linearly with image scale")
	}
	if SlideToleranceFor(meta2) != SlideToleranceFor(meta1)*2 {
		t.Error("slide tolerance should scale linearly with image scale")
	}
	if RotateToleranceFor(meta2) != RotateToleranceFor(meta1) {
		t.Error("rotate tolerance is angular and must not scale")
	}
}

func TestLevelFromState(t *testing.T) {
	riskCfg := config.RiskConfig{Enabled: true, EscalateThreshold: 55, BlockThreshold: 85}

	if got := LevelFromState(3, ClientRisk{}, riskCfg); got != 3 {
		t.Errorf("fresh client should stay at base, got %d", got)
	}
	if got := LevelFromState(3, ClientRisk{Fails: 2}, riskCfg); got != 4 {
		t.Errorf("2 fails should escalate once, got %d", got)
	}
	if got := LevelFromState(3, ClientRisk{Fails: 4}, riskCfg); got != 5 {
		t.Errorf("4 fails should escalate twice, got %d", got)
	}
	if got := LevelFromState(1, ClientRisk{LastScore: 60}, riskCfg); got != 2 {
		t.Errorf("score above escalate threshold should add one level, got %d", got)
	}
	if got := LevelFromState(1, ClientRisk{LastScore: 90}, riskCfg); got != 3 {
		t.Errorf("score above block threshold should add two levels, got %d", got)
	}
	disabled := config.RiskConfig{Enabled: false, EscalateThreshold: 55, BlockThreshold: 85}
	if got := LevelFromState(2, ClientRisk{LastScore: 100}, disabled); got != 2 {
		t.Errorf("disabled risk should ignore score, got %d", got)
	}
}

func TestIPPressureBoost(t *testing.T) {
	cases := []struct{ requests, threshold, want int }{
		{0, 30, 0},
		{30, 30, 0},
		{31, 30, 1},
		{60, 30, 1},
		{61, 30, 2},
		{100000, 30, 2},
		{500, 0, 0},
	}
	for _, c := range cases {
		if got := IPPressureBoost(c.requests, c.threshold); got != c.want {
			t.Errorf("IPPressureBoost(%d, %d) = %d, want %d", c.requests, c.threshold, got, c.want)
		}
	}

	// 封顶值恰好是加难到顶所需的最小计数：少一次还没到顶，计数停在这里不影响结果
	sat := IPPressureSaturation(30)
	if IPPressureBoost(sat, 30) != 2 || IPPressureBoost(sat-1, 30) != 1 {
		t.Errorf("IPPressureSaturation(30) = %d should be the first count reaching the top boost", sat)
	}
	if IPPressureSaturation(0) != 0 {
		t.Error("threshold 0 disables counting")
	}
}

func TestChooseAutoType(t *testing.T) {
	first := func(int) int { return 0 }
	if got := ChooseAutoType(1, first); got != "slide-text" {
		t.Errorf("low risk first pick should be slide-text, got %s", got)
	}
	if got := ChooseAutoType(5, first); got != "click-text" {
		t.Errorf("high risk should only offer click types, got %s", got)
	}

	hard := map[string]bool{"click-text": true, "click-shape": true}
	for i := 0; i < 20; i++ {
		idx := i
		got := ChooseAutoType(4, func(n int) int { return idx % n })
		if !hard[got] {
			t.Errorf("level 4 should never offer %s", got)
		}
	}

	if got := ChooseAutoType(2, func(int) int { return 99 }); got == "" {
		t.Error("out-of-range picker must still return a type")
	}
}
