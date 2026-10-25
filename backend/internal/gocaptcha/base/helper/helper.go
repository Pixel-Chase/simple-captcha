// 本文件源自 github.com/wenlng/go-captcha v2.0.5（Apache-2.0），已修改，改动见 internal/gocaptcha/NOTICE.md

/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package helper

import (
	"errors"
	"image/color"
	"math"
	"unicode"
	"unicode/utf8"

	"simple-captcha/internal/gocaptcha/base/random"
)

// FormatAlpha formats the alpha value
func FormatAlpha(val float32) uint8 {
	a := math.Min(float64(val), 1)
	alpha := a * 255
	return uint8(alpha)
}

var (
	ErrColorHexFormat = errors.New("hex color must start with '#'")
	ErrColorInvalid   = errors.New("hexToByte component invalid")
)

// ParseHexColor converts a hex color to an RGBA color
func ParseHexColor(s string) (c color.RGBA, err error) {
	c.A = 0xff
	if s[0] != '#' {
		return c, ErrColorHexFormat
	}

	hexToByte := func(b byte) byte {
		switch {
		case b >= '0' && b <= '9':
			return b - '0'
		case b >= 'a' && b <= 'f':
			return b - 'a' + 10
		case b >= 'A' && b <= 'F':
			return b - 'A' + 10
		}
		err = ErrColorInvalid
		return 0
	}

	switch len(s) {
	case 7:
		c.R = hexToByte(s[1])<<4 + hexToByte(s[2])
		c.G = hexToByte(s[3])<<4 + hexToByte(s[4])
		c.B = hexToByte(s[5])<<4 + hexToByte(s[6])

	case 4:
		c.R = hexToByte(s[1]) * 17
		c.G = hexToByte(s[2]) * 17
		c.B = hexToByte(s[3]) * 17
	default:
		err = ErrColorInvalid
	}
	return
}

// InArrayWithStr checks if a string is in an array
func InArrayWithStr(items []string, s string) bool {
	for _, item := range items {
		if item == s {
			return true
		}
	}
	return false
}

// IsChineseChar checks if a string contains Chinese characters
func IsChineseChar(str string) bool {
	for _, r := range str {
		if unicode.Is(unicode.Scripts["Han"], r) {
			return true
		}
	}
	return false
}

// LenChineseChar calculates the character length of a string (supports Chinese)
func LenChineseChar(str string) int {
	return utf8.RuneCountInString(str)
}

// RandIndex generates a random index
func RandIndex(length int) int {
	if length == 0 {
		return -1
	}

	index := random.RandIntFast(0, length-1)
	if index >= length {
		index = length - 1
	}

	return index
}
