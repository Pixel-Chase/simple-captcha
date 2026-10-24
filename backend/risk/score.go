package risk

import (
	"fmt"
	"strings"
)

// 头less / 软件渲染器特征，常见于无头浏览器与容器环境
var headlessRenderers = []string{"swiftshader", "llvmpipe", "mesa offscreen", "softpipe"}

// Evaluate 基于规则对一次验证行为打分：0 表示很像人类，100 表示几乎可以确定是脚本。
// 规则按"叠加扣分 + 少量人类特征加分"的方式计算，最终裁剪到 [0, 100]。
func Evaluate(in Input) Result {
	score := 0
	reasons := make([]string, 0, 8)
	add := func(points int, reason string) {
		score += points
		reasons = append(reasons, fmt.Sprintf("%s(%+d)", reason, points))
	}

	drag := IsDragType(in.Type)
	features := Extract(in.Trace)

	// ---------- 环境信号 ----------
	if in.Env == nil {
		add(10, "env_missing")
	} else {
		env := in.Env
		if env.Webdriver {
			add(45, "webdriver")
		}
		if len(env.Automation) > 0 {
			add(35, "automation_markers")
		}
		if renderer := strings.ToLower(env.WebGL); renderer != "" {
			for _, marker := range headlessRenderers {
				if strings.Contains(renderer, marker) {
					add(25, "software_renderer")
					break
				}
			}
		}
		if in.Trace != nil && in.Trace.PointerType == "touch" && env.TouchPoints == 0 {
			add(20, "touch_without_touchscreen")
		}
		if env.Viewport[0] > env.Screen[0]+1 && env.Screen[0] > 0 ||
			env.Viewport[1] > env.Screen[1]+1 && env.Screen[1] > 0 {
			add(10, "viewport_larger_than_screen")
		}
		if len(env.Languages) == 0 {
			add(10, "no_languages")
		}
		if env.Plugins == 0 && env.TouchPoints == 0 && isDesktopChromeUA(env.UA) {
			add(10, "desktop_chrome_without_plugins")
		}
		if env.Cores == 0 && env.UA != "" {
			add(5, "no_hardware_concurrency")
		}
	}

	// ---------- PoW ----------
	if in.PowExpected && !in.PowProvided {
		add(15, "pow_missing")
	}

	// ---------- 行为轨迹 ----------
	if in.Trace == nil {
		if drag {
			add(30, "trace_missing")
		} else {
			add(20, "trace_missing")
		}
	} else {
		trace := in.Trace
		if trace.Untrusted > 0 {
			add(35, "untrusted_events")
		}
		if drag {
			if features.Points < 6 {
				add(25, "too_few_points")
			}
			if features.DurationMs > 0 && features.DurationMs < 150 {
				add(25, "drag_too_fast")
			}
			if features.Points >= 10 {
				switch {
				case features.Straightness > 0 && features.Straightness < 1.01:
					add(25, "perfectly_straight")
				case features.Straightness > 0 && features.Straightness < 1.05:
					add(10, "nearly_straight")
				}
				if features.SpeedMean > 0 && features.SpeedCV < 0.08 {
					add(20, "constant_speed")
				}
				if features.DtMean > 0 && features.DtStd < 0.5 {
					add(15, "uniform_sampling")
				}
			}
			if features.Points >= 15 && features.DirectionChanges == 0 && features.Pauses == 0 {
				add(10, "no_hesitation")
			}
			// 人类特征
			if features.Overshoot {
				add(-10, "overshoot_correction")
			}
			if features.Pauses >= 1 {
				add(-5, "pauses")
			}
			if features.SpeedCV > 0.3 {
				add(-5, "variable_speed")
			}
			if features.DirectionChanges >= 2 {
				add(-5, "direction_changes")
			}
		} else {
			if features.ClickCount == 0 {
				add(20, "no_clicks_recorded")
			}
			if features.ClickCount >= 3 && features.ClickIntervalStd < 8 {
				add(25, "uniform_click_intervals")
			}
			if features.ClickCount >= 2 && features.MinClickInterval < 60 {
				add(15, "click_interval_too_short")
			}
			if features.Points < 3 && features.ClickCount > 0 {
				add(15, "clicks_without_movement")
			}
		}
		if features.InteractionMs > 0 && features.InteractionMs < 100 {
			add(20, "interaction_too_short")
		}
		if trace.StartedAt > 0 && trace.StartedAt < 80 {
			add(10, "no_reaction_time")
		}
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return Result{Score: score, Reasons: reasons, Features: &features}
}

func isDesktopChromeUA(ua string) bool {
	lower := strings.ToLower(ua)
	if !strings.Contains(lower, "chrome/") {
		return false
	}
	for _, mobile := range []string{"mobile", "android", "iphone", "ipad"} {
		if strings.Contains(lower, mobile) {
			return false
		}
	}
	return true
}
