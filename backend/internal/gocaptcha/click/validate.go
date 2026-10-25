// 本文件源自 github.com/wenlng/go-captcha v2.0.5（Apache-2.0），已修改，改动见 internal/gocaptcha/NOTICE.md

/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package click

// Validate checks if a click point is within the specified area
// params:
//   - sx, sy: Coordinates of the click point
//   - dx, dy: Top-left coordinates of the target area
//   - width, height: Width and height of the target area
//   - padding: Padding of the area, applied evenly on all four sides
//
// return: Whether the point is within the area
//
// 上游以 max(dx, dx-padding) 计算左上角，结果恒为 dx，导致容差全部堆到右侧与下方：
// 有效区间是 [dx, dx+width+2p] 而非 [dx-p, dx+width+p]。这里改为四边各放宽 padding。
func Validate(sx, sy, dx, dy, width, height, padding int) bool {
	newWidth := width + (padding * 2)
	newHeight := height + (padding * 2)
	newDx := dx - padding
	newDy := dy - padding

	return sx >= newDx &&
		sx <= newDx+newWidth &&
		sy >= newDy &&
		sy <= newDy+newHeight
}
