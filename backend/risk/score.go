package risk

import (
	"fmt"
	"strings"
)

// 头less / 软件渲染器特征，常见于无头浏览器与容器环境
var headlessRenderers = []string{"swiftshader", "llvmpipe", "mesa offscreen", "softpipe"}

// anomalyWeight SDK 上报的异常标记对应的分值；hard 表示几乎只有自动化工具或篡改才会出现
type anomalyWeight struct {
	points int
	hard   bool
}

// anomalyWeights 已知标记的分值表；未知标记忽略，只随遥测记录
var anomalyWeights = map[string]anomalyWeight{
	"zero_outer_size":          {points: 20},
	"permission_mismatch":      {points: 15},
	"no_media_devices":         {points: 10},
	"chrome_object_missing":    {points: 10},
	"worker_ua_mismatch":       {points: 30, hard: true},
	"worker_cores_mismatch":    {points: 30, hard: true},
	"worker_platform_mismatch": {points: 30, hard: true},
}

// getterPatchedPrefix navigator 关键属性的 getter 被改写，无论改了几个属性只计一次
const getterPatchedPrefix = "getter_patched:"

// scoreAnomalies 把 SDK 上报的异常标记映射为分值；重复标记只计一次。
// softwareRenderer 为真（VDI、虚拟机）时跳过 no_media_devices：虚拟桌面本来就没有摄像头与麦克风，
// 两条规则指向同一个正常原因，叠加会把 VDI 用户推过加难阈值
func scoreAnomalies(s *scorer, anomalies []string, softwareRenderer bool) {
	seen := make(map[string]bool, len(anomalies))
	getterPatched := false
	for _, marker := range anomalies {
		if strings.HasPrefix(marker, getterPatchedPrefix) {
			if !getterPatched {
				getterPatched = true
				s.addHard(30, "getter_patched")
			}
			continue
		}
		if seen[marker] {
			continue
		}
		seen[marker] = true
		if marker == "no_media_devices" && softwareRenderer {
			continue
		}
		if w, ok := anomalyWeights[marker]; ok {
			if w.hard {
				s.addHard(w.points, marker)
			} else {
				s.add(w.points, marker)
			}
		}
	}
}

// scorer 累加分值与原因，原因格式固定为 name(+分值)，便于日志与遥测对照
type scorer struct {
	score   int
	reasons []string
	hard    bool
}

func (s *scorer) add(points int, reason string) {
	s.score += points
	s.reasons = append(s.reasons, fmt.Sprintf("%s(%+d)", reason, points))
}

// addHard 记录硬性自动化标记：几乎只有自动化工具才会留下的痕迹，允许单独支撑拦截
func (s *scorer) addHard(points int, reason string) {
	s.add(points, reason)
	s.hard = true
}

// EvaluateEnv 对环境与请求侧信号打分。env、req 均可为 nil，两者都为 nil 时返回零值。
// 这里不计 env_missing：telemetry=off 时出题不带 env 是正常情况，只有校验阶段才把缺失视为疑点。
func EvaluateEnv(env *Env, req *RequestSignals) EnvResult {
	s := &scorer{}
	softwareRenderer := false
	if env != nil {
		if env.Webdriver {
			s.addHard(45, "webdriver")
		}
		if len(env.Automation) > 0 {
			s.addHard(35, "automation_markers")
		}
		if renderer := strings.ToLower(env.WebGL); renderer != "" {
			for _, marker := range headlessRenderers {
				if strings.Contains(renderer, marker) {
					softwareRenderer = true
					s.add(25, "software_renderer")
					break
				}
			}
		}
		if env.Viewport[0] > env.Screen[0]+1 && env.Screen[0] > 0 ||
			env.Viewport[1] > env.Screen[1]+1 && env.Screen[1] > 0 {
			s.add(10, "viewport_larger_than_screen")
		}
		if len(env.Languages) == 0 {
			s.add(10, "no_languages")
		}
		if env.Plugins == 0 && env.TouchPoints == 0 && isDesktopChromeUA(env.UA) {
			s.add(10, "desktop_chrome_without_plugins")
		}
		if env.Cores == 0 && env.UA != "" {
			s.add(5, "no_hardware_concurrency")
		}
		scoreAnomalies(s, env.Anomalies, softwareRenderer)
	}
	result := EnvResult{}
	if req != nil {
		// 请求头核验：不像浏览器或与自报环境矛盾的请求按分值并入环境侧，受同一上限约束
		r := EvaluateRequest(req, env)
		s.score += r.Score
		s.reasons = append(s.reasons, r.Reasons...)
		if r.HardMarker {
			s.hard = true
		}
		result.Consistent = r.Consistent
		result.RequestReasons = r.Reasons
	}
	result.Score = s.score
	result.Reasons = s.reasons
	result.HardMarker = s.hard
	return result
}

