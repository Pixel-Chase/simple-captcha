package click

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateAppliesPaddingEvenly(t *testing.T) {
	// 目标框左上角 (100, 50)，宽 40、高 30，四边各放宽 10
	const dx, dy, w, h, p = 100, 50, 40, 30, 10
	cases := []struct {
		name   string
		sx, sy int
		want   bool
	}{
		{"框内中心", dx + w/2, dy + h/2, true},
		{"左侧容差边界", dx - p, dy + h/2, true},
		{"左侧超出容差", dx - p - 1, dy + h/2, false},
		{"右侧容差边界", dx + w + p, dy + h/2, true},
		{"右侧超出容差", dx + w + p + 1, dy + h/2, false},
		{"上方容差边界", dx + w/2, dy - p, true},
		{"上方超出容差", dx + w/2, dy - p - 1, false},
		{"下方容差边界", dx + w/2, dy + h + p, true},
		{"下方超出容差", dx + w/2, dy + h + p + 1, false},
		{"右下角容差边界", dx + w + p, dy + h + p, true},
		{"左上角容差边界", dx - p, dy - p, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, Validate(c.sx, c.sy, dx, dy, w, h, p))
		})
	}
}

func TestValidateWithoutPadding(t *testing.T) {
	assert.True(t, Validate(100, 50, 100, 50, 40, 30, 0))
	assert.True(t, Validate(140, 80, 100, 50, 40, 30, 0))
	assert.False(t, Validate(99, 50, 100, 50, 40, 30, 0))
	assert.False(t, Validate(141, 80, 100, 50, 40, 30, 0))
}
