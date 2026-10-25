package services

import (
	"image"
	"image/draw"
	"math/rand/v2"
)

// 每道题的扰动幅度：亮度 ±10 色阶、对比度与饱和度 ±6%、噪点 ±3 色阶
const (
	jitterBrightness = 10.0
	jitterContrast   = 0.06
	jitterSaturation = 0.06
	jitterNoise      = 3
)

// imageJitter 对生成好的题目图片做轻微、随机的颜色扰动并叠加细噪点。
// 肉眼几乎察觉不到，但同一张素材每次出题的像素都不同，拿到干净原图也无法逐像素相减找出缺口或文字。
// 同一道题的主图与拼图块 / 旋转内圆共用一个 imageJitter（同一组颜色参数），对齐后颜色仍然一致；噪点逐张独立。
type imageJitter struct {
	lut        [256]uint8 // 亮度与对比度合成的查找表
	saturation float64
}

func newImageJitter() *imageJitter {
	brightness := randSigned(jitterBrightness)
	contrast := 1 + randSigned(jitterContrast)
	j := &imageJitter{saturation: 1 + randSigned(jitterSaturation)}
	for i := range j.lut {
		j.lut[i] = clampUint8((float64(i)-128)*contrast + 128 + brightness)
	}
	return j
}

// apply 返回扰动后的图片：*image.NRGBA 原地修改（go-captcha 每次出题都新建画布，不会污染素材），
// 其他类型先转换为 NRGBA。只改 RGB，Alpha 保持不变，完全透明的像素跳过
func (j *imageJitter) apply(img image.Image) image.Image {
	dst, ok := img.(*image.NRGBA)
	if !ok {
		b := img.Bounds()
		dst = image.NewNRGBA(b)
		draw.Draw(dst, b, img, b.Min, draw.Src)
	}

	// 按 Rect 与 Stride 逐行遍历：go-captcha 的主图可能是更大画布的 SubImage
	b := dst.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		row := dst.Pix[dst.PixOffset(b.Min.X, y):dst.PixOffset(b.Max.X, y)]
		for i := 0; i+3 < len(row); i += 4 {
			if row[i+3] == 0 {
				continue
			}
			r, g, bl := float64(row[i]), float64(row[i+1]), float64(row[i+2])
			luma := 0.299*r + 0.587*g + 0.114*bl
			noise := rand.Uint32()
			row[i] = j.channel(luma+(r-luma)*j.saturation, noise)
			row[i+1] = j.channel(luma+(g-luma)*j.saturation, noise>>8)
			row[i+2] = j.channel(luma+(bl-luma)*j.saturation, noise>>16)
		}
	}
	return dst
}

// channel 对单个通道套用查找表并叠加噪点，noise 的低 8 位决定噪点值
func (j *imageJitter) channel(v float64, noise uint32) uint8 {
	n := int(noise&0xff)%(2*jitterNoise+1) - jitterNoise
	return clampUint8(float64(int(j.lut[clampUint8(v)]) + n))
}

// randSigned 返回 [-amplitude, amplitude) 内的均匀随机数
func randSigned(amplitude float64) float64 {
	return (rand.Float64()*2 - 1) * amplitude
}

func clampUint8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}
