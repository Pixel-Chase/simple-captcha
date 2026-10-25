// Package risk 包含与具体存储无关的人机判别逻辑：
// 前端遥测数据结构、轨迹特征提取、规则评分以及工作量证明（PoW）校验。
package risk

// Trace 前端 SDK 在验证过程中采集的交互轨迹。
// 所有时间均为相对"题目加载完成"时刻的毫秒数。
type Trace struct {
	// Points 指针移动采样点 [x, y, t]，坐标相对验证码容器左上角（CSS 像素）
	Points [][3]float64 `json:"points"`
	// Clicks 点选模式下的点击 [x, y, t]
	Clicks [][3]float64 `json:"clicks,omitempty"`
	// PointerType 首次按下时的指针类型：mouse / touch / pen
	PointerType string `json:"pointerType,omitempty"`
	// StartedAt 首次交互（pointerdown）时刻
	StartedAt float64 `json:"startedAt"`
	// SubmittedAt 提交答案时刻
	SubmittedAt float64 `json:"submittedAt"`
	// Downs / Ups / Moves 原始事件计数（Moves 为降采样前的数量）
	Downs int `json:"downs"`
	Ups   int `json:"ups"`
	Moves int `json:"moves"`
	// Untrusted isTrusted == false 的事件数量（脚本合成事件）
	Untrusted int `json:"untrusted"`
}

// Env 前端采集的运行环境信号。所有字段均可为空，缺失本身也是一种信号。
type Env struct {
	UA                string   `json:"ua,omitempty"`
	Platform          string   `json:"platform,omitempty"`
	Languages         []string `json:"languages,omitempty"`
	Timezone          string   `json:"tz,omitempty"`
	TzOffset          int      `json:"tzOffset"`
	Screen            [2]int   `json:"screen"`
	Viewport          [2]int   `json:"viewport"`
	DPR               float64  `json:"dpr"`
	TouchPoints       int      `json:"touchPoints"`
	Cores             int      `json:"cores"`
	Memory            float64  `json:"memory"`
	Webdriver         bool     `json:"webdriver"`
	Plugins           int      `json:"plugins"`
	WebGL             string   `json:"webgl,omitempty"`
	Automation        []string `json:"automation,omitempty"`
	Fingerprint       string   `json:"fp,omitempty"`
	VisibilityChanges int      `json:"visibilityChanges"`
	Mobile            *bool    `json:"mobile,omitempty"`
}

// PowSolution 前端提交的工作量证明
type PowSolution struct {
	Nonce    string `json:"nonce"`
	Solution string `json:"solution"`
}

// Input 风控评分输入
type Input struct {
	// Type 验证码类型，用于区分拖动类与点选类的行为预期
	Type  string
	Trace *Trace
	Env   *Env
	// PowExpected 本题是否下发了 PoW；PowProvided 客户端是否携带了 PoW
	PowExpected bool
	PowProvided bool
}

// Result 评分结果，Score 越高越像机器（0-100）
type Result struct {
	Score    int            `json:"score"`
	Reasons  []string       `json:"reasons"`
	Features *TraceFeatures `json:"features,omitempty"`
}

// IsDragType 判断该类型是否需要拖动操作（滑块、旋转），否则为点选
func IsDragType(captchaType string) bool {
	switch captchaType {
	case "slide-text", "slide-region", "rotate":
		return true
	}
	return false
}
