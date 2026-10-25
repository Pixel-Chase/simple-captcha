// 本文件源自 github.com/wenlng/go-captcha v2.0.5（Apache-2.0），已修改，改动见 internal/gocaptcha/NOTICE.md

/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package slide

// Validate checks if the point position is within the specified range
// params:
//   - sx: Source X coordinate
//   - sy: Source Y coordinate
//   - dx: Target X coordinate
//   - dy: Target Y coordinate
//   - padding: Padding
//
// return: Whether within range
func Validate(sx, sy, dx, dy, padding int) bool {
	newX := padding * 2
	newY := padding * 2
	newDx := dx - padding
	newDy := dy - padding

	return sx >= newDx &&
		sx <= newDx+newX &&
		sy >= newDy &&
		sy <= newDy+newY
}

// Deprecated: As of 2.1.0, it will be removed, please use [slide.Validate]
func CheckPoint(sx, sy, dx, dy, padding int64) bool {
	newX := padding * 2
	newY := padding * 2
	newDx := dx - padding
	newDy := dy - padding

	return sx >= newDx &&
		sx <= newDx+newX &&
		sy >= newDy &&
		sy <= newDy+newY
}
