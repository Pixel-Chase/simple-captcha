// 本文件源自 github.com/wenlng/go-captcha v2.0.5（Apache-2.0），已修改，改动见 internal/gocaptcha/NOTICE.md

/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package rotate

import (
	"simple-captcha/internal/gocaptcha/base/option"
)

// defaultOptions .
func defaultOptions() Option {
	return func(opts *Options) {
		opts.imageSquareSize = 220
		opts.rangeAnglePos = []*option.RangeVal{
			{Min: 30, Max: 330},
		}

		opts.thumbImageAlpha = 1
		opts.rangeThumbImageSquareSize = []int{140, 150, 160, 170}
	}
}

// defaultResource .
func defaultResource() Resource {
	return func(resources *Resources) {
		// ...
	}
}
