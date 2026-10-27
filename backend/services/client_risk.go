package services

import (
	"simple-captcha/cache"
	"simple-captcha/config"
	"time"
)

// ClientRisk 单个客户端（IP + UA 哈希）在观察窗口内的风控状态
type ClientRisk struct {
	Fails     int   `json:"fails"`
	Successes int   `json:"successes"`
	LastScore int   `json:"lastScore"`
	UpdatedAt int64 `json:"updatedAt"`
}

// ClientRiskStore 客户端风控状态与指纹集合（进程内存，原子更新）
type ClientRiskStore struct {
	settings settingsSource
	store    *cache.Memory
}

// NewClientRiskStore 创建客户端风控状态存储；各处创建的实例共用同一份进程内存数据
func NewClientRiskStore() *ClientRiskStore {
	return &ClientRiskStore{store: cache.Shared()}
}

// Get 读取客户端状态，不存在时返回零值
func (s *ClientRiskStore) Get(clientHash string) ClientRisk {
	if clientHash == "" {
		return ClientRisk{}
	}
	state, _ := s.store.GetClient(clientHash)
	return ClientRisk{
		Fails:     state.Fails,
		Successes: state.Successes,
		LastScore: state.LastScore,
		UpdatedAt: state.UpdatedAt,
	}
}

// Record 记录一次校验结果。失败累加失败计数，成功则让失败计数衰减一次（下限 0）；
// 读改写在同一把锁内完成，并发失败不会互相覆盖而低估失败次数
func (s *ClientRiskStore) Record(clientHash string, success bool, score int) {
	if clientHash == "" {
		return
	}
	ttl := s.settings.get().Risk.StateTTL()
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	s.store.RecordClient(clientHash, success, score, ttl)
}

// LevelFromState 纯函数：基础难度 + 近期失败次数 + 上次风险分 → 难度等级
func LevelFromState(base int, state ClientRisk, riskCfg config.RiskSettings) int {
	level := base
	if state.Fails >= 2 {
		level++
	}
	if state.Fails >= 4 {
		level++
	}
	if riskCfg.Enabled {
		if state.LastScore >= riskCfg.EscalateThreshold {
			level++
		}
		if state.LastScore >= riskCfg.BlockThreshold {
			level++
		}
	}
	return ClampDifficulty(level)
}

// TouchFingerprint 记录一次出题中 fp 与客户端的对应关系，返回时间窗内该指纹对应的不同客户端数。
// 同一客户端重复出题不增加计数；阈值配置为 0 或参数为空时不记录并返回 0。
// 集合大小达到"阈值 + 1"后不再写入新成员（持续重放也不会把集合撑大），但新客户端仍按"超过阈值"报告
func (s *ClientRiskStore) TouchFingerprint(fp, clientHash string) int {
	rt := s.settings.get()
	threshold := rt.Risk.FPReplayThreshold
	if threshold <= 0 || fp == "" || clientHash == "" {
		return 0
	}
	window := rt.Risk.FPReplayWindow()
	if window <= 0 {
		window = 10 * time.Minute
	}
	return s.store.TouchFingerprint(fp, clientHash, threshold+1, window)
}

// RecentTelemetry 读取最近 limit 条风控观察记录（新的在前），limit 裁剪到 [1, MaxRecentTelemetry]
func (s *ClientRiskStore) RecentTelemetry(limit int) ([]map[string]interface{}, error) {
	t := sharedTelemetry.Load()
	if t == nil {
		return []map[string]interface{}{}, nil
	}
	return t.Recent(limit)
}

// LogTelemetry 追加一条风控观察记录（异步写入 SQLite，按条数保留最近 N 条）
func (s *ClientRiskStore) LogTelemetry(entry interface{}) {
	sharedTelemetry.Load().Log(entry)
}
