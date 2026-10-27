// Package cache 短期数据存储：带过期时间的数据放在进程内存，重启即丢失；持久数据见 storage 包
package cache

import (
	"container/heap"
	"log"
	"sync"
	"time"
)

// Kind 短期数据的类别，每类单独计数并有独立的条数上限：某一类被刷爆时不会挤掉其他类的数据
type Kind string

const (
	KindChallenge   Kind = "challenge"   // 题目数据（答案、尝试次数）与二次验证 key
	KindRateLimit   Kind = "ratelimit"   // 单 IP 滑动窗口计数（验证码加难、管理员登录限流）
	KindClient      Kind = "client"      // 客户端风控状态（近期失败 / 成功次数、上次风险分）
	KindFingerprint Kind = "fingerprint" // 设备指纹 -> 客户端集合（重放计数）
)

const (
	// DefaultLimitPerKind 每类数据的默认条数上限（Q4 默认）：超限时淘汰最早到期的条目
	DefaultLimitPerKind = 200_000
	// defaultSweepInterval 后台清理过期条目的间隔
	defaultSweepInterval = 30 * time.Second
)

// ClientState 客户端风控状态
type ClientState struct {
	Fails     int
	Successes int
	LastScore int
	UpdatedAt int64 // Unix 秒
}

// Memory 进程内存中的短期数据存储：每个条目都有过期时间，重启即丢失（单实例部署，可以接受）。
// 所有读改写都在同一把锁内完成，保证原子语义（如一次性凭证只能被消费一次）
type Memory struct {
	mu      sync.Mutex
	now     func() time.Time
	limit   int
	buckets map[Kind]*bucket

	evicted  map[Kind]int64 // 因超限被淘汰的条目累计数
	lastWarn map[Kind]time.Time

	stop     chan struct{}
	stopOnce sync.Once
}

// Option 创建 Memory 时的可选项
type Option func(*Memory)

// WithClock 注入时钟，供测试推进时间
func WithClock(now func() time.Time) Option {
	return func(m *Memory) { m.now = now }
}

// WithLimit 设置每类数据的条数上限
func WithLimit(n int) Option {
	return func(m *Memory) { m.limit = n }
}

// WithoutSweeper 不启动后台清理（测试中手动调用 Sweep）
func WithoutSweeper() Option {
	return func(m *Memory) { m.stop = nil }
}

// NewMemory 创建内存存储，并启动每 30 秒一次的过期清理
func NewMemory(opts ...Option) *Memory {
	m := &Memory{
		now:      time.Now,
		limit:    DefaultLimitPerKind,
		buckets:  make(map[Kind]*bucket),
		evicted:  make(map[Kind]int64),
		lastWarn: make(map[Kind]time.Time),
		stop:     make(chan struct{}),
	}
	for _, opt := range opts {
		opt(m)
	}
	if m.stop != nil {
		go m.sweepLoop(defaultSweepInterval)
	}
	return m
}

var (
	shared     *Memory
	sharedOnce sync.Once
)

// Shared 进程内共用的短期数据存储
func Shared() *Memory {
	sharedOnce.Do(func() { shared = NewMemory() })
	return shared
}

// Stop 停止后台清理
func (m *Memory) Stop() {
	m.stopOnce.Do(func() {
		if m.stop != nil {
			close(m.stop)
		}
	})
}

func (m *Memory) sweepLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
			m.Sweep()
		}
	}
}

// Sweep 删除全部已过期条目
func (m *Memory) Sweep() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	for _, b := range m.buckets {
		for b.h.Len() > 0 && !b.h[0].expiresAt.After(now) {
			e := heap.Pop(&b.h).(*entry)
			delete(b.items, e.key)
		}
	}
}

// Len 某类数据当前的条目数（含尚未清理的过期条目）
func (m *Memory) Len(kind Kind) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if b := m.buckets[kind]; b != nil {
		return len(b.items)
	}
	return 0
}

// Evicted 某类数据因超限被淘汰的累计条目数
func (m *Memory) Evicted(kind Kind) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.evicted[kind]
}

// ---------------------------------------------------------------------------
// 键值
// ---------------------------------------------------------------------------

// Set 写入值并设置过期时间（覆盖已有值）
func (m *Memory) Set(kind Kind, key string, value []byte, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.put(kind, key, append([]byte(nil), value...), ttl)
}