// EvaluateBehavior 对交互轨迹与 PoW 打分。trace 为 nil 时记 trace_missing。
// 规则按"叠加扣分 + 少量人类特征加分"计算，结果不裁剪，人类特征明显时可为负。
func EvaluateBehavior(captchaType string, trace *Trace, powExpected, powProvided bool) BehaviorResult {
	s := &scorer{}
	drag := IsDragType(captchaType)
	features := Extract(trace)

	if powExpected && !powProvided {
		s.add(15, "pow_missing")
	}

	if trace == nil {
		if drag {
			s.add(30, "trace_missing")
		} else {
			s.add(20, "trace_missing")
		}
		return BehaviorResult{Score: s.score, Reasons: s.reasons, Features: &features}
	}

	if trace.Untrusted > 0 {
		s.addHard(35, "untrusted_events")
	}
	if drag {
		if features.Points < 6 {
			s.add(25, "too_few_points")
		}
		if features.DurationMs > 0 && features.DurationMs < 150 {
			s.add(25, "drag_too_fast")
		}
		if features.Points >= 10 {
			switch {
			case features.Straightness > 0 && features.Straightness < 1.01:
				s.add(25, "perfectly_straight")
			case features.Straightness > 0 && features.Straightness < 1.05:
				s.add(10, "nearly_straight")
			}
			if features.SpeedMean > 0 && features.SpeedCV < 0.08 {
				s.add(20, "constant_speed")
			}
			if features.DtMean > 0 && features.DtStd < 0.5 {
				s.add(15, "uniform_sampling")
			}
		}
		if features.Points >= 15 && features.DirectionChanges == 0 && features.Pauses == 0 {
			s.add(10, "no_hesitation")
		}
		// 人类特征
		if features.Overshoot {
			s.add(-10, "overshoot_correction")
		}
		if features.Pauses >= 1 {
			s.add(-5, "pauses")
		}
		if features.SpeedCV > 0.3 {
			s.add(-5, "variable_speed")
		}
		if features.DirectionChanges >= 2 {
			s.add(-5, "direction_changes")
		}
	} else {
		if features.ClickCount == 0 {
			s.add(20, "no_clicks_recorded")
		}
		if features.ClickCount >= 3 && features.ClickIntervalStd < 8 {
			s.add(25, "uniform_click_intervals")
		}
		if features.ClickCount >= 2 && features.MinClickInterval < 60 {
			s.add(15, "click_interval_too_short")
		}
		if features.Points < 3 && features.ClickCount > 0 {
			s.add(15, "clicks_without_movement")
		}
	}
	if features.InteractionMs > 0 && features.InteractionMs < 100 {
		s.add(20, "interaction_too_short")
	}
	if trace.StartedAt > 0 && trace.StartedAt < 80 {
		s.add(10, "no_reaction_time")
	}

	return BehaviorResult{Score: s.score, Reasons: s.reasons, HardMarker: s.hard, Features: &features}
}

// Evaluate 对一次校验行为打分：0 表示很像人类，100 表示几乎可以确定是脚本。
// 环境侧与行为侧分开计算：环境侧按 EnvScoreCap 裁剪后与行为侧相加，再裁剪到 [0, 100]。
func Evaluate(in Input) Result {
	env := EvaluateEnv(in.Env, in.Request)
	envScore := env.Score
	envReasons := env.Reasons
	if in.Env == nil {
		envScore += 10
		envReasons = append(envReasons, "env_missing(+10)")
	} else if in.Trace != nil && in.Trace.PointerType == "touch" && in.Env.TouchPoints == 0 {
		// 跨环境与轨迹的规则：声称触控操作但设备没有触摸屏
		envScore += 20
		envReasons = append(envReasons, "touch_without_touchscreen(+20)")
	}
	if in.EnvScoreCap > 0 && envScore > in.EnvScoreCap {
		envScore = in.EnvScoreCap
	}

	behavior := EvaluateBehavior(in.Type, in.Trace, in.PowExpected, in.PowProvided)

	score := envScore + behavior.Score
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	reasons := make([]string, 0, len(envReasons)+len(behavior.Reasons))
	reasons = append(reasons, envReasons...)
	reasons = append(reasons, behavior.Reasons...)

	return Result{
		Score:          score,
		Reasons:        reasons,
		Features:       behavior.Features,
		EnvScore:       envScore,
		BehaviorScore:  behavior.Score,
		HardMarker:     env.HardMarker || behavior.HardMarker,
		Consistent:     env.Consistent,
		RequestReasons: env.RequestReasons,
	}
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
