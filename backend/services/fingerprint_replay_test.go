package services

import (
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
)

// TestTouchFingerprint_FailOpen Redis 不可达时按未重放处理：返回 0、不 panic、不阻塞主流程
func TestTouchFingerprint_FailOpen(t *testing.T) {
	store := &ClientRiskStore{cfg: testPolicyConfig()}
	broken := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 200 * time.Millisecond, MaxRetries: 0})
	defer broken.Close()

	start := time.Now()
	if got := store.touchFingerprint(broken, "fp-broken", "client-a"); got != 0 {
		t.Errorf("unreachable redis must fail open with 0, got %d", got)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("fail-open should return quickly, took %s", elapsed)
	}
}

func TestTouchFingerprint_DisabledOrEmptyInputsSkipRedis(t *testing.T) {
	store := &ClientRiskStore{cfg: testPolicyConfig()}
	// client 为 nil：任何真正的 Redis 调用都会 panic，返回 0 说明没有触达 Redis
	if got := store.touchFingerprint(nil, "fp", "client"); got != 0 {
		t.Errorf("nil client should return 0, got %d", got)
	}

	disabled := testPolicyConfig()
	disabled.Risk.FPReplayThreshold = 0
	store = &ClientRiskStore{cfg: disabled}
	broken := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 100 * time.Millisecond, MaxRetries: 0})
	defer broken.Close()
	if got := store.touchFingerprint(broken, "fp", "client"); got != 0 {
		t.Errorf("threshold 0 must skip redis and return 0, got %d", got)
	}
	if got := store.touchFingerprint(broken, "", "client"); got != 0 {
		t.Errorf("empty fp must return 0, got %d", got)
	}
}
