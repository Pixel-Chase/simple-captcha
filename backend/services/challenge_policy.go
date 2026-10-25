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

// DifficultyFor 计算该客户端下一题的难度等级
func (p *ChallengePolicy) DifficultyFor(clientHash string) int {
	if p == nil {
		return 3
	}
	if p.store == nil || clientHash == "" {
		return ClampDifficulty(p.cfg.Captcha.DefaultDifficulty)
	}
	return p.store.LevelFor(clientHash)
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
