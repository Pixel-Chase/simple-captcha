// 本文件源自 github.com/wenlng/go-captcha v2.0.5（Apache-2.0），已修改，改动见 internal/gocaptcha/NOTICE.md

/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package slide

import (
	"image"
)

// Block defines the block data for the slide CAPTCHA
type Block struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
	Angle  int `json:"angle"`
	// Display x,y
	DX int `json:"dx"`
	DY int `json:"dy"`
}

// DrawBlock defines the parameters for drawing slide CAPTCHA blocks
type DrawBlock struct {
	Block  *Block
	X      int
	Y      int
	Image  image.Image
	Width  int
	Height int
	Angle  int
}
