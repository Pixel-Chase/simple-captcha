package services

import (
	"image"
	"log"
	"math"
	"sync"

	xdraw "golang.org/x/image/draw"
)

// backgroundCache 按目标尺寸缓存背景图集合。
//
// go-captcha 会从背景图中随机裁出 imageSize 大小的窗口；当背景图小于 imageSize
// （例如 420x420 的素材配 2x/3x 出图）时裁切会被截断，主图尺寸与坐标空间不一致。
// 这里在构建生成器前把背景图按"覆盖"方式放大到至少 width×height，保证坐标空间正确。
// 更大的原图不会被缩小，以保留随机裁切带来的多样性。
type backgroundCache struct {
	mu    sync.Mutex
	base  []image.Image
	items map[[2]int][]image.Image
}

func newBackgroundCache(base []image.Image) *backgroundCache {
	return &backgroundCache{
		base:  base,
		items: make(map[[2]int][]image.Image),
	}
}

// get 返回至少能覆盖 width×height 的背景图集合
func (c *backgroundCache) get(width, height int) []image.Image {
	key := [2]int{width, height}

	c.mu.Lock()
	defer c.mu.Unlock()

	if images, ok := c.items[key]; ok {
		return images
	}

	images := make([]image.Image, 0, len(c.base))
	upscaled := 0
	for _, img := range c.base {
		resized, changed := coverResize(img, width, height)
		if changed {
			upscaled++
		}
		images = append(images, resized)
	}
	if upscaled > 0 {
		log.Printf("背景素材尺寸不足，已将 %d/%d 张放大到至少 %dx%d；如需更清晰的高倍率图请提供 ≥ %dx%d 的素材",
			upscaled, len(c.base), width, height, width, height)
	}

	c.items[key] = images
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
