package services

import (
	"context"
	"encoding/json"
	"log"
	"simple-captcha/cache"
	"simple-captcha/config"
	"strconv"
	"time"
)

const (
	clientRiskKeyPrefix = "captcha:client:"
	telemetryLogKey     = "captcha:telemetry"
)

// ClientRisk 单个客户端（IP + UA 哈希）在观察窗口内的风控状态
type ClientRisk struct {
	Fails     int   `json:"fails"`
	Successes int   `json:"successes"`
	LastScore int   `json:"lastScore"`
	UpdatedAt int64 `json:"updatedAt"`
}

// ClientRiskStore 客户端风控状态存储（Redis），所有操作 fail-open：Redis 异常时不影响主流程
type ClientRiskStore struct {
	cfg *config.Config
}

// NewClientRiskStore 创建客户端风控状态存储
func NewClientRiskStore() *ClientRiskStore {
	return &ClientRiskStore{cfg: config.LoadConfig()}
}

func (s *ClientRiskStore) stateKey(clientHash string) string {
	return clientRiskKeyPrefix + clientHash
}

// Get 读取客户端状态，不存在或出错时返回零值
func (s *ClientRiskStore) Get(clientHash string) ClientRisk {
	state := ClientRisk{}
	if clientHash == "" {
		return state
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	values, err := cache.GetRedisClient().HGetAll(ctx, s.stateKey(clientHash)).Result()
	if err != nil {
		log.Printf("读取客户端风控状态失败: client=%s, error=%v", clientHash, err)
		return state
	}

	state.Fails = atoi(values["fails"])
	state.Successes = atoi(values["successes"])
	state.LastScore = atoi(values["lastScore"])
	state.UpdatedAt = int64(atoi(values["updatedAt"]))
	return state
}

// Record 记录一次校验结果。失败累加失败计数，成功则让失败计数衰减一次。
func (s *ClientRiskStore) Record(clientHash string, success bool, score int) {
	if clientHash == "" {
		return
	}

	current := s.Get(clientHash)
	if success {
		current.Successes++
		if current.Fails > 0 {
			current.Fails--
		}
	} else {
		current.Fails++
	}
	current.LastScore = score
	current.UpdatedAt = time.Now().Unix()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	key := s.stateKey(clientHash)
	pipe := cache.GetRedisClient().TxPipeline()
	pipe.HSet(ctx, key, map[string]interface{}{
		"fails":     current.Fails,
		"successes": current.Successes,
		"lastScore": current.LastScore,
		"updatedAt": current.UpdatedAt,
	})
	pipe.Expire(ctx, key, s.cfg.Risk.StateTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("写入客户端风控状态失败: client=%s, error=%v", clientHash, err)
	}
}

// LevelFor 根据基础难度与客户端状态计算下一题的难度等级
func (s *ClientRiskStore) LevelFor(clientHash string) int {
	level := s.cfg.Captcha.DefaultDifficulty
	if clientHash == "" {
		return ClampDifficulty(level)
	}

	state := s.Get(clientHash)
	return LevelFromState(level, state, s.cfg.Risk)
}

// LevelFromState 纯函数：基础难度 + 近期失败次数 + 上次风险分 → 难度等级
func LevelFromState(base int, state ClientRisk, riskCfg config.RiskConfig) int {
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

// LogTelemetry 把一条遥测记录追加到 Redis 列表，保留最近 N 条供离线分析
func (s *ClientRiskStore) LogTelemetry(entry interface{}) {
	size := s.cfg.Risk.TelemetryLogSize
	if size <= 0 {
		return
	}

	payload, err := json.Marshal(entry)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pipe := cache.GetRedisClient().TxPipeline()
	pipe.LPush(ctx, telemetryLogKey, payload)
	pipe.LTrim(ctx, telemetryLogKey, 0, int64(size-1))
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("写入遥测日志失败: error=%v", err)
	}
}

func atoi(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}
