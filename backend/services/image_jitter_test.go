package services

import (
	"image"
	"image/color"
	"testing"
)

func filledNRGBA(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

func TestImageJitter_PreservesAlphaAndSkipsTransparent(t *testing.T) {
	img := filledNRGBA(8, 4, color.NRGBA{R: 100, G: 150, B: 200, A: 255})
	for y := 0; y < 4; y++ {
		img.SetNRGBA(0, y, color.NRGBA{R: 10, G: 20, B: 30, A: 0})
		img.SetNRGBA(1, y, color.NRGBA{R: 10, G: 20, B: 30, A: 128})
	}

	out := newImageJitter().apply(img).(*image.NRGBA)
	if out.Bounds() != img.Bounds() {
		t.Fatalf("bounds changed: %v", out.Bounds())
	}
	for y := 0; y < 4; y++ {
		if got := out.NRGBAAt(0, y); got != (color.NRGBA{R: 10, G: 20, B: 30, A: 0}) {
			t.Errorf("fully transparent pixel should be untouched, got %v", got)
		}
		if got := out.NRGBAAt(1, y).A; got != 128 {
			t.Errorf("alpha should be preserved, got %d", got)
		}
	}
}

func TestImageJitter_BoundedAndDiffersPerChallenge(t *testing.T) {
	gray := color.NRGBA{R: 128, G: 128, B: 128, A: 255}
	first := newImageJitter().apply(filledNRGBA(64, 64, gray)).(*image.NRGBA)
	second := newImageJitter().apply(filledNRGBA(64, 64, gray)).(*image.NRGBA)

	// 中灰像素的最大偏移：亮度 10 + 对比度 6%×128 + 噪点 3，再加取整
	const maxShift = 22
	differs := false
	for i := 0; i < len(first.Pix); i += 4 {
		for c := 0; c < 3; c++ {
			if d := absDiff(first.Pix[i+c], 128); d > maxShift {
				t.Fatalf("channel shifted by %d, want <= %d", d, maxShift)
			}
			if first.Pix[i+c] != second.Pix[i+c] {
				differs = true
			}
		}
	}
	if !differs {
		t.Error("two challenges from the same background should not be pixel-identical")
	}
}

func TestImageJitter_SharedParamsKeepColorsConsistent(t *testing.T) {
	// 主图与拼图块共用一个 imageJitter：同一像素只差各自的噪点
	c := color.NRGBA{R: 60, G: 140, B: 220, A: 255}
	jitter := newImageJitter()
	master := jitter.apply(filledNRGBA(32, 32, c)).(*image.NRGBA)
	tile := jitter.apply(filledNRGBA(32, 32, c)).(*image.NRGBA)
	for i := 0; i < len(master.Pix); i += 4 {
		for ch := 0; ch < 3; ch++ {
			if d := absDiff(master.Pix[i+ch], tile.Pix[i+ch]); d > 2*jitterNoise {
				t.Fatalf("shared jitter should only differ by noise, got %d", d)
			}
		}
	}
}

func TestImageJitter_SubImageOnlyTouchesItsRect(t *testing.T) {
	gray := color.NRGBA{R: 128, G: 128, B: 128, A: 255}
	base := filledNRGBA(10, 10, gray)
	sub := base.SubImage(image.Rect(2, 2, 6, 6)).(*image.NRGBA)

	out := newImageJitter().apply(sub)
	if out.Bounds() != image.Rect(2, 2, 6, 6) {
		t.Fatalf("bounds changed: %v", out.Bounds())
	}
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			if (image.Point{X: x, Y: y}).In(sub.Bounds()) {
				continue
			}
			if got := base.NRGBAAt(x, y); got != gray {
				t.Fatalf("pixel (%d,%d) outside the sub image changed to %v", x, y, got)
			}
		}
	}
}

func TestImageJitter_ConvertsOtherImageTypes(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	out := newImageJitter().apply(src)
	if _, ok := out.(*image.NRGBA); !ok {
		t.Fatalf("expected *image.NRGBA, got %T", out)
	}
	if out.Bounds() != src.Bounds() {
		t.Fatalf("bounds changed: %v", out.Bounds())
	}
}
