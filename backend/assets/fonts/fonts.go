// Package fonts 内置验证码出图使用的字体。
//
// ZCOOLKuaiLe-Regular.ttf：站酷快乐体，SIL Open Font License 1.1（全文见同目录 OFL.txt），
// 取自 github.com/google/fonts 的 ofl/zcoolkuaile（commit a606d6f），原样嵌入、未做任何修改。
// 覆盖点选题字符集全部汉字，由 fonts_test.go 保证。
package fonts

import (
	_ "embed"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
)

//go:embed ZCOOLKuaiLe-Regular.ttf
var zcoolKuaiLe []byte

// ClickTextFont 返回点选文字题使用的字体
func ClickTextFont() (*truetype.Font, error) {
	return freetype.ParseFont(zcoolKuaiLe)
}
