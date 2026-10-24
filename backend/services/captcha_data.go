package services

// CaptchaData Service 层生成的验证码数据，Controller 据此组装响应
type CaptchaData struct {
	Key   string `json:"key"`
	Image string `json:"image"`
	Thumb string `json:"thumb"` // 点选提示图 / 旋转内圆
	Tile  string `json:"tile"`  // 滑块拼图块

	// 滑块拼图块尺寸与初始位置（图像像素空间）
	Width  int `json:"width"`
	Height int `json:"height"`
	X      int `json:"x"`
	Y      int `json:"y"`

	// 主图位图尺寸（图像像素空间），前端据此把 CSS 坐标换算回图像坐标
	ImageWidth  int `json:"imageWidth"`
	ImageHeight int `json:"imageHeight"`
	// 点选提示图 / 旋转内圆的位图尺寸
	ThumbWidth  int `json:"thumbWidth"`
	ThumbHeight int `json:"thumbHeight"`

	Scale      int `json:"scale"`
	Difficulty int `json:"difficulty"`

	// PoW 挑战（PowBits 为 0 表示本题未要求）
	PowNonce string `json:"powNonce,omitempty"`
	PowBits  int    `json:"powBits,omitempty"`
}
