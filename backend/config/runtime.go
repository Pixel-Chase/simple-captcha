package config

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"
)

// RuntimeSettings 验证码与风控策略（23 项）：存于 SQLite，在管理后台「策略配置」修改后立即生效。
// 各服务每次使用时通过 Runtime() 读取快照，不得在构造时保存副本。
// JSON 字段名带单位后缀，时长一律用整数秒或毫秒，不用 Go duration 字符串。
type RuntimeSettings struct {
	Captcha CaptchaSettings `json:"captcha"`
	Pow     PowSettings     `json:"pow"`
	Risk    RiskSettings    `json:"risk"`
}

// CaptchaSettings 出题与校验
type CaptchaSettings struct {
	ExpireSeconds             int  `json:"expireSeconds"`             // 题目有效期
	SecondVerifyExpireSeconds int  `json:"secondVerifyExpireSeconds"` // 二次验证 key 有效期
	MaxAttempts               int  `json:"maxAttempts"`               // 同一道题最多提交次数
	IPEscalatePerMinute       int  `json:"ipEscalatePerMinute"`       // 同一 IP 每分钟超过该次数后续题目加 1 级、超过 2 倍加 2 级，0 关闭；从不因 IP 拒绝
	DefaultDifficulty         int  `json:"defaultDifficulty"`         // 基础难度 1-5，风控状态在此基础上升级
	MinSolveTimeMs            int  `json:"minSolveTimeMs"`            // 出题到提交的最短间隔，低于该值判定为自动化提交
	BindClient                bool `json:"bindClient"`                // 题目绑定出题客户端（IP + UA 哈希）
	ImageQuality              int  `json:"imageQuality"`              // 主图 JPEG 质量
	MaxScale                  int  `json:"maxScale"`                  // 允许的最大出图倍率（对应前端 devicePixelRatio）
}

// PowSettings 工作量证明
type PowSettings struct {
	Enabled       bool `json:"enabled"`       // 是否下发 PoW 挑战
	Required      bool `json:"required"`      // 是否强制要求提交 PoW（false 时未提交只计入风险分）
	Difficulty    int  `json:"difficulty"`    // 基础难度：SHA-256 前导零位数
	MaxDifficulty int  `json:"maxDifficulty"` // 难度上限
}

// RiskSettings 行为与环境风控
type RiskSettings struct {
	Enabled               bool   `json:"enabled"`               // 是否启用行为 / 环境风控评分
	Mode                  string `json:"mode"`                  // enforce：按阈值拦截；log：只记录分数不拦截
	EscalateThreshold     int    `json:"escalateThreshold"`     // 达到该分数后提升该客户端后续题目的难度
	BlockThreshold        int    `json:"blockThreshold"`        // 达到该分数拒绝本次校验（enforce 模式）
	StateTTLSeconds       int    `json:"stateTtlSeconds"`       // 客户端风控状态保留时间
	TelemetryRetention    int    `json:"telemetryRetention"`    // 风控观察保留的最近记录条数，0 表示不记录
	EnvScoreCap           int    `json:"envScoreCap"`           // 环境 / 请求侧信号对风险分的贡献上限，0 不限制
	BlockMinBehavior      int    `json:"blockMinBehavior"`      // 触发拦截所需的最低行为分（命中硬性自动化标记时不要求）
	FPReplayThreshold     int    `json:"fpReplayThreshold"`     // 同一设备指纹在时间窗内对应的不同客户端数超过该值视为重放，0 关闭
	FPReplayWindowSeconds int    `json:"fpReplayWindowSeconds"` // 指纹重放计数的时间窗
}

// Expire 题目有效期
func (c CaptchaSettings) Expire() time.Duration { return time.Duration(c.ExpireSeconds) * time.Second }

// SecondVerifyExpire 二次验证 key 有效期
func (c CaptchaSettings) SecondVerifyExpire() time.Duration {
	return time.Duration(c.SecondVerifyExpireSeconds) * time.Second
}

