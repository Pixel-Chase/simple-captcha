package cache

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeClock 可手动推进的时钟
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{t: time.Unix(1_700_000_000, 0)} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newTestMemory(t *testing.T, opts ...Option) (*Memory, *fakeClock) {
	clock := newFakeClock()
	m := NewMemory(append([]Option{WithClock(clock.Now), WithoutSweeper()}, opts...)...)
	t.Cleanup(m.Stop)
	return m, clock
}

func TestMemory_SetGetExpire(t *testing.T) {
	m, clock := newTestMemory(t)

	m.Set(KindChallenge, "k", []byte("v1"), time.Minute)
	got, ok := m.Get(KindChallenge, "k")
	require.True(t, ok)
	assert.Equal(t, "v1", string(got))

	// 类别互相隔离
	_, ok = m.Get(KindClient, "k")
	assert.False(t, ok)

	// 返回的是副本，修改不影响存储
	got[0] = 'X'
	again, _ := m.Get(KindChallenge, "k")
	assert.Equal(t, "v1", string(again))

	clock.Advance(59 * time.Second)
	_, ok = m.Get(KindChallenge, "k")
	assert.True(t, ok)
	clock.Advance(time.Second)
	_, ok = m.Get(KindChallenge, "k")
	assert.False(t, ok, "到期即失效")
	assert.Zero(t, m.Len(KindChallenge), "读到过期条目时顺手删除")

	// 覆盖写入刷新过期时间
	m.Set(KindChallenge, "k", []byte("v2"), time.Minute)
	clock.Advance(30 * time.Second)
	m.Set(KindChallenge, "k", []byte("v3"), time.Minute)
	clock.Advance(45 * time.Second)
	got, ok = m.Get(KindChallenge, "k")
	require.True(t, ok)
	assert.Equal(t, "v3", string(got))
}

func TestMemory_DelAndGetDel(t *testing.T) {
	m, clock := newTestMemory(t)

	m.Set(KindChallenge, "a", []byte("1"), time.Minute)
	assert.True(t, m.Del(KindChallenge, "a"))
	assert.False(t, m.Del(KindChallenge, "a"))

	m.Set(KindChallenge, "b", []byte("2"), time.Minute)
	v, ok := m.GetDel(KindChallenge, "b")
	require.True(t, ok)
	assert.Equal(t, "2", string(v))
	_, ok = m.GetDel(KindChallenge, "b")
	assert.False(t, ok, "一次性：第二次取不到")

	m.Set(KindChallenge, "c", []byte("3"), time.Second)
	clock.Advance(time.Second)
	_, ok = m.GetDel(KindChallenge, "c")
	assert.False(t, ok, "过期的凭证不能消费")
}

func TestMemory_GetDelConcurrentOnlyOnce(t *testing.T) {
	m := NewMemory()
	t.Cleanup(m.Stop)

	for round := 0; round < 50; round++ {
		key := fmt.Sprintf("token-%d", round)
		m.Set(KindChallenge, key, []byte("ok"), time.Minute)
		var wins atomic.Int32
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, ok := m.GetDel(KindChallenge, key); ok {
					wins.Add(1)
				}
			}()
		}
		wg.Wait()
		require.Equal(t, int32(1), wins.Load(), "round=%d", round)
	}
}

func TestMemory_UpdateIsAtomicAndKeepsTTL(t *testing.T) {
	m, clock := newTestMemory(t)

	// 不存在时以 ttl 新建
	m.Update(KindChallenge, "n", time.Minute, func(old []byte, ok bool) ([]byte, bool) {
		assert.False(t, ok)
		return []byte{0}, true
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.Update(KindChallenge, "n", time.Hour, func(old []byte, ok bool) ([]byte, bool) {
				return []byte{old[0] + 1}, true
			})
		}()
	}
	wg.Wait()
	v, ok := m.Get(KindChallenge, "n")
	require.True(t, ok)
	assert.Equal(t, byte(100), v[0], "并发自增没有丢失")

	// 更新沿用原过期时间，不会被 ttl 参数延长
	clock.Advance(time.Minute)
	_, ok = m.Get(KindChallenge, "n")
	assert.False(t, ok)

	// keep=false 删除
	m.Set(KindChallenge, "d", []byte("x"), time.Minute)
	m.Update(KindChallenge, "d", time.Minute, func([]byte, bool) ([]byte, bool) { return nil, false })
	_, ok = m.Get(KindChallenge, "d")
	assert.False(t, ok)
}

func TestMemory_HitSlidingWindow(t *testing.T) {
	m, clock := newTestMemory(t)
	const window = time.Minute

	tests := []struct {
		name    string
		advance time.Duration
		count   int
		allowed bool
	}{
		{"第 1 次", 0, 1, true},
		{"第 2 次", 10 * time.Second, 2, true},
		{"第 3 次达到上限", 10 * time.Second, 3, true},
		{"超限被拒绝且不计入", 10 * time.Second, 3, false},
		{"仍在窗口内继续拒绝", 10 * time.Second, 3, false},
		// 距第 1 次已过 60 秒：第 1 次滑出窗口
		{"最早一次滑出窗口后放行", 20 * time.Second, 3, true},
		{"再次达到上限", 0, 3, false},
	}
	for _, tt := range tests {
		clock.Advance(tt.advance)
		count, allowed := m.Hit("ip", window, 3)
		assert.Equal(t, tt.count, count, tt.name)
		assert.Equal(t, tt.allowed, allowed, tt.name)
	}

	assert.Equal(t, 3, m.WindowCount("ip", window), "只读计数与 Hit 一致")

	// limit <= 0 不限也不计数
	count, allowed := m.Hit("other", window, 0)
	assert.Zero(t, count)
	assert.True(t, allowed)
	assert.Equal(t, 1, m.Len(KindRateLimit))

	// 整个窗口无请求后计数清零
	clock.Advance(2 * window)
	count, allowed = m.Hit("ip", window, 3)
	assert.Equal(t, 1, count)
	assert.True(t, allowed)
}

