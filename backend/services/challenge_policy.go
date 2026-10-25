package services

import (
	"math/rand"
	"simple-captcha/config"
)

// ChallengePolicy 出题策略：根据客户端风控状态决定难度等级，并为 auto 类型挑选具体题型
type ChallengePolicy struct {
	cfg   *config.Config
	store *ClientRiskStore
	pick  func(n int) int
}

// NewChallengePolicy 创建出题策略
func NewChallengePolicy(store *ClientRiskStore) *ChallengePolicy {
	return &ChallengePolicy{
		cfg:   config.LoadConfig(),
		store: store,
		pick:  rand.Intn,
	}
}

// DifficultyFor 计算该客户端下一题的难度等级：先按客户端历史表现定级，
// 再按其出口 IP 最近一分钟的请求数（ipRequests）加难
func (p *ChallengePolicy) DifficultyFor(clientHash string, ipRequests int) int {
	if p == nil {
		return 3
	}
	level := p.cfg.Captcha.DefaultDifficulty
	if p.store != nil && clientHash != "" {
		level = p.store.LevelFor(clientHash)
	}
	return ClampDifficulty(level + IPPressureBoost(ipRequests, p.cfg.Captcha.IPEscalatePerMin))
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
