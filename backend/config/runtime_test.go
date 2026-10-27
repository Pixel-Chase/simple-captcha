package config

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultRuntimeSettingsValid(t *testing.T) {
	d := DefaultRuntimeSettings()
	require.NoError(t, d.Validate())
	// 与原环境变量的默认值一致，风控观察保留条数改为 10000
	assert.Equal(t, 300, d.Captcha.ExpireSeconds)
	assert.Equal(t, 3, d.Captcha.DefaultDifficulty)
	assert.Equal(t, 500, d.Captcha.MinSolveTimeMs)
	assert.Equal(t, 14, d.Pow.Difficulty)
	assert.Equal(t, "enforce", d.Risk.Mode)
	assert.Equal(t, 10000, d.Risk.TelemetryRetention)
	assert.Equal(t, 600, d.Risk.FPReplayWindowSeconds)
}

// 23 个字段逐一检查上下界：边界值合法，越界一位即报错，且错误信息含字段名
func TestRuntimeSettingsBounds(t *testing.T) {
	type bound struct {
		field    string
		set      func(s *RuntimeSettings, v int)
		min, max int
	}
	intFields := []bound{
		{"captcha.expireSeconds", func(s *RuntimeSettings, v int) { s.Captcha.ExpireSeconds = v }, 30, 3600},
		{"captcha.secondVerifyExpireSeconds", func(s *RuntimeSettings, v int) { s.Captcha.SecondVerifyExpireSeconds = v }, 10, 3600},
		{"captcha.maxAttempts", func(s *RuntimeSettings, v int) { s.Captcha.MaxAttempts = v }, 1, 20},
		{"captcha.ipEscalatePerMinute", func(s *RuntimeSettings, v int) { s.Captcha.IPEscalatePerMinute = v }, 0, 100000},
		{"captcha.defaultDifficulty", func(s *RuntimeSettings, v int) { s.Captcha.DefaultDifficulty = v }, 1, 5},
		{"captcha.minSolveTimeMs", func(s *RuntimeSettings, v int) { s.Captcha.MinSolveTimeMs = v }, 0, 10000},
		{"captcha.imageQuality", func(s *RuntimeSettings, v int) { s.Captcha.ImageQuality = v }, 55, 100},
		{"captcha.maxScale", func(s *RuntimeSettings, v int) { s.Captcha.MaxScale = v }, 1, 3},
		// pow.difficulty 与 pow.maxDifficulty 互相约束：调其中一个时另一个取能满足约束的值
		{"pow.difficulty", func(s *RuntimeSettings, v int) { s.Pow.Difficulty = v; s.Pow.MaxDifficulty = 26 }, 1, 26},
		{"pow.maxDifficulty", func(s *RuntimeSettings, v int) { s.Pow.MaxDifficulty = v; s.Pow.Difficulty = 1 }, 1, 26},
		{"risk.escalateThreshold", func(s *RuntimeSettings, v int) { s.Risk.EscalateThreshold = v; s.Risk.BlockThreshold = 100 }, 0, 100},
		{"risk.blockThreshold", func(s *RuntimeSettings, v int) { s.Risk.BlockThreshold = v; s.Risk.EscalateThreshold = 0 }, 0, 100},
		{"risk.stateTtlSeconds", func(s *RuntimeSettings, v int) { s.Risk.StateTTLSeconds = v }, 60, 86400},
		{"risk.telemetryRetention", func(s *RuntimeSettings, v int) { s.Risk.TelemetryRetention = v }, 0, 1000000},
		{"risk.envScoreCap", func(s *RuntimeSettings, v int) { s.Risk.EnvScoreCap = v }, 0, 100},
		{"risk.blockMinBehavior", func(s *RuntimeSettings, v int) { s.Risk.BlockMinBehavior = v }, 0, 100},
		{"risk.fpReplayThreshold", func(s *RuntimeSettings, v int) { s.Risk.FPReplayThreshold = v }, 0, 1000},
		{"risk.fpReplayWindowSeconds", func(s *RuntimeSettings, v int) { s.Risk.FPReplayWindowSeconds = v }, 10, 86400},
	}
	for _, b := range intFields {
		t.Run(b.field, func(t *testing.T) {
			for _, ok := range []int{b.min, b.max} {
				s := DefaultRuntimeSettings()
				b.set(&s, ok)
				assert.NoError(t, s.Validate(), "边界值 %d 合法", ok)
			}
			for _, bad := range []int{b.min - 1, b.max + 1} {
				s := DefaultRuntimeSettings()
				b.set(&s, bad)
				err := s.Validate()
				require.Error(t, err, "越界值 %d", bad)
				assert.Contains(t, err.Error(), b.field)
			}
		})
	}

	t.Run("risk.mode", func(t *testing.T) {
		for _, ok := range []string{"enforce", "log"} {
			s := DefaultRuntimeSettings()
			s.Risk.Mode = ok
			assert.NoError(t, s.Validate())
		}
		for _, bad := range []string{"", "block", "ENFORCE"} {
			s := DefaultRuntimeSettings()
			s.Risk.Mode = bad
			assert.ErrorContains(t, s.Validate(), "risk.mode")
		}
	})

	// 布尔字段没有越界取值：两个值都合法
	bools := map[string]func(s *RuntimeSettings, v bool){
		"captcha.bindClient": func(s *RuntimeSettings, v bool) { s.Captcha.BindClient = v },
		"pow.enabled":        func(s *RuntimeSettings, v bool) { s.Pow.Enabled = v },
		"pow.required":       func(s *RuntimeSettings, v bool) { s.Pow.Required = v },
		"risk.enabled":       func(s *RuntimeSettings, v bool) { s.Risk.Enabled = v },
	}
	for field, set := range bools {
		t.Run(field, func(t *testing.T) {
			for _, v := range []bool{true, false} {
				s := DefaultRuntimeSettings()
				set(&s, v)
				assert.NoError(t, s.Validate())
			}
		})
	}
	assert.Len(t, intFields, 18)
	assert.Equal(t, 23, len(intFields)+1+len(bools), "覆盖全部 23 个字段")
}

