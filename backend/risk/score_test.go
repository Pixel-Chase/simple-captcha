package risk

import (
	"math"
	"math/rand"
	"testing"
)

// humanLikeTrace 模拟真人拖动：速度先快后慢、带抖动、越过终点后回调、途中有停顿
func humanLikeTrace() *Trace {
	rnd := rand.New(rand.NewSource(42))
	points := make([][3]float64, 0, 60)
	t := 120.0
	x := 0.0
	for i := 0; i < 50; i++ {
		progress := float64(i) / 49
		speed := 6*math.Sin(progress*math.Pi) + 0.5 // 钟形速度曲线
		x += speed + rnd.Float64()*1.5 - 0.75
		y := 2*math.Sin(progress*6) + rnd.Float64()*2 - 1
		dt := 14 + rnd.Float64()*10
		t += dt
		points = append(points, [3]float64{x, y, t})
		if i == 30 {
			// 中途停顿：时间流逝但几乎不动
			t += 180
			points = append(points, [3]float64{x + 0.3, y, t})
		}
	}
	// 越过终点再回调
	points = append(points, [3]float64{x + 7, 0.5, t + 20}, [3]float64{x + 1, 0.2, t + 60})
	return &Trace{
		Points:      points,
		PointerType: "mouse",
		StartedAt:   120,
		SubmittedAt: t + 200,
		Downs:       1,
		Ups:         1,
		Moves:       120,
	}
}

func humanEnv() *Env {
	return &Env{
		UA:          "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36",
		Platform:    "MacIntel",
		Languages:   []string{"zh-CN", "en"},
		Timezone:    "Asia/Shanghai",
		Screen:      [2]int{1920, 1080},
		Viewport:    [2]int{1440, 900},
		DPR:         2,
		Cores:       8,
		Plugins:     5,
		WebGL:       "ANGLE (Apple, Apple M1, OpenGL 4.1)",
		TouchPoints: 0,
	}
}

func TestEvaluate_HumanLikeScoresLow(t *testing.T) {
	result := Evaluate(Input{Type: "slide-text", Trace: humanLikeTrace(), Env: humanEnv(), PowExpected: true, PowProvided: true})
	if result.Score > 20 {
		t.Errorf("human-like input scored %d (%v), expected <= 20", result.Score, result.Reasons)
	}
	if !result.Features.Overshoot {
		t.Error("expected overshoot to be detected")
	}
	if result.Features.Pauses == 0 {
		t.Error("expected at least one pause")
	}
}

func TestEvaluate_LinearBotScoresHigh(t *testing.T) {
	points := make([][3]float64, 0, 40)
	for i := 0; i < 40; i++ {
		points = append(points, [3]float64{float64(i * 3), 0, float64(i * 4)})
	}
	trace := &Trace{Points: points, PointerType: "mouse", StartedAt: 10, SubmittedAt: 170, Untrusted: 40}
	env := humanEnv()
	env.Webdriver = true

	result := Evaluate(Input{Type: "slide-text", Trace: trace, Env: env, PowExpected: true, PowProvided: false})
	if result.Score < 85 {
		t.Errorf("bot-like input scored %d (%v), expected >= 85", result.Score, result.Reasons)
	}
}

func TestEvaluate_MissingTelemetryIsOnlyModeratelySuspicious(t *testing.T) {
	// telemetry=off：没有任何遥测，不应直接触发拦截阈值，但应留下明显痕迹
	result := Evaluate(Input{Type: "click-text", PowExpected: true, PowProvided: false})
	if result.Score < 30 || result.Score >= 85 {
		t.Errorf("missing telemetry scored %d (%v), expected within [30, 85)", result.Score, result.Reasons)
	}
}

func TestEvaluate_ClickModeUniformIntervals(t *testing.T) {
	trace := &Trace{
		Points:      [][3]float64{{10, 10, 100}, {50, 60, 400}, {120, 80, 700}},
		Clicks:      [][3]float64{{10, 10, 300}, {50, 60, 600}, {120, 80, 900}},
		PointerType: "mouse",
		StartedAt:   100,
		SubmittedAt: 1000,
	}
	result := Evaluate(Input{Type: "click-text", Trace: trace, Env: humanEnv(), PowExpected: true, PowProvided: true})
	found := false
	for _, r := range result.Reasons {
		if len(r) >= 23 && r[:23] == "uniform_click_intervals" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected uniform_click_intervals reason, got %v", result.Reasons)
	}
}

func TestEvaluate_ScoreClamped(t *testing.T) {
	result := Evaluate(Input{Type: "rotate", Env: &Env{Webdriver: true, Automation: []string{"x"}, WebGL: "SwiftShader"}, Trace: &Trace{Untrusted: 3}, PowExpected: true})
	if result.Score != 100 {
		t.Errorf("expected clamp to 100, got %d", result.Score)
	}
}

func TestExtract_Straightness(t *testing.T) {
	f := Extract(&Trace{Points: [][3]float64{{0, 0, 0}, {10, 0, 10}, {20, 0, 20}, {30, 0, 30}}})
	if math.Abs(f.Straightness-1) > 1e-9 {
		t.Errorf("straight line should have straightness 1, got %f", f.Straightness)
	}
	if f.DirectionChanges != 0 {
		t.Errorf("straight line should have no direction changes, got %d", f.DirectionChanges)
	}

	zigzag := Extract(&Trace{Points: [][3]float64{{0, 0, 0}, {10, 10, 10}, {20, 0, 20}, {30, 10, 30}, {40, 0, 40}}})
	if zigzag.DirectionChanges < 3 {
		t.Errorf("zigzag should have direction changes, got %d", zigzag.DirectionChanges)
	}
	if zigzag.Straightness <= 1.1 {
		t.Errorf("zigzag should not be straight, got %f", zigzag.Straightness)
	}
}
