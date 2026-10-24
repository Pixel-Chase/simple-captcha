package services

import (
	"github.com/wenlng/go-captcha/v2/base/option"
)

// 难度与倍率边界
const (
	MinDifficulty = 1
	MaxDifficulty = 5
	MinScale      = 1
	MaxScale      = 3
)

// 1 倍率下的基础出图尺寸，与前端 SDK 的默认渲染尺寸一致
const (
	baseImageWidth  = 300
	baseImageHeight = 220
	baseThumbWidth  = 150
	baseThumbHeight = 40
	baseRotateSize  = 220
)

// DifficultyProfile 单个难度等级的出题参数，全部按 1 倍率定义，使用时按 Scale 放大
type DifficultyProfile struct {
	Level int

	// 点选
	ClickLen       option.RangeVal   // 图中字符/图形总数
	ClickVerifyLen option.RangeVal   // 需要依次点选的数量
	ClickFontSize  option.RangeVal   // 字号 / 图形尺寸
	ClickAngles    []option.RangeVal // 倾斜角度区间
	ThumbDistort   int               // 提示图背景扭曲等级
	ThumbCircles   int               // 提示图干扰圆点数量
	ThumbSlimLines int               // 提示图干扰细线数量
	ClickTolerance int               // 点选容差（像素）

	// 滑块
	SlideGraphSize option.RangeVal // 拼图块尺寸区间
	SlideTolerance int             // 滑块容差（像素）

	// 旋转
	RotateAngles    []option.RangeVal // 初始旋转角度区间
	RotateTolerance int               // 旋转容差（度）

	// 在基础 PoW 难度上附加的前导零位数。
	// 实测纯 JS SHA-256：14 位约 80ms、16 位约 0.4s、18 位约 1.3s（桌面端），移动端约慢 2-3 倍，
	// 且 PoW 在题目加载后即后台计算，通常在用户完成操作前已算完。
	PowExtraBits int
}

var difficultyProfiles = map[int]DifficultyProfile{
	1: {
		Level:           1,
		ClickLen:        option.RangeVal{Min: 3, Max: 4},
		ClickVerifyLen:  option.RangeVal{Min: 2, Max: 2},
		ClickFontSize:   option.RangeVal{Min: 28, Max: 34},
		ClickAngles:     []option.RangeVal{{Min: 0, Max: 20}, {Min: 340, Max: 360}},
		ThumbDistort:    option.DistortLevel1,
		ThumbCircles:    8,
		ThumbSlimLines:  0,
		ClickTolerance:  30,
		SlideGraphSize:  option.RangeVal{Min: 60, Max: 70},
		SlideTolerance:  8,
		RotateAngles:    []option.RangeVal{{Min: 30, Max: 330}},
		RotateTolerance: 8,
		PowExtraBits:    0,
	},
	2: {
		Level:           2,
		ClickLen:        option.RangeVal{Min: 4, Max: 5},
		ClickVerifyLen:  option.RangeVal{Min: 2, Max: 3},
		ClickFontSize:   option.RangeVal{Min: 26, Max: 32},
		ClickAngles:     []option.RangeVal{{Min: 10, Max: 35}, {Min: 325, Max: 350}},
		ThumbDistort:    option.DistortLevel2,
		ThumbCircles:    16,
		ThumbSlimLines:  1,
		ClickTolerance:  27,
		SlideGraphSize:  option.RangeVal{Min: 55, Max: 65},
		SlideTolerance:  7,
		RotateAngles:    []option.RangeVal{{Min: 25, Max: 335}},
		RotateTolerance: 7,
		PowExtraBits:    0,
	},
	3: {
		Level:          3,
		ClickLen:       option.RangeVal{Min: 4, Max: 6},
		ClickVerifyLen: option.RangeVal{Min: 2, Max: 4},
		ClickFontSize:  option.RangeVal{Min: 26, Max: 32},
		ClickAngles: []option.RangeVal{
			{Min: 20, Max: 35}, {Min: 35, Max: 45}, {Min: 45, Max: 60},
			{Min: 290, Max: 305}, {Min: 305, Max: 325}, {Min: 325, Max: 340},
		},
		ThumbDistort:    option.DistortLevel4,
		ThumbCircles:    24,
		ThumbSlimLines:  2,
		ClickTolerance:  25,
		SlideGraphSize:  option.RangeVal{Min: 50, Max: 60},
		SlideTolerance:  6,
		RotateAngles:    []option.RangeVal{{Min: 20, Max: 340}},
		RotateTolerance: 6,
		PowExtraBits:    1,
	},
	4: {
		Level:          4,
		ClickLen:       option.RangeVal{Min: 5, Max: 7},
		ClickVerifyLen: option.RangeVal{Min: 3, Max: 4},
		ClickFontSize:  option.RangeVal{Min: 24, Max: 30},
		ClickAngles: []option.RangeVal{
			{Min: 15, Max: 40}, {Min: 40, Max: 70},
			{Min: 290, Max: 320}, {Min: 320, Max: 345},
		},
		ThumbDistort:    option.DistortLevel5,
		ThumbCircles:    30,
		ThumbSlimLines:  3,
		ClickTolerance:  20,
		SlideGraphSize:  option.RangeVal{Min: 45, Max: 55},
		SlideTolerance:  5,
		RotateAngles:    []option.RangeVal{{Min: 15, Max: 345}},
		RotateTolerance: 5,
		PowExtraBits:    3,
	},
	5: {
		Level:          5,
		ClickLen:       option.RangeVal{Min: 6, Max: 8},
		ClickVerifyLen: option.RangeVal{Min: 3, Max: 5},
		ClickFontSize:  option.RangeVal{Min: 22, Max: 28},
		ClickAngles: []option.RangeVal{
			{Min: 15, Max: 40}, {Min: 40, Max: 75},
			{Min: 285, Max: 320}, {Min: 320, Max: 345},
		},
		ThumbDistort:    option.DistortLevel5,
		ThumbCircles:    36,
		ThumbSlimLines:  4,
		ClickTolerance:  16,
		SlideGraphSize:  option.RangeVal{Min: 40, Max: 50},
		SlideTolerance:  4,
		RotateAngles:    []option.RangeVal{{Min: 15, Max: 345}},
		RotateTolerance: 4,
		PowExtraBits:    4,
	},
}

