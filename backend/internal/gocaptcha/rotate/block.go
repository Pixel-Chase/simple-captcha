// 本文件源自 github.com/wenlng/go-captcha v2.0.5（Apache-2.0），已修改，改动见 internal/gocaptcha/NOTICE.md

/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package rotate

// Block ..
type Block struct {
	// Deprecated: As of 2.1.0, it will be removed, please use [[CaptchaInstance].GetOptions().GetImageSize()].
	ParentWidth int `json:"parent_width"`
	// Deprecated: As of 2.1.0, it will be removed, please use [[CaptchaInstance].GetOptions().GetImageSize()].
	ParentHeight int `json:"parent_height"`
	Width        int `json:"width"`
	Height       int `json:"height"`
	Angle        int `json:"angle"`
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
