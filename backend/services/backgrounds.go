package services

import (
	"fmt"
	"image"
	"log"
	"math"
	"simple-captcha/helper"
	"slices"
	"sync"
	"sync/atomic"

	xdraw "golang.org/x/image/draw"
)

// 背景素材规格：下限保证 3 倍率出图无需放大，建议尺寸为放大取景与清晰度留出余量
const (
	minBackgroundWidth          = baseImageWidth * MaxScale
	minBackgroundHeight         = baseImageHeight * MaxScale
	recommendedBackgroundWidth  = 1800
	recommendedBackgroundHeight = 1200
)

// backgroundZoomPercents 背景图相对目标尺寸的放大档位（%）。每个出图尺寸按每一档各派生一份，
// go-captcha 随机挑一份并在多出的范围内随机取景：缩放比例与取景位置都随机，
// 同一张素材每次出题的像素网格都对不上，无法拿干净原图直接比对出缺口。
// 档位越多越难比对，但内存按档位数成倍增加
var backgroundZoomPercents = []int{110, 125}

// BackgroundStore 背景图仓库：全部题型共用同一份素材，并支持热重载。
//
// 每张素材加载时等比缩放成工作副本（恰好覆盖 3 倍率主图的最大放大档），原图随即释放，
// 高清原图不会常驻内存。各题型再按自身目标尺寸和每个放大档，从工作副本派生"恰好覆盖"的版本并缓存：
// 整图等比缩放保留素材的完整构图，放大档与随机取景提供每次出题的差异。
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
	maxZoom := slices.Max(backgroundZoomPercents)
	workWidth, workHeight := zoomed(minBackgroundWidth, maxZoom), zoomed(minBackgroundHeight, maxZoom)
	var undersized atomic.Int32
	images, err := helper.GetImagesFromDir(s.dir, func(img image.Image) image.Image {
		if b := img.Bounds(); b.Dx() < minBackgroundWidth || b.Dy() < minBackgroundHeight {
			undersized.Add(1)
		}
		return coverResize(img, workWidth, workHeight)
	})
	if err != nil {
		return fmt.Errorf("reload backgrounds: %w", err)
	}
	if n := undersized.Load(); n > 0 {
		log.Printf("有 %d/%d 张背景素材小于 %dx%d，3 倍率出图时会被放大而发糊；建议提供 3:2、≥ %dx%d 的素材",
			n, len(images), minBackgroundWidth, minBackgroundHeight, recommendedBackgroundWidth, recommendedBackgroundHeight)
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

// Get 返回用于 width×height 出图的背景图集合：每张素材按每个放大档各一份，
// 等比缩放到恰好覆盖“目标尺寸 × 放大档”的大小
func (s *BackgroundStore) Get(width, height int) []image.Image {
	key := [2]int{width, height}

	s.mu.Lock()
	defer s.mu.Unlock()

	if images, ok := s.sized[key]; ok {
		return images
	}

	images := make([]image.Image, 0, len(s.base)*len(backgroundZoomPercents))
	for _, zoom := range backgroundZoomPercents {
		for _, img := range s.base {
			images = append(images, coverResize(img, zoomed(width, zoom), zoomed(height, zoom)))
		}
	}

	s.sized[key] = images
	return images
}

// zoomed 按百分比放大尺寸（向上取整，整数运算避免浮点误差）
func zoomed(size, percent int) int {
	return (size*percent + 99) / 100
}

// coverResize 等比缩放（放大或缩小）到恰好覆盖 width×height：受限的一边与目标相等，另一边不小于目标。
// 尺寸已经恰好覆盖时原样返回
func coverResize(img image.Image, width, height int) image.Image {
	bounds := img.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return img
	}

	// 先确定受限的一边再按比例推另一边，避免浮点误差让受限边多出 1 像素
	dstW, dstH := width, height
	if float64(width)/float64(srcW) >= float64(height)/float64(srcH) {
		dstH = max(height, int(math.Round(float64(srcH)*float64(width)/float64(srcW))))
	} else {
		dstW = max(width, int(math.Round(float64(srcW)*float64(height)/float64(srcH))))
	}
	if dstW == srcW && dstH == srcH {
		return img
	}

	dst := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, xdraw.Src, nil)
	return dst
}
