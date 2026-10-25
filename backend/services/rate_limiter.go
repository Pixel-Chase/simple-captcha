package services

import (
	"context"
	"fmt"
	"simple-captcha/cache"
	"simple-captcha/helper"
	"time"

	"github.com/go-redis/redis/v8"
)

// RateLimitKeyPrefix 单 IP 计数在 Redis 中的 key 前缀
const RateLimitKeyPrefix = "captcha:ratelimit:"

// 滑动窗口限流脚本：
// KEYS[1] 限流 key；ARGV[1] 当前毫秒时间戳；ARGV[2] 窗口毫秒数；ARGV[3] 上限；ARGV[4] 本次请求成员
// 先清理窗口外的记录，再判断数量，放行时写入并刷新过期时间，整体原子执行。
// 返回 {窗口内已放行数, 是否放行}：被拒绝时返回现有数量，放行时返回含本次在内的数量。
var slidingWindowScript = redis.NewScript(`
redis.call('ZREMRANGEBYSCORE', KEYS[1], 0, tonumber(ARGV[1]) - tonumber(ARGV[2]))
local count = redis.call('ZCARD', KEYS[1])
if count >= tonumber(ARGV[3]) then
  return {count, 0}
end
redis.call('ZADD', KEYS[1], tonumber(ARGV[1]), ARGV[4])
redis.call('PEXPIRE', KEYS[1], tonumber(ARGV[2]))
return {count + 1, 1}
`)

// redisSlidingWindowLimiter 基于 Redis 有序集合的滑动窗口计数，多实例共享
type redisSlidingWindowLimiter struct {
	window time.Duration
	prefix string
}

func newRedisSlidingWindowLimiter(window time.Duration) *redisSlidingWindowLimiter {
	return &redisSlidingWindowLimiter{window: window, prefix: RateLimitKeyPrefix}
}

// Hit 记录 key 的一次请求，返回当前窗口内已放行的请求数（含本次）与是否放行。
// 窗口内已放行 limit 次后拒绝，被拒绝的请求不计入窗口；limit <= 0 表示不限也不计数
func (l *redisSlidingWindowLimiter) Hit(key string, limit int) (int, bool, error) {
	if limit <= 0 {
		return 0, true, nil
	}
	now := time.Now().UnixMilli()
	member := fmt.Sprintf("%d-%s", now, helper.GenerateRandomString(6))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := slidingWindowScript.Run(
		ctx,
		cache.GetRedisClient(),
		[]string{l.prefix + key},
		now, l.window.Milliseconds(), limit, member,
	).Int64Slice()
	if err != nil {
		return 0, false, err
	}
	if len(res) != 2 {
		return 0, false, fmt.Errorf("限流脚本返回值异常: %v", res)
	}
	return int(res[0]), res[1] == 1, nil
}
