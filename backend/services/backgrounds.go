package services

import (
	"fmt"
	"image"
	"log"
	"math"
	"simple-captcha/helper"
	"sync"

	xdraw "golang.org/x/image/draw"
)

// BackgroundStore 背景图仓库：全部题型共用同一份解码后的素材，并支持热重载。
//
// go-captcha 会从背景图中随机裁出 imageSize 大小的窗口；当背景图小于 imageSize
// （例如 420x420 的素材配 2x/3x 出图）时裁切会被截断，主图尺寸与坐标空间不一致。
// 因此按目标尺寸缓存一份"覆盖"放大后的副本，保证坐标空间正确；更大的原图不缩小，
// 以保留随机裁切带来的多样性。
//
// 管理接口增删素材后调用 Reload：重新读取目录、替换素材集合并清空尺寸缓存，
// 随后通知各题型服务丢弃已构建的生成器实例，下一次出题即使用新素材，无需重启。
type BackgroundStore struct {
	dir string

	mu      sync.Mutex
	base    []image.Image
	sized   map[[2]int][]image.Image
	version uint64

	listenerMu sync.Mutex
	listeners  []func()
}

// NewBackgroundStore 读取目录下全部图片并构建仓库；目录中没有可用图片时返回错误
func NewBackgroundStore(dir string) (*BackgroundStore, error) {
	store := &BackgroundStore{
		dir:   dir,
		sized: make(map[[2]int][]image.Image),
	}
	if err := store.Reload(); err != nil {
		return nil, err
	}
	return store, nil
}

// Dir 返回素材目录
func (s *BackgroundStore) Dir() string {
	return s.dir
}

// Count 返回当前已加载的背景图数量
func (s *BackgroundStore) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.base)
}

// Version 每次成功重载后递增，便于观测与测试
func (s *BackgroundStore) Version() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.version
}

// OnReload 注册重载回调；题型服务借此丢弃缓存的生成器实例
func (s *BackgroundStore) OnReload(fn func()) {
	s.listenerMu.Lock()
	defer s.listenerMu.Unlock()
	s.listeners = append(s.listeners, fn)
}

// Reload 重新读取素材目录。读取失败时保留原有素材并返回错误，不会让服务落入无图状态
func (s *BackgroundStore) Reload() error {
	images, err := helper.GetImagesFromDir(s.dir)
	if err != nil {
		return fmt.Errorf("reload backgrounds: %w", err)
	}

	s.mu.Lock()
	s.base = images
	s.sized = make(map[[2]int][]image.Image)
	s.version++
	version := s.version
	s.mu.Unlock()

	// 先替换素材再通知：回调执行后新建的生成器实例必然拿到新素材
	s.listenerMu.Lock()
	listeners := append([]func(){}, s.listeners...)
	s.listenerMu.Unlock()
	for _, fn := range listeners {
		fn()
	}

	log.Printf("背景图已加载: dir=%s, count=%d, version=%d", s.dir, len(images), version)
	return nil
}

// Get 返回至少能覆盖 width×height 的背景图集合
func (s *BackgroundStore) Get(width, height int) []image.Image {
	key := [2]int{width, height}

	s.mu.Lock()
	defer s.mu.Unlock()

	if images, ok := s.sized[key]; ok {
		return images
	}

	images := make([]image.Image, 0, len(s.base))
	upscaled := 0
	for _, img := range s.base {
		resized, changed := coverResize(img, width, height)
		if changed {
			upscaled++
		}
		images = append(images, resized)
	}
	if upscaled > 0 {
		log.Printf("背景素材尺寸不足，已将 %d/%d 张放大到至少 %dx%d；如需更清晰的高倍率图请提供 ≥ %dx%d 的素材",
			upscaled, len(s.base), width, height, width, height)
	}

	s.sized[key] = images
	return images
}

// coverResize 当图片任一边小于目标尺寸时，按等比放大到恰好覆盖目标尺寸；否则原样返回
func coverResize(img image.Image, width, height int) (image.Image, bool) {
	bounds := img.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW <= 0 || srcH <= 0 || (srcW >= width && srcH >= height) {
		return img, false
	}

	factor := math.Max(float64(width)/float64(srcW), float64(height)/float64(srcH))
	dstW := int(math.Ceil(float64(srcW) * factor))
	dstH := int(math.Ceil(float64(srcH) * factor))
	if dstW < width {
		dstW = width
	}
	if dstH < height {
		dstH = height
	}

	dst := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, xdraw.Src, nil)
	return dst, true
}