// MinSolveTime 出题到提交的最短间隔
func (c CaptchaSettings) MinSolveTime() time.Duration {
	return time.Duration(c.MinSolveTimeMs) * time.Millisecond
}

// StateTTL 客户端风控状态保留时间
func (r RiskSettings) StateTTL() time.Duration { return time.Duration(r.StateTTLSeconds) * time.Second }

// FPReplayWindow 指纹重放计数的时间窗
func (r RiskSettings) FPReplayWindow() time.Duration {
	return time.Duration(r.FPReplayWindowSeconds) * time.Second
}

// DefaultRuntimeSettings 默认策略，与原先环境变量的默认值一致（风控观察保留条数为 10000）
func DefaultRuntimeSettings() RuntimeSettings {
	return RuntimeSettings{
		Captcha: CaptchaSettings{
			ExpireSeconds:             300,
			SecondVerifyExpireSeconds: 120,
			MaxAttempts:               5,
			IPEscalatePerMinute:       30,
			DefaultDifficulty:         3,
			MinSolveTimeMs:            500,
			BindClient:                true,
			ImageQuality:              80,
			MaxScale:                  3,
		},
		Pow: PowSettings{
			Enabled:       true,
			Required:      false,
			Difficulty:    14,
			MaxDifficulty: 20,
		},
		Risk: RiskSettings{
			Enabled:               true,
			Mode:                  "enforce",
			EscalateThreshold:     55,
			BlockThreshold:        85,
			StateTTLSeconds:       1800,
			TelemetryRetention:    10000,
			EnvScoreCap:           60,
			BlockMinBehavior:      20,
			FPReplayThreshold:     5,
			FPReplayWindowSeconds: 600,
		},
	}
}

// settingRule 单个字段（或一组相互约束的字段）的校验规则；reset 把相关字段恢复为默认值
type settingRule struct {
	field string
	check func(s *RuntimeSettings) error
	reset func(s *RuntimeSettings, d *RuntimeSettings)
}

func intRange(field string, get func(*RuntimeSettings) *int, min, max int) settingRule {
	return settingRule{
		field: field,
		check: func(s *RuntimeSettings) error {
			if v := *get(s); v < min || v > max {
				return fmt.Errorf("%s 必须在 %d～%d 之间，当前为 %d", field, min, max, v)
			}
			return nil
		},
		reset: func(s, d *RuntimeSettings) { *get(s) = *get(d) },
	}
}

