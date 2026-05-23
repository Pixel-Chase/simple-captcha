// cache/redis.go

package cache

import (
	"context"
	"errors"
	"fmt"
	"log"
	"simple-captcha/config"
	"strconv"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

var (
	client *redis.Client
	once   sync.Once
	ctx    = context.Background()
)

// CacheError 缓存错误类型
type CacheError struct {
	Operation string `json:"operation"`
	Key       string `json:"key"`
	Message   string `json:"message"`
	Err       error  `json:"-"`
}

func (e *CacheError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("缓存操作失败 [%s:%s] %s: %v", e.Operation, e.Key, e.Message, e.Err)
	}
	return fmt.Sprintf("缓存操作失败 [%s:%s] %s", e.Operation, e.Key, e.Message)
}

// initClient 初始化 Redis 客户端
func initClient() {
	cfg := config.LoadConfig()

	db, err := strconv.Atoi(strconv.Itoa(cfg.Redis.DB))
	if err != nil {
		log.Fatalf("无效的 REDIS_DB 值: %v", err)
	}

	client = redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%s", cfg.Redis.Host, cfg.Redis.Port),
		Password:     cfg.Redis.Password,
		DB:           db,
		PoolSize:     10,              // 连接池大小
		MinIdleConns: 5,               // 最小空闲连接数
		MaxRetries:   3,               // 最大重试次数
		DialTimeout:  5 * time.Second, // 连接超时
		ReadTimeout:  3 * time.Second, // 读取超时
		WriteTimeout: 3 * time.Second, // 写入超时
		IdleTimeout:  5 * time.Minute, // 空闲连接超时
	})

	// 测试连接是否成功，带重试机制
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		if err := client.Ping(ctx).Err(); err != nil {
			log.Printf("Redis连接失败 (尝试 %d/%d): %v", i+1, maxRetries, err)
			if i == maxRetries-1 {
				log.Fatalf("无法连接到 Redis，已达到最大重试次数: %v", err)
			}
			time.Sleep(time.Duration(i+1) * time.Second) // 递增延迟
			continue
		}
		break
	}

	log.Println("成功连接到 Redis")
}

// GetRedisClient 返回单例 Redis 客户端
func GetRedisClient() *redis.Client {
	once.Do(initClient)
	return client
}

// Get 获取键对应的值，如果键不存在或发生错误，则返回 nil
func Get(key string) *string {
	if key == "" {
		log.Printf("缓存获取: 键不能为空")
		return nil
	}

	val, err := GetRedisClient().Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// 键不存在，这是正常情况，不记录错误日志
			return nil
		}
		// 记录错误日志
		log.Printf("缓存获取失败: key=%s, error=%v", key, err)
		return nil
	}
	return &val
}

// Set 设置指定键的值，并设置过期时间。
// 该函数使用Redis存储数据，适用于需要缓存或临时存储信息的场景。
// 参数:
//
//	key: 要设置的键，用于唯一标识存储的值。
//	value: 要存储的值，可以是任意类型的数据。
//	expiration: 键的过期时间，用于指定数据的缓存时长。
//
// 返回值:
//
//	error: 如果设置操作失败，则返回错误信息；否则返回nil。
func Set(key string, value interface{}, expiration time.Duration) error {
	if key == "" {
		return &CacheError{
			Operation: "SET",
			Key:       key,
			Message:   "键不能为空",
		}
	}

	err := GetRedisClient().Set(ctx, key, value, expiration).Err()
	if err != nil {
		log.Printf("缓存设置失败: key=%s, expiration=%v, error=%v", key, expiration, err)
		return &CacheError{
			Operation: "SET",
			Key:       key,
			Message:   "设置缓存失败",
			Err:       err,
		}
	}
	return nil
}

