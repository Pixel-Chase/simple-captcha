package services

import (
	"context"
	"encoding/json"
	"log"
	"simple-captcha/cache"
	"simple-captcha/config"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

const (
	clientRiskKeyPrefix  = "captcha:client:"
	fingerprintKeyPrefix = "captcha:fp:"
	telemetryLogKey      = "captcha:telemetry"
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

// recordScript 在 Redis 端原子更新计数器：失败 fails+1；成功 successes+1 且 fails 衰减 1（下限 0）。
// 之前"先 HGETALL 再 HSET"的读改写在并发失败时会互相覆盖，批量脚本的失败次数被严重低估，
// 难度升级与拦截都会因此延迟触发。
var recordScript = redis.NewScript(`
local key = KEYS[1]
local fails
if ARGV[1] == "1" then
  redis.call("HINCRBY", key, "successes", 1)
  fails = redis.call("HINCRBY", key, "fails", -1)
  if fails < 0 then
    redis.call("HSET", key, "fails", 0)
    fails = 0
  end
else
  fails = redis.call("HINCRBY", key, "fails", 1)
end
redis.call("HSET", key, "lastScore", ARGV[2], "updatedAt", ARGV[3])
redis.call("PEXPIRE", key, ARGV[4])
return fails
`)

// Record 记录一次校验结果。失败累加失败计数，成功则让失败计数衰减一次。
func (s *ClientRiskStore) Record(clientHash string, success bool, score int) {
	if clientHash == "" {
		return
	}

	successFlag := 0
	if success {
		successFlag = 1
	}
	ttl := s.cfg.Risk.StateTTL
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := recordScript.Run(ctx, cache.GetRedisClient(), []string{s.stateKey(clientHash)},
		successFlag, score, time.Now().Unix(), ttl.Milliseconds()).Err()
	if err != nil {
		log.Printf("写入客户端风控状态失败: client=%s, error=%v", clientHash, err)
	}
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

// fingerprintScript 原子记录指纹 -> 客户端集合：
// KEYS[1] 指纹 key；ARGV[1] 客户端哈希；ARGV[2] 饱和上限（阈值 + 1）；ARGV[3] 过期毫秒数。
// 集合大小达到饱和上限后不再写入新成员（持续重放也不会把集合撑大），但返回值仍按"超过阈值"报告。
var fingerprintScript = redis.NewScript(`
local key = KEYS[1]
local limit = tonumber(ARGV[2])
local count = redis.call("SCARD", key)
if count < limit then
  redis.call("SADD", key, ARGV[1])
  count = redis.call("SCARD", key)
elseif redis.call("SISMEMBER", key, ARGV[1]) == 0 then
  count = count + 1
end
redis.call("PEXPIRE", key, ARGV[3])
return count
`)

// TouchFingerprint 记录一次出题中 fp 与客户端的对应关系，返回时间窗内该指纹对应的不同客户端数。
// 同一客户端重复出题不增加计数；阈值配置为 0 或参数为空时不读写 Redis 并返回 0；Redis 出错时 fail-open 返回 0
func (s *ClientRiskStore) TouchFingerprint(fp, clientHash string) int {
	return s.touchFingerprint(cache.GetRedisClient(), fp, clientHash)
}

func (s *ClientRiskStore) touchFingerprint(client redis.Scripter, fp, clientHash string) int {
	threshold := s.cfg.Risk.FPReplayThreshold
	if threshold <= 0 || fp == "" || clientHash == "" || client == nil {
		return 0
	}
	window := s.cfg.Risk.FPReplayWindow
	if window <= 0 {
		window = 10 * time.Minute
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	count, err := fingerprintScript.Run(ctx, client, []string{fingerprintKeyPrefix + fp},
		clientHash, threshold+1, window.Milliseconds()).Int()
	if err != nil {
		log.Printf("记录指纹重放计数失败，本次按未重放处理: fp=%s, error=%v", fp, err)
		return 0
	}
	return count
}

// MaxRecentTelemetry 管理接口一次最多返回的遥测记录条数
const MaxRecentTelemetry = 500

// RecentTelemetry 读取最近 limit 条遥测记录（新的在前），无法解析的条目跳过；limit 裁剪到 [1, MaxRecentTelemetry]
func (s *ClientRiskStore) RecentTelemetry(limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > MaxRecentTelemetry {
		limit = MaxRecentTelemetry
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	raw, err := cache.GetRedisClient().LRange(ctx, telemetryLogKey, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, err
	}
	items := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		var entry map[string]interface{}
		if json.Unmarshal([]byte(item), &entry) != nil {
			continue
		}
		items = append(items, entry)
	}
	return items, nil
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
