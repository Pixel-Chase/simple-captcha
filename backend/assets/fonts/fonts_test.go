package fonts

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wenlng/go-captcha-assets/bindata/chars"
)

// 字体缺字时引擎画不出字形，题目里会出现空白或方框，用户无从点选
func TestClickTextFontCoversAllChars(t *testing.T) {
	font, err := ClickTextFont()
	require.NoError(t, err)

	var missing []string
	for _, c := range chars.GetChineseChars() {
		for _, r := range c {
			if font.Index(r) == 0 {
				missing = append(missing, c)
			}
		}
	}
	assert.Empty(t, missing, "点选字体缺少以下字符")
}
