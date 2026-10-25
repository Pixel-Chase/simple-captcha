package risk

import (
	"math"
	"sort"
)

// TraceFeatures 从交互轨迹中提取的统计特征
type TraceFeatures struct {
	Points           int     `json:"points"`
	DurationMs       float64 `json:"durationMs"`    // 首末采样点时间差
	InteractionMs    float64 `json:"interactionMs"` // 首次交互到提交的时间
	PrefaceMs        float64 `json:"prefaceMs"`     // 题目加载到首次交互的时间
	PathLength       float64 `json:"pathLength"`
	Displacement     float64 `json:"displacement"`
	Straightness     float64 `json:"straightness"` // 路径长度 / 位移，1 表示完全直线
	SpeedMean        float64 `json:"speedMean"`
	SpeedStd         float64 `json:"speedStd"`
	SpeedCV          float64 `json:"speedCV"` // 速度变异系数 std/mean
	DtMean           float64 `json:"dtMean"`
	DtStd            float64 `json:"dtStd"`
	DirectionChanges int     `json:"directionChanges"`
	Pauses           int     `json:"pauses"`
	Overshoot        bool    `json:"overshoot"` // 是否越过终点后回调（人类常见）
	ClickCount       int     `json:"clickCount"`
	ClickIntervalStd float64 `json:"clickIntervalStd"`
	MinClickInterval float64 `json:"minClickInterval"`
}

// Extract 计算轨迹特征。trace 为 nil 时返回零值。
func Extract(trace *Trace) TraceFeatures {
	f := TraceFeatures{}
	if trace == nil {
		return f
	}

	f.InteractionMs = trace.SubmittedAt - trace.StartedAt
	f.PrefaceMs = trace.StartedAt

	pts := sanitizePoints(trace.Points)
	f.Points = len(pts)
	if len(pts) >= 2 {
		first, last := pts[0], pts[len(pts)-1]
		f.DurationMs = last[2] - first[2]
		f.Displacement = math.Hypot(last[0]-first[0], last[1]-first[1])

		speeds := make([]float64, 0, len(pts)-1)
		dts := make([]float64, 0, len(pts)-1)
		var prevDx, prevDy float64
		maxX := first[0]
		for i := 1; i < len(pts); i++ {
			dx := pts[i][0] - pts[i-1][0]
			dy := pts[i][1] - pts[i-1][1]
			dt := pts[i][2] - pts[i-1][2]
			dist := math.Hypot(dx, dy)
			f.PathLength += dist
			dts = append(dts, dt)
			if dt > 0 {
				speeds = append(speeds, dist/dt)
			}
			// 停顿：间隔较长且几乎没有位移
			if dt >= 120 && dist < 2 {
				f.Pauses++
			}
			// 方向变化：相邻位移向量夹角超过 60°
			if i > 1 && dist > 0.5 && math.Hypot(prevDx, prevDy) > 0.5 {
				cos := (dx*prevDx + dy*prevDy) / (dist * math.Hypot(prevDx, prevDy))
				if cos < 0.5 {
					f.DirectionChanges++
				}
			}
			if dist > 0.5 {
				prevDx, prevDy = dx, dy
			}
			if pts[i][0] > maxX {
				maxX = pts[i][0]
			}
		}
		if f.Displacement > 0 {
			f.Straightness = f.PathLength / f.Displacement
		}
		f.SpeedMean, f.SpeedStd = meanStd(speeds)
		if f.SpeedMean > 0 {
			f.SpeedCV = f.SpeedStd / f.SpeedMean
		}
		f.DtMean, f.DtStd = meanStd(dts)
		// 越过终点再回调：最大 X 超过最终 X 一定距离
		f.Overshoot = maxX > last[0]+3
	}

	clicks := sanitizePoints(trace.Clicks)
	f.ClickCount = len(clicks)
	if len(clicks) >= 2 {
		intervals := make([]float64, 0, len(clicks)-1)
		f.MinClickInterval = math.MaxFloat64
		for i := 1; i < len(clicks); i++ {
			d := clicks[i][2] - clicks[i-1][2]
			intervals = append(intervals, d)
			if d < f.MinClickInterval {
				f.MinClickInterval = d
			}
		}
		_, f.ClickIntervalStd = meanStd(intervals)
	}

	return f
}

// sanitizePoints 过滤非法数值并按时间排序
func sanitizePoints(points [][3]float64) [][3]float64 {
	out := make([][3]float64, 0, len(points))
	for _, p := range points {
		if math.IsNaN(p[0]) || math.IsNaN(p[1]) || math.IsNaN(p[2]) ||
			math.IsInf(p[0], 0) || math.IsInf(p[1], 0) || math.IsInf(p[2], 0) {
			continue
		}
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i][2] < out[j][2] })
	return out
}

func meanStd(values []float64) (mean, std float64) {
	n := float64(len(values))
	if n == 0 {
		return 0, 0
	}
	for _, v := range values {
		mean += v
	}
	mean /= n
	for _, v := range values {
		d := v - mean
		std += d * d
	}
	std = math.Sqrt(std / n)
	return mean, std
}
