package services

import (
	"fmt"
	"simple-captcha/cache"
	"testing"
	"time"
)

func TestTouchFingerprint_DisabledOrEmptyInputsSkipStore(t *testing.T) {
	mem := cache.NewMemory(cache.WithoutSweeper())
	defer mem.Stop()

	disabled := testPolicyConfig()
	disabled.Risk.FPReplayThreshold = 0
	store := &ClientRiskStore{settings: fixedSettings(disabled), store: mem}
	if got := store.TouchFingerprint("fp", "client"); got != 0 {
		t.Errorf("threshold 0 must skip store and return 0, got %d", got)
	}

	store = &ClientRiskStore{settings: fixedSettings(testPolicyConfig()), store: mem}
	if got := store.TouchFingerprint("", "client"); got != 0 {
		t.Errorf("empty fp must return 0, got %d", got)
	}
	if got := store.TouchFingerprint("fp", ""); got != 0 {
		t.Errorf("empty client must return 0, got %d", got)
	}
	if n := mem.Len(cache.KindFingerprint); n != 0 {
		t.Errorf("skipped calls must not create entries, got %d", n)
	}
}

func TestTouchFingerprint_SaturatesAtThresholdPlusOne(t *testing.T) {
	mem := cache.NewMemory(cache.WithoutSweeper())
	defer mem.Stop()
	cfg := testPolicyConfig()
	cfg.Risk.FPReplayThreshold = 2
	cfg.Risk.FPReplayWindowSeconds = 60
	store := &ClientRiskStore{settings: fixedSettings(cfg), store: mem}

	for i, want := range []int{1, 2, 3, 4, 4} {
		if got := store.TouchFingerprint("fp", fmt.Sprintf("client-%d", i)); got != want {
			t.Errorf("client-%d: want %d, got %d", i, want, got)
		}
	}
	if size := mem.FingerprintSize("fp"); size != 3 {
		t.Errorf("set must stop growing at threshold+1, got %d", size)
	}
	if ttl, ok := mem.TTL(cache.KindFingerprint, "fp"); !ok || ttl > time.Minute {
		t.Errorf("ttl must be refreshed to the window, got %s %t", ttl, ok)
	}
}
