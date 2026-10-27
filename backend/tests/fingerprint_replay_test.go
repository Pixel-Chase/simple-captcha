package tests

import (
	"context"
	"fmt"
	"net/http"
	"simple-captcha/cache"
	"simple-captcha/services"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// issueWithFingerprint 以 UA 派生的出口 IP 与指定 fp 发一次 POST 出题，返回响应体
func issueWithFingerprint(t *testing.T, ua, fp string) map[string]interface{} {
	env := cleanIssueEnvFor(ua)
	env.Fingerprint = fp
	req := withBrowserHeaders(rawIssueRequest(t, http.MethodPost, "slide-text", map[string]interface{}{"dpr": 2, "env": env}), ua)
	req.RemoteAddr = remoteAddrFor(ua)
	return issueBody(t, serve(req))
}

func TestFingerprintReplay_DistinctClientsEscalate(t *testing.T) {
	cfg := skipWithoutRisk(t)
	threshold := cfg.Risk.FPReplayThreshold
	if threshold <= 0 {
		t.Skip("RISK_FP_REPLAY_THRESHOLD=0")
	}
	base := services.ClampDifficulty(cfg.Captcha.DefaultDifficulty)
	fp := fmt.Sprintf("replay%026d", time.Now().UnixNano()%1_000_000_000)
	rdb := cache.GetRedisClient()
	t.Cleanup(func() { rdb.Del(context.Background(), "captcha:fp:"+fp) })

	// 前 threshold 个不同客户端：同一份 env 换着 UA 重放，还在阈值内，难度不变
	for i := 1; i <= threshold; i++ {
		body := issueWithFingerprint(t, uniqueUA(fmt.Sprintf("replay-%d", i)), fp)
		assert.Equal(t, base, int(body["difficulty"].(float64)), "第 %d 个客户端仍在阈值内", i)
	}

	// 第 threshold+1 个客户端起视为重放：加 1 级并记录原因
	body := issueWithFingerprint(t, uniqueUA("replay-over"), fp)
	assert.Equal(t, services.ClampDifficulty(base+1), int(body["difficulty"].(float64)), "超过阈值的客户端应加 1 级")
	if cfg.Risk.TelemetryLogSize > 0 {
		assert.Contains(t, fmt.Sprint(telemetryEntryFor(t, body["key"].(string))["reasons"]), "fp_replayed(+30)")
	}

	// 饱和截断：集合里最多保留 threshold+1 个成员，再多客户端也不写入
	for i := 0; i < 5; i++ {
		issueWithFingerprint(t, uniqueUA(fmt.Sprintf("replay-flood-%d", i)), fp)
	}
	assert.EqualValues(t, threshold+1, rdb.SCard(context.Background(), "captcha:fp:"+fp).Val())
}

func TestFingerprintReplay_SameClientRepeatsStayDefault(t *testing.T) {
	cfg := skipWithoutRisk(t)
	if cfg.Risk.FPReplayThreshold <= 0 {
		t.Skip("RISK_FP_REPLAY_THRESHOLD=0")
	}
	base := services.ClampDifficulty(cfg.Captcha.DefaultDifficulty)
	fp := fmt.Sprintf("same%028d", time.Now().UnixNano()%1_000_000_000)
	rdb := cache.GetRedisClient()
	t.Cleanup(func() { rdb.Del(context.Background(), "captcha:fp:"+fp) })

	ua := uniqueUA("replay-same-client")
	for i := 0; i < 20; i++ {
		body := issueWithFingerprint(t, ua, fp)
		require.Equal(t, base, int(body["difficulty"].(float64)), "同一客户端第 %d 次出题不应被判为重放", i+1)
	}
	assert.EqualValues(t, 1, rdb.SCard(context.Background(), "captcha:fp:"+fp).Val(), "同一客户端只计 1 个成员")
	ttl := rdb.PTTL(context.Background(), "captcha:fp:"+fp).Val()
	assert.Greater(t, ttl, time.Duration(0))
	assert.LessOrEqual(t, ttl, cfg.Risk.FPReplayWindow)
}

func TestFingerprintReplay_NotCountedWithoutFingerprint(t *testing.T) {
	cfg := skipWithoutRisk(t)
	if cfg.Risk.FPReplayThreshold <= 0 {
		t.Skip("RISK_FP_REPLAY_THRESHOLD=0")
	}
	ua := uniqueUA("replay-nofp")
	env := cleanIssueEnvFor(ua)
	env.Fingerprint = ""
	before := len(cache.GetRedisClient().Keys(context.Background(), "captcha:fp:*").Val())
	issueBody(t, issueRequest(t, http.MethodPost, "slide-text", map[string]interface{}{"dpr": 2, "env": env}, ua))
	after := len(cache.GetRedisClient().Keys(context.Background(), "captcha:fp:*").Val())
	assert.Equal(t, before, after, "没有 fp 的出题不应新增指纹集合")
}
