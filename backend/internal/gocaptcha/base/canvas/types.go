// 本文件源自 github.com/wenlng/go-captcha v2.0.5（Apache-2.0），已修改，改动见 internal/gocaptcha/NOTICE.md

/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package canvas

import (
	"image/color"

	"github.com/golang/freetype/truetype"
)

// AreaRect struct for defining a rectangular area
type AreaRect struct {
	MinX, MaxX, MinY, MaxY int
}

// PositionRect struct for defining a rectangle's position and size
type PositionRect struct {
	X, Y, Width, Height int
}

// DrawStringParams struct for string drawing parameters
type DrawStringParams struct {
	Color   color.Color
	Size    int
	Width   int
	Height  int
	FontDPI int
	Text    string
	Font    *truetype.Font
}