func TestRuntimeSettingsCrossFieldRules(t *testing.T) {
	s := DefaultRuntimeSettings()
	s.Pow.Difficulty, s.Pow.MaxDifficulty = 18, 16
	assert.ErrorContains(t, s.Validate(), "pow.maxDifficulty")

	s = DefaultRuntimeSettings()
	s.Risk.EscalateThreshold, s.Risk.BlockThreshold = 90, 80
	assert.ErrorContains(t, s.Validate(), "risk.escalateThreshold")

	// 多个字段同时越界时全部报告
	s = DefaultRuntimeSettings()
	s.Captcha.MaxAttempts = 0
	s.Risk.EnvScoreCap = 101
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "captcha.maxAttempts")
	assert.Contains(t, err.Error(), "risk.envScoreCap")
}

func TestRuntimeSettingsSanitize(t *testing.T) {
	s := DefaultRuntimeSettings()
	s.Captcha.DefaultDifficulty = 9
	s.Captcha.MaxScale = 2 // 合法值保留
	s.Risk.EscalateThreshold, s.Risk.BlockThreshold = 95, 70
	reset := s.Sanitize()
	assert.ElementsMatch(t, []string{"captcha.defaultDifficulty", "risk.escalateThreshold"}, reset)
	assert.Equal(t, 3, s.Captcha.DefaultDifficulty)
	assert.Equal(t, 2, s.Captcha.MaxScale)
	assert.Equal(t, 55, s.Risk.EscalateThreshold)
	assert.Equal(t, 85, s.Risk.BlockThreshold)
	assert.NoError(t, s.Validate())
}

func TestApplyRuntimeReplacesSnapshot(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, ApplyRuntime(DefaultRuntimeSettings())) })

	before := Runtime()
	next := DefaultRuntimeSettings()
	next.Captcha.DefaultDifficulty = 4
	require.NoError(t, ApplyRuntime(next))
	after := Runtime()
	assert.Equal(t, 4, after.Captcha.DefaultDifficulty)
	assert.Equal(t, 3, before.Captcha.DefaultDifficulty, "旧快照不被原地修改")

	// 调用方改动传入的值不影响已生效的快照
	next.Captcha.DefaultDifficulty = 5
	assert.Equal(t, 4, Runtime().Captcha.DefaultDifficulty)

	bad := DefaultRuntimeSettings()
	bad.Captcha.MaxScale = 0
	assert.Error(t, ApplyRuntime(bad))
	assert.Equal(t, 4, Runtime().Captcha.DefaultDifficulty, "校验失败不替换")

	// 并发读写：读者只会看到完整的旧快照或新快照
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			s := DefaultRuntimeSettings()
			s.Captcha.DefaultDifficulty = 1 + i%5
			s.Captcha.MaxScale = 1 + i%3
			_ = ApplyRuntime(s)
		}(i)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				rt := Runtime()
				if rt.Captcha.DefaultDifficulty < 1 || rt.Captcha.MaxScale < 1 {
					t.Errorf("读到不完整的快照: %+v", rt.Captcha)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestRemovedStrategyEnvWarns(t *testing.T) {
	t.Setenv("CAPTCHA_DEFAULT_DIFFICULTY", "5")
	t.Setenv("RISK_MODE", "log")
	warnings := strings.Join(removedEnvWarnings(), "\n")
	assert.Contains(t, warnings, "CAPTCHA_DEFAULT_DIFFICULTY")
	assert.Contains(t, warnings, "RISK_MODE")
	assert.Contains(t, warnings, "策略配置")
	assert.NoError(t, validateConfig(loadFromEnv()), "旧变量只告警，不影响启动")
}