// Get 读取未过期的值
func (m *Memory) Get(kind Kind, key string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.live(kind, key)
	if e == nil {
		return nil, false
	}
	value, _ := e.value.([]byte)
	return append([]byte(nil), value...), true
}

// Del 删除条目，返回删除前是否存在（且未过期）
func (m *Memory) Del(kind Kind, key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	existed := m.live(kind, key) != nil
	m.remove(kind, key)
	return existed
}

// GetDel 原子地读取并删除：并发调用时只有一个能拿到值，用于一次性凭证
func (m *Memory) GetDel(kind Kind, key string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.live(kind, key)
	if e == nil {
		return nil, false
	}
	m.remove(kind, key)
	value, _ := e.value.([]byte)
	return value, true
}

// Update 原子地读改写：fn 收到当前值（不存在时 ok=false），返回新值与是否保留；
// keep=false 时删除条目。保留时过期时间沿用原值，条目原先不存在时使用 ttl
func (m *Memory) Update(kind Kind, key string, ttl time.Duration, fn func(old []byte, ok bool) (next []byte, keep bool)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.live(kind, key)
	var old []byte
	if e != nil {
		old, _ = e.value.([]byte)
	}
	next, keep := fn(old, e != nil)
	if !keep {
		m.remove(kind, key)
		return
	}
	if e != nil {
		e.value = append([]byte(nil), next...)
		return
	}
	m.put(kind, key, append([]byte(nil), next...), ttl)
}

// ---------------------------------------------------------------------------
// 滑动窗口计数
// ---------------------------------------------------------------------------

