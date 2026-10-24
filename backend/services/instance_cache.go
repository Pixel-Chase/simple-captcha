package services

import "sync"

// instanceKey 验证码生成器实例的缓存键：难度 × 倍率 × 主题
type instanceKey struct {
	level int
	scale int
	mode  string
}

// instanceCache 按需构建并复用 go-captcha 生成器实例。
// 构建器本身只保存配置与资源引用，开销很小；真正的绘制发生在 Generate 时。
type instanceCache[T any] struct {
	mu    sync.Mutex
	items map[instanceKey]T
	build func(key instanceKey) T
}

func newInstanceCache[T any](build func(key instanceKey) T) *instanceCache[T] {
	return &instanceCache[T]{
		items: make(map[instanceKey]T),
		build: build,
	}
}

// get 获取指定难度/倍率/主题的实例，不存在时构建并缓存
func (c *instanceCache[T]) get(level, scale int, mode string) T {
	key := instanceKey{level: level, scale: scale, mode: mode}

	c.mu.Lock()
	defer c.mu.Unlock()

	if inst, ok := c.items[key]; ok {
		return inst
	}
	inst := c.build(key)
	c.items[key] = inst
	return inst
}

// reset 丢弃全部已构建实例；背景素材热重载后调用，下一次 get 会用新素材重建
func (c *instanceCache[T]) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[instanceKey]T)
}
