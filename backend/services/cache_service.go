package services

import (
	"simple-captcha/cache"
	"time"
)

// RedisCacheService Redis缓存服务实现
type RedisCacheService struct{}

// NewRedisCacheService 创建Redis缓存服务
func NewRedisCacheService() *RedisCacheService {
	return &RedisCacheService{}
}

// Set 设置缓存
func (r *RedisCacheService) Set(key string, value interface{}, expiration int64) error {
	return cache.Set(key, value, time.Duration(expiration)*time.Second)
}

// Get 获取缓存
func (r *RedisCacheService) Get(key string) ([]byte, error) {
	data := cache.Get(key)
	if data == nil {
		return nil, nil
	}
	return []byte(*data), nil
}

// Delete 删除缓存
func (r *RedisCacheService) Delete(key string) error {
	cache.Del(key)
	return nil
}

// Exists 检查是否存在
func (r *RedisCacheService) Exists(key string) bool {
	return cache.Exists(key)
}

// Ping 检查连接状态
func (r *RedisCacheService) Ping() error {
	// 通过设置和获取一个测试键来检查连接
	testKey := "ping_test"
	if err := r.Set(testKey, "pong", 1); err != nil {
		return err
	}
	r.Delete(testKey)
	return nil
}