// Hit 在 window 时间窗内记录 key 的一次请求，返回窗口内已放行的请求数（含本次）与是否放行。
// 窗口内已放行 limit 次后拒绝，被拒绝的请求不计入窗口；limit <= 0 表示不限也不计数
func (m *Memory) Hit(key string, window time.Duration, limit int) (int, bool) {
	if limit <= 0 {
		return 0, true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	cutoff := now.Add(-window)

	var stamps []time.Time
	if e := m.live(KindRateLimit, key); e != nil {
		stamps, _ = e.value.([]time.Time)
	}
	kept := stamps[:0]
	for _, t := range stamps {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= limit {
		m.put(KindRateLimit, key, kept, m.ttlLeft(KindRateLimit, key, window))
		return len(kept), false
	}
	kept = append(kept, now)
	m.put(KindRateLimit, key, kept, window)
	return len(kept), true
}

// WindowCount 窗口内已放行的请求数，只读不记录（观测与测试用）
func (m *Memory) WindowCount(key string, window time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.live(KindRateLimit, key)
	if e == nil {
		return 0
	}
	stamps, _ := e.value.([]time.Time)
	cutoff := m.now().Add(-window)
	n := 0
	for _, t := range stamps {
		if t.After(cutoff) {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// 客户端风控状态
// ---------------------------------------------------------------------------

// RecordClient 原子更新客户端状态：失败 fails+1；成功 successes+1 且 fails 衰减 1（下限 0）；
// 同时更新上次风险分与时间，并把过期时间刷新为 ttl
func (m *Memory) RecordClient(key string, success bool, score int, ttl time.Duration) ClientState {
	m.mu.Lock()
	defer m.mu.Unlock()
	var state ClientState
	if e := m.live(KindClient, key); e != nil {
		state, _ = e.value.(ClientState)
	}
	if success {
		state.Successes++
		if state.Fails > 0 {
			state.Fails--
		}
	} else {
		state.Fails++
	}
	state.LastScore = score
	state.UpdatedAt = m.now().Unix()
	m.put(KindClient, key, state, ttl)
	return state
}

// GetClient 读取客户端状态，不存在时返回零值与 false
func (m *Memory) GetClient(key string) (ClientState, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.live(KindClient, key)
	if e == nil {
		return ClientState{}, false
	}
	state, _ := e.value.(ClientState)
	return state, true
}

// ---------------------------------------------------------------------------
// 指纹集合
// ---------------------------------------------------------------------------

// TouchFingerprint 记录 fp 与 client 的对应关系，返回时间窗内该指纹对应的不同客户端数，并把过期时间刷新为 ttl。
// 集合大小达到 saturation 后不再写入新成员（持续重放也不会把集合撑大），但新客户端仍按"多一个"报告
func (m *Memory) TouchFingerprint(fp, client string, saturation int, ttl time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	var set map[string]struct{}
	if e := m.live(KindFingerprint, fp); e != nil {
		set, _ = e.value.(map[string]struct{})
	}
	if set == nil {
		set = make(map[string]struct{})
	}
	count := len(set)
	if count < saturation {
		set[client] = struct{}{}
		count = len(set)
	} else if _, ok := set[client]; !ok {
		count++
	}
	m.put(KindFingerprint, fp, set, ttl)
	return count
}

// FingerprintSize 指纹集合当前的成员数，不存在时为 0（观测与测试用）
func (m *Memory) FingerprintSize(fp string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.live(KindFingerprint, fp)
	if e == nil {
		return 0
	}
	set, _ := e.value.(map[string]struct{})
	return len(set)
}

// TTL 条目的剩余有效期，不存在或已过期时返回 false
func (m *Memory) TTL(kind Kind, key string) (time.Duration, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.live(kind, key)
	if e == nil {
		return 0, false
	}
	return e.expiresAt.Sub(m.now()), true
}

// ---------------------------------------------------------------------------
// 内部：按类别分桶，map 查找 + 按到期时间排序的最小堆（淘汰与清理都取堆顶）
// ---------------------------------------------------------------------------

type entry struct {
	key       string
	value     any
	expiresAt time.Time
	index     int
}

type expiryHeap []*entry

func (h expiryHeap) Len() int           { return len(h) }
func (h expiryHeap) Less(i, j int) bool { return h[i].expiresAt.Before(h[j].expiresAt) }
func (h expiryHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}
func (h *expiryHeap) Push(x any) {
	e := x.(*entry)
	e.index = len(*h)
	*h = append(*h, e)
}
func (h *expiryHeap) Pop() any {
	old := *h
	n := len(old)
	e := old[n-1]
	old[n-1] = nil
	e.index = -1
	*h = old[:n-1]
	return e
}

type bucket struct {
	items map[string]*entry
	h     expiryHeap
}

func (m *Memory) bucket(kind Kind) *bucket {
	b := m.buckets[kind]
	if b == nil {
		b = &bucket{items: make(map[string]*entry)}
		m.buckets[kind] = b
	}
	return b
}

// live 返回未过期的条目；已过期的顺手删除
func (m *Memory) live(kind Kind, key string) *entry {
	b := m.buckets[kind]
	if b == nil {
		return nil
	}
	e := b.items[key]
	if e == nil {
		return nil
	}
	if !e.expiresAt.After(m.now()) {
		heap.Remove(&b.h, e.index)
		delete(b.items, key)
		return nil
	}
	return e
}

// ttlLeft 条目剩余的有效期，不存在时返回 fallback
func (m *Memory) ttlLeft(kind Kind, key string, fallback time.Duration) time.Duration {
	if b := m.buckets[kind]; b != nil {
		if e := b.items[key]; e != nil {
			return e.expiresAt.Sub(m.now())
		}
	}
	return fallback
}

// put 写入或覆盖条目；新增条目使该类超过上限时淘汰最早到期的条目
func (m *Memory) put(kind Kind, key string, value any, ttl time.Duration) {
	b := m.bucket(kind)
	expiresAt := m.now().Add(ttl)
	if e := b.items[key]; e != nil {
		e.value = value
		e.expiresAt = expiresAt
		heap.Fix(&b.h, e.index)
		return
	}
	if m.limit > 0 && len(b.items) >= m.limit {
		victim := heap.Pop(&b.h).(*entry)
		delete(b.items, victim.key)
		m.evicted[kind]++
		m.warnEviction(kind)
	}
	e := &entry{key: key, value: value, expiresAt: expiresAt}
	heap.Push(&b.h, e)
	b.items[key] = e
}

func (m *Memory) remove(kind Kind, key string) {
	b := m.buckets[kind]
	if b == nil {
		return
	}
	if e := b.items[key]; e != nil {
		heap.Remove(&b.h, e.index)
		delete(b.items, key)
	}
}

// warnEviction 超限淘汰时告警，同一类每分钟最多打印一次
func (m *Memory) warnEviction(kind Kind) {
	now := m.now()
	if last, ok := m.lastWarn[kind]; ok && now.Sub(last) < time.Minute {
		return
	}
	m.lastWarn[kind] = now
	log.Printf("短期数据超过上限，已淘汰最早到期的条目: kind=%s, limit=%d, evicted_total=%d", kind, m.limit, m.evicted[kind])
}
