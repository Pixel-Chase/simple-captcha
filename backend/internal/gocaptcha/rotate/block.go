// 本文件源自 github.com/wenlng/go-captcha v2.0.5（Apache-2.0），已修改，改动见 internal/gocaptcha/NOTICE.md

/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package rotate

// Block ..
type Block struct {
	Width  int `json:"width"`
	Height int `json:"height"`
	Angle  int `json:"angle"`
}

// DrawBlock .
type DrawBlock struct {
	Block  *Block
	X      int
	Y      int
	Width  int
	Height int
	Angle  int
}
