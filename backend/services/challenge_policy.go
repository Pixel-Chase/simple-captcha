package services

import (
	"fmt"
	"math/rand"
	"simple-captcha/risk"
)

// 出题前按环境分加难的阈值与分值。
// 这里的阈值作用于出题前的环境 / 请求侧得分，与策略 risk.escalateThreshold（作用于上一次校验的总分）语义不同：
// 单个明显痕迹（webdriver 45、automation 35、指纹重放 30）即可加 1 级，两个以上叠加加 2 级。
const (
	issueBoostOneAt = 30
	issueBoostTwoAt = 55
	// fpReplayPoints 同一指纹在时间窗内被多个客户端复用的分值，单独命中即加 1 级
	fpReplayPoints = 30
)

// IssueInput 出题前的难度决策输入
type IssueInput struct {
	ClientHash string
	// IPRequests 出口 IP 最近一分钟的请求数
	IPRequests int
	// Env 客户端自报的环境信号，旧版 SDK 或 telemetry=off 时为 nil
	Env *risk.Env
	// Request 服务端从请求头提取的信号，可为 nil
	Request *risk.RequestSignals
	// FPReplays 该指纹在时间窗内对应的不同客户端数，0 表示未统计；超过策略 risk.fpReplayThreshold 视为重放
	FPReplays int
	// State 客户端历史状态；nil 时由策略按 ClientHash 从存储读取
	State *ClientRisk
}

// IssueDecision 出题前的难度决策结果
type IssueDecision struct {
	// Level 最终难度等级，永不低于 Base
	Level int
	// Base 配置的默认难度
	Base int
	// Boost 环境 / 请求侧信号带来的加难级数（0-2）
	Boost int
	// EnvScore 环境 / 请求侧得分（已按上限裁剪）
	EnvScore int
	// Consistent 请求头齐全且与自报环境一致，仅记录不参与决策；无请求侧信号时为 false
	Consistent bool
	// Reasons 本次加难的全部原因，格式 name(+级数或分值)
	Reasons []string
}

// ChallengePolicy 出题策略：根据客户端风控状态与环境信号决定难度等级，并为 auto 类型挑选具体题型
type ChallengePolicy struct {
	settings settingsSource
	store    *ClientRiskStore
	pick     func(n int) int
}

// NewChallengePolicy 创建出题策略；策略值每次出题时从 config.Runtime() 读取
func NewChallengePolicy(store *ClientRiskStore) *ChallengePolicy {
	return newChallengePolicyWithSettings(nil, store)
}

func newChallengePolicyWithSettings(settings settingsSource, store *ClientRiskStore) *ChallengePolicy {
	return &ChallengePolicy{
		settings: settings,
		store:    store,
		pick:     rand.Intn,
	}
}

// Decide 计算该客户端下一题的难度等级。难度只会在默认值之上向上调整：
//  1. 按客户端历史表现（近期失败次数、上次风险分）升级；
//  2. 按出口 IP 最近一分钟的请求数加难；
//  3. 按环境 / 请求侧得分加难：>= issueBoostOneAt 加 1 级，>= issueBoostTwoAt 加 2 级。
//
// 没有任何降级分支：真人不需要证明自己就拿默认难度，脚本因为忘了藏的痕迹逐级变难。
func (p *ChallengePolicy) Decide(in IssueInput) IssueDecision {
	if p == nil {
		return IssueDecision{Level: ClampDifficulty(0), Base: ClampDifficulty(0)}
	}
	rt := p.settings.get()
	base := ClampDifficulty(rt.Captcha.DefaultDifficulty)
	decision := IssueDecision{Base: base}
	reasons := make([]string, 0, 8)

	state := in.State
	if state == nil {
		loaded := ClientRisk{}
		if p.store != nil && in.ClientHash != "" {
			loaded = p.store.Get(in.ClientHash)
		}
		state = &loaded
	}

	level := LevelFromState(base, *state, rt.Risk)
	if up := level - base; up > 0 {
		reasons = append(reasons, fmt.Sprintf("client_history(%+d)", up))
	}
	if ip := IPPressureBoost(in.IPRequests, rt.Captcha.IPEscalatePerMinute); ip > 0 {
		level += ip
		reasons = append(reasons, fmt.Sprintf("ip_pressure(%+d)", ip))
	}

	if rt.Risk.Enabled && (in.Env != nil || in.Request != nil) {
		env := risk.EvaluateEnv(in.Env, in.Request)
		reasons = append(reasons, env.Reasons...)
		decision.Consistent = env.Consistent
		envScore := env.Score
		if threshold := rt.Risk.FPReplayThreshold; threshold > 0 && in.FPReplays > threshold {
			envScore += fpReplayPoints
			reasons = append(reasons, fmt.Sprintf("fp_replayed(%+d)", fpReplayPoints))
		}
		if cap := rt.Risk.EnvScoreCap; cap > 0 && envScore > cap {
			envScore = cap
		}
		decision.EnvScore = envScore
		switch {
		case envScore >= issueBoostTwoAt:
			decision.Boost = 2
		case envScore >= issueBoostOneAt:
			decision.Boost = 1
		}
		if decision.Boost > 0 {
			reasons = append(reasons, fmt.Sprintf("env_boost(%+d)", decision.Boost))
			level += decision.Boost
		}
	}

	decision.Level = ClampDifficulty(level)
	decision.Reasons = reasons
	return decision
}

// IPPressureBoost 纯函数：出口 IP 每分钟请求数超过阈值 1 倍加 1 级、超过 2 倍加 2 级；阈值 <= 0 时不加难。
// IP 只调难度不做拒绝：共用出口的正常用户最多是题目变难、PoW 多算一会儿，批量脚本的成本则随请求量上升
func IPPressureBoost(requests, threshold int) int {
	if threshold <= 0 {
		return 0
	}
	switch {
	case requests > 2*threshold:
		return 2
	case requests > threshold:
		return 1
	default:
		return 0
	}
}

// IPPressureSaturation 计数达到该值时 IPPressureBoost 已到顶，窗口无需再记录更多请求；阈值 <= 0 时不计数
func IPPressureSaturation(threshold int) int {
	if threshold <= 0 {
		return 0
	}
	return 2*threshold + 1
}

// ResolveType 把请求类型解析为具体题型；非 auto 原样返回
func (p *ChallengePolicy) ResolveType(requested string, level int) string {
	if requested != "auto" {
		return requested
	}
	pick := rand.Intn
	if p != nil && p.pick != nil {
		pick = p.pick
	}
	return ChooseAutoType(level, pick)
}

// ChooseAutoType 按难度等级从候选题型中随机挑选：
// 低风险客户端可以得到更顺手的滑块/旋转，高风险客户端只给自动化成本最高的点选类题型。
func ChooseAutoType(level int, pick func(n int) int) string {
	var pool []string
	switch {
	case level <= 2:
		pool = []string{"slide-text", "rotate", "click-shape", "click-text"}
	case level == 3:
		pool = []string{"click-text", "click-shape", "rotate"}
	default:
		pool = []string{"click-text", "click-shape"}
	}
	idx := pick(len(pool))
	if idx < 0 || idx >= len(pool) {
		idx = 0
	}
	return pool[idx]
}