func TestMemory_RecordClientDecay(t *testing.T) {
	m, clock := newTestMemory(t)
	ttl := 30 * time.Minute

	tests := []struct {
		success   bool
		score     int
		fails     int
		successes int
	}{
		{false, 40, 1, 0},
		{false, 50, 2, 0},
		{false, 60, 3, 0},
		{true, 10, 2, 1},
		{true, 5, 1, 2},
		{true, 5, 0, 3},
		{true, 5, 0, 4}, // 下限 0
		{false, 70, 1, 4},
	}
	for i, tt := range tests {
		state := m.RecordClient("c", tt.success, tt.score, ttl)
		assert.Equal(t, tt.fails, state.Fails, "step %d", i)
		assert.Equal(t, tt.successes, state.Successes, "step %d", i)
		assert.Equal(t, tt.score, state.LastScore, "step %d", i)
		assert.Equal(t, clock.Now().Unix(), state.UpdatedAt)
	}

	got, ok := m.GetClient("c")
	require.True(t, ok)
	assert.Equal(t, 1, got.Fails)

	// 每次记录都刷新过期时间
	clock.Advance(20 * time.Minute)
	m.RecordClient("c", true, 0, ttl)
	clock.Advance(20 * time.Minute)
	_, ok = m.GetClient("c")
	assert.True(t, ok)
	clock.Advance(10 * time.Minute)
	_, ok = m.GetClient("c")
	assert.False(t, ok)
}

func TestMemory_TouchFingerprintSaturation(t *testing.T) {
	m, clock := newTestMemory(t)
	ttl := 10 * time.Minute
	const saturation = 3 // 阈值 2 + 1

	assert.Equal(t, 1, m.TouchFingerprint("fp", "a", saturation, ttl))
	assert.Equal(t, 1, m.TouchFingerprint("fp", "a", saturation, ttl), "同一客户端重复出题不增加")
	assert.Equal(t, 2, m.TouchFingerprint("fp", "b", saturation, ttl))
	assert.Equal(t, 3, m.TouchFingerprint("fp", "c", saturation, ttl))
	// 已饱和：新客户端按"多一个"报告但不写入，已有成员仍报告当前大小
	assert.Equal(t, 4, m.TouchFingerprint("fp", "d", saturation, ttl))
	assert.Equal(t, 4, m.TouchFingerprint("fp", "e", saturation, ttl))
	assert.Equal(t, 3, m.TouchFingerprint("fp", "a", saturation, ttl))

	clock.Advance(ttl)
	assert.Equal(t, 1, m.TouchFingerprint("fp", "z", saturation, ttl), "过期后重新计数")
}

func TestMemory_EvictsEarliestExpiringWhenFull(t *testing.T) {
	m, _ := newTestMemory(t, WithLimit(3))

	m.Set(KindChallenge, "long", []byte("1"), time.Hour)
	m.Set(KindChallenge, "short", []byte("2"), time.Minute)
	m.Set(KindChallenge, "mid", []byte("3"), 10*time.Minute)
	// 其他类别不受影响
	m.Set(KindClient, "x", []byte("4"), time.Second)

	m.Set(KindChallenge, "new", []byte("5"), 30*time.Minute)
	assert.Equal(t, 3, m.Len(KindChallenge))
	assert.Equal(t, int64(1), m.Evicted(KindChallenge))
	_, ok := m.Get(KindChallenge, "short")
	assert.False(t, ok, "最早到期的被淘汰")
	for _, key := range []string{"long", "mid", "new"} {
		_, ok := m.Get(KindChallenge, key)
		assert.True(t, ok, key)
	}
	_, ok = m.Get(KindClient, "x")
	assert.True(t, ok)

	// 覆盖已有键不触发淘汰
	m.Set(KindChallenge, "long", []byte("6"), time.Hour)
	assert.Equal(t, int64(1), m.Evicted(KindChallenge))
}

func TestMemory_SweepRemovesExpired(t *testing.T) {
	m, clock := newTestMemory(t)
	for i := 0; i < 10; i++ {
		m.Set(KindChallenge, fmt.Sprintf("k%d", i), []byte("v"), time.Duration(i+1)*time.Second)
	}
	m.Hit("ip", 5*time.Second, 10)

	clock.Advance(5 * time.Second)
	m.Sweep()
	assert.Equal(t, 5, m.Len(KindChallenge))
	assert.Zero(t, m.Len(KindRateLimit))

	clock.Advance(time.Hour)
	m.Sweep()
	assert.Zero(t, m.Len(KindChallenge))
}