// settingRules 全部校验规则：取值范围沿用原环境变量的校验，原先没有范围的字段补上合理上下界
var settingRules = []settingRule{
	intRange("captcha.expireSeconds", func(s *RuntimeSettings) *int { return &s.Captcha.ExpireSeconds }, 30, 3600),
	intRange("captcha.secondVerifyExpireSeconds", func(s *RuntimeSettings) *int { return &s.Captcha.SecondVerifyExpireSeconds }, 10, 3600),
	intRange("captcha.maxAttempts", func(s *RuntimeSettings) *int { return &s.Captcha.MaxAttempts }, 1, 20),
	intRange("captcha.ipEscalatePerMinute", func(s *RuntimeSettings) *int { return &s.Captcha.IPEscalatePerMinute }, 0, 100000),
	intRange("captcha.defaultDifficulty", func(s *RuntimeSettings) *int { return &s.Captcha.DefaultDifficulty }, 1, 5),
	intRange("captcha.minSolveTimeMs", func(s *RuntimeSettings) *int { return &s.Captcha.MinSolveTimeMs }, 0, 10000),
	intRange("captcha.imageQuality", func(s *RuntimeSettings) *int { return &s.Captcha.ImageQuality }, 55, 100),
	intRange("captcha.maxScale", func(s *RuntimeSettings) *int { return &s.Captcha.MaxScale }, 1, 3),
	intRange("pow.difficulty", func(s *RuntimeSettings) *int { return &s.Pow.Difficulty }, 1, 26),
	intRange("pow.maxDifficulty", func(s *RuntimeSettings) *int { return &s.Pow.MaxDifficulty }, 1, 26),
	{
		field: "pow.maxDifficulty",
		check: func(s *RuntimeSettings) error {
			if s.Pow.MaxDifficulty < s.Pow.Difficulty {
				return fmt.Errorf("pow.maxDifficulty 不能小于 pow.difficulty（%d）", s.Pow.Difficulty)
			}
			return nil
		},
		reset: func(s, d *RuntimeSettings) {
			s.Pow.Difficulty, s.Pow.MaxDifficulty = d.Pow.Difficulty, d.Pow.MaxDifficulty
		},
	},
	{
		field: "risk.mode",
		check: func(s *RuntimeSettings) error {
			if s.Risk.Mode != "enforce" && s.Risk.Mode != "log" {
				return fmt.Errorf("risk.mode 只能是 enforce 或 log，当前为 %q", s.Risk.Mode)
			}
			return nil
		},
		reset: func(s, d *RuntimeSettings) { s.Risk.Mode = d.Risk.Mode },
	},
	intRange("risk.escalateThreshold", func(s *RuntimeSettings) *int { return &s.Risk.EscalateThreshold }, 0, 100),
	intRange("risk.blockThreshold", func(s *RuntimeSettings) *int { return &s.Risk.BlockThreshold }, 0, 100),
	{
		field: "risk.escalateThreshold",
		check: func(s *RuntimeSettings) error {
			if s.Risk.EscalateThreshold > s.Risk.BlockThreshold {
				return fmt.Errorf("risk.escalateThreshold 不能大于 risk.blockThreshold（%d）", s.Risk.BlockThreshold)
			}
			return nil
		},
		reset: func(s, d *RuntimeSettings) {
			s.Risk.EscalateThreshold, s.Risk.BlockThreshold = d.Risk.EscalateThreshold, d.Risk.BlockThreshold
		},
	},
	intRange("risk.stateTtlSeconds", func(s *RuntimeSettings) *int { return &s.Risk.StateTTLSeconds }, 60, 86400),
	intRange("risk.telemetryRetention", func(s *RuntimeSettings) *int { return &s.Risk.TelemetryRetention }, 0, 1000000),
	intRange("risk.envScoreCap", func(s *RuntimeSettings) *int { return &s.Risk.EnvScoreCap }, 0, 100),
	intRange("risk.blockMinBehavior", func(s *RuntimeSettings) *int { return &s.Risk.BlockMinBehavior }, 0, 100),
	intRange("risk.fpReplayThreshold", func(s *RuntimeSettings) *int { return &s.Risk.FPReplayThreshold }, 0, 1000),
	intRange("risk.fpReplayWindowSeconds", func(s *RuntimeSettings) *int { return &s.Risk.FPReplayWindowSeconds }, 10, 86400),
}

// Validate 校验全部字段，返回所有不合法字段的错误（中文，含字段名与范围）
func (s RuntimeSettings) Validate() error {
	var errs []error
	for _, rule := range settingRules {
		if err := rule.check(&s); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Sanitize 把不满足当前校验规则的字段恢复为默认值，返回被恢复的字段名。
// 用于启动加载库中的旧值：规则收紧后旧值不至于让服务起不来
func (s *RuntimeSettings) Sanitize() []string {
	defaults := DefaultRuntimeSettings()
	var reset []string
	for _, rule := range settingRules {
		if rule.check(s) != nil {
			rule.reset(s, &defaults)
			reset = append(reset, rule.field)
		}
	}
	return reset
}

var runtimeSettings atomic.Pointer[RuntimeSettings]

func init() {
	defaults := DefaultRuntimeSettings()
	runtimeSettings.Store(&defaults)
}

// Runtime 当前生效的策略快照。快照不可变：调用方只读，不得修改返回值的字段；
// 同一次处理里只读取一次，保证前后使用同一组值
func Runtime() *RuntimeSettings {
	return runtimeSettings.Load()
}

// ApplyRuntime 校验后整体替换策略快照
func ApplyRuntime(s RuntimeSettings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	snapshot := s
	runtimeSettings.Store(&snapshot)
	return nil
}