// ClampDifficulty 将难度裁剪到合法区间，0 或非法值回落到 3
func ClampDifficulty(level int) int {
	if level < MinDifficulty || level > MaxDifficulty {
		if level <= 0 {
			return 3
		}
		if level < MinDifficulty {
			return MinDifficulty
		}
		return MaxDifficulty
	}
	return level
}

// ClampScale 将倍率裁剪到 [1, maxScale]，maxScale 本身也受全局上限约束
func ClampScale(scale, maxScale int) int {
	if maxScale < MinScale || maxScale > MaxScale {
		maxScale = MaxScale
	}
	if scale < MinScale {
		return MinScale
	}
	if scale > maxScale {
		return maxScale
	}
	return scale
}

// GetDifficultyProfile 获取难度等级对应的出题参数
func GetDifficultyProfile(level int) DifficultyProfile {
	return difficultyProfiles[ClampDifficulty(level)]
}

// scaleRange 按倍率放大区间
func scaleRange(r option.RangeVal, scale int) option.RangeVal {
	return option.RangeVal{Min: r.Min * scale, Max: r.Max * scale}
}

// scaleInts 按倍率放大整数序列
func scaleInts(values []int, scale int) []int {
	out := make([]int, len(values))
	for i, v := range values {
		out[i] = v * scale
	}
	return out
}

// imageSize 返回按倍率放大的主图尺寸
func imageSize(scale int) option.Size {
	return option.Size{Width: baseImageWidth * scale, Height: baseImageHeight * scale}
}

// thumbSize 返回按倍率放大的点选提示图尺寸
func thumbSize(scale int) option.Size {
	return option.Size{Width: baseThumbWidth * scale, Height: baseThumbHeight * scale}
}

// ClickToleranceFor 点选容差（已按倍率换算到图像像素空间）
func ClickToleranceFor(meta *ChallengeMeta) float64 {
	return float64(GetDifficultyProfile(meta.Difficulty).ClickTolerance * meta.Scale)
}

// SlideToleranceFor 滑块容差（已按倍率换算）
func SlideToleranceFor(meta *ChallengeMeta) float64 {
	return float64(GetDifficultyProfile(meta.Difficulty).SlideTolerance * meta.Scale)
}

// RotateToleranceFor 旋转容差（角度，与倍率无关）
func RotateToleranceFor(meta *ChallengeMeta) float64 {
	return float64(GetDifficultyProfile(meta.Difficulty).RotateTolerance)
}