// Del 删除指定的 key
func Del(keys ...string) error {
	if len(keys) == 0 {
		return &CacheError{
			Operation: "DEL",
			Key:       "",
			Message:   "没有指定要删除的键",
		}
	}

	// 过滤空键
	validKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		if key != "" {
			validKeys = append(validKeys, key)
		}
	}

	if len(validKeys) == 0 {
		return &CacheError{
			Operation: "DEL",
			Key:       "",
			Message:   "没有有效的键可删除",
		}
	}

	err := GetRedisClient().Del(ctx, validKeys...).Err()
	if err != nil {
		log.Printf("缓存删除失败: keys=%v, error=%v", validKeys, err)
		return &CacheError{
			Operation: "DEL",
			Key:       fmt.Sprintf("%v", validKeys),
			Message:   "删除缓存失败",
			Err:       err,
		}
	}
	return nil
}

// Exists 检查指定的 key 是否存在
func Exists(key string) bool {
	if key == "" {
		log.Printf("缓存存在性检查: 键不能为空")
		return false
	}

	count, err := GetRedisClient().Exists(ctx, key).Result()
	if err != nil {
		log.Printf("缓存存在性检查失败: key=%s, error=%v", key, err)
		return false
	}
	return count > 0
}

// HSet 在Redis中设置哈希表中指定字段的值。
// 该函数接受三个参数：
// - key: 哈希表的键名。
// - field: 哈希表字段的名称。
// - value: 要设置给字段的值。
// 函数返回一个错误，如果操作成功，则返回nil。
func HSet(key string, field string, value interface{}) error {
	if key == "" || field == "" {
		return &CacheError{
			Operation: "HSET",
			Key:       key,
			Message:   "键和字段不能为空",
		}
	}

	err := GetRedisClient().HSet(ctx, key, field, value).Err()
	if err != nil {
		log.Printf("哈希缓存设置失败: key=%s, field=%s, error=%v", key, field, err)
		return &CacheError{
			Operation: "HSET",
			Key:       key,
			Message:   "设置哈希缓存失败",
			Err:       err,
		}
	}
	return nil
}

// HGet 从哈希表中获取指定字段的值。
// 该函数需要两个参数：
//
//	key - 哈希表的键名。
//	field - 要获取的字段名。
//
// 函数返回值：
//
//	成功时返回字段的值和 nil 错误。
//	如果键不存在或者字段在哈希表中不存在，返回空字符串和 nil 错误。
//	如果遇到其他错误，返回空字符串和相应的错误信息。
func HGet(key string, field string) (string, error) {
	if key == "" || field == "" {
		return "", &CacheError{
			Operation: "HGET",
			Key:       key,
			Message:   "键和字段不能为空",
		}
	}

	val, err := GetRedisClient().HGet(ctx, key, field).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// 键或字段不存在，返回空字符串和nil错误
			return "", nil
		}
		log.Printf("哈希缓存获取失败: key=%s, field=%s, error=%v", key, field, err)
		return "", &CacheError{
			Operation: "HGET",
			Key:       key,
			Message:   "获取哈希缓存失败",
			Err:       err,
		}
	}
	return val, nil
}

// GetStats 获取Redis连接统计信息
func GetStats() map[string]interface{} {
	client := GetRedisClient()
	stats := client.PoolStats()

	return map[string]interface{}{
		"hits":        stats.Hits,
		"misses":      stats.Misses,
		"timeouts":    stats.Timeouts,
		"total_conns": stats.TotalConns,
		"idle_conns":  stats.IdleConns,
		"stale_conns": stats.StaleConns,
	}
}

// Ping 检查Redis连接状态
func Ping() error {
	err := GetRedisClient().Ping(ctx).Err()
	if err != nil {
		return &CacheError{
			Operation: "PING",
			Key:       "",
			Message:   "Redis连接检查失败",
			Err:       err,
		}
	}
	return nil
}

// Close 关闭Redis连接
func Close() error {
	if client != nil {
		err := client.Close()
		if err != nil {
			log.Printf("关闭Redis连接失败: %v", err)
			return &CacheError{
				Operation: "CLOSE",
				Key:       "",
				Message:   "关闭Redis连接失败",
				Err:       err,
			}
		}
		log.Println("Redis连接已关闭")
	}
	return nil
}
