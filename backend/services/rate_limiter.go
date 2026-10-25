package services

import (
	"context"
	"fmt"
	"simple-captcha/cache"
	"simple-captcha/helper"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

// RateLimiter 限流器接口
type RateLimiter interface {
	// Allow 判断 key 在当前窗口内是否仍可放行；error 非空表示限流后端不可用
	Allow(key string) (bool, error)
}

// 滑动窗口限流脚本：
// KEYS[1] 限流 key；ARGV[1] 当前毫秒时间戳；ARGV[2] 窗口毫秒数；ARGV[3] 上限；ARGV[4] 本次请求成员
// 先清理窗口外的记录，再判断数量，放行时写入并刷新过期时间，整体原子执行。
var slidingWindowScript = redis.NewScript(`
redis.call('ZREMRANGEBYSCORE', KEYS[1], 0, tonumber(ARGV[1]) - tonumber(ARGV[2]))
local count = redis.call('ZCARD', KEYS[1])
if count >= tonumber(ARGV[3]) then
  return 0
end
redis.call('ZADD', KEYS[1], tonumber(ARGV[1]), ARGV[4])
redis.call('PEXPIRE', KEYS[1], tonumber(ARGV[2]))
return 1
`)

// redisSlidingWindowLimiter 基于 Redis 有序集合的滑动窗口限流，多实例共享计数
type redisSlidingWindowLimiter struct {
	limit  int
	window time.Duration
	prefix string
}

func newRedisSlidingWindowLimiter(limit int, window time.Duration) *redisSlidingWindowLimiter {
	return &redisSlidingWindowLimiter{limit: limit, window: window, prefix: "captcha:ratelimit:"}
}

// Allow 实现 RateLimiter
func (l *redisSlidingWindowLimiter) Allow(key string) (bool, error) {
	if l.limit <= 0 {
		return true, nil
	}
	now := time.Now().UnixMilli()
	member := fmt.Sprintf("%d-%s", now, helper.GenerateRandomString(6))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := slidingWindowScript.Run(
		ctx,
		cache.GetRedisClient(),
		[]string{l.prefix + key},
		now, l.window.Milliseconds(), l.limit, member,
	).Int()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}

// memoryRateLimiter 进程内固定窗口限流，作为 Redis 不可用时的兜底或单实例部署的选择
type memoryRateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]*memoryBucket
}

type memoryBucket struct {
	count       int
	windowStart time.Time
}

func newMemoryRateLimiter(limit int, window time.Duration) *memoryRateLimiter {
	return &memoryRateLimiter{
		limit:   limit,
		window:  window,
		buckets: make(map[string]*memoryBucket),
	}
}

// Allow 实现 RateLimiter
func (l *memoryRateLimiter) Allow(key string) (bool, error) {
	if l.limit <= 0 {
		return true, nil
	}
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	bucket, ok := l.buckets[key]
	if !ok || now.Sub(bucket.windowStart) >= l.window {
		l.buckets[key] = &memoryBucket{count: 1, windowStart: now}
		return true, nil
	}
	if bucket.count >= l.limit {
		return false, nil
	}
	bucket.count++
	return true, nil
}

// cleanup 清理过期窗口
func (l *memoryRateLimiter) cleanup(now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, bucket := range l.buckets {
		if now.Sub(bucket.windowStart) >= l.window*2 {
			delete(l.buckets, key)
		}
	}
}

// size 当前跟踪的 key 数量
func (l *memoryRateLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}
