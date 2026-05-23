package services

type CaptchaData struct {
	Key    string `json:"key"`
	Image  string `json:"image"`
	Thumb  string `json:"thumb"`
	Tile   string `json:"tile"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
}
