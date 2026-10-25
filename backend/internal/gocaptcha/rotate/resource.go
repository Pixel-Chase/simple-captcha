// 本文件源自 github.com/wenlng/go-captcha v2.0.5（Apache-2.0），已修改，改动见 internal/gocaptcha/NOTICE.md

/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package rotate

import (
	"image"
)

// Resources defines the resources for the rotate CAPTCHA
type Resources struct {
	rangImages []image.Image
}

// NewResources .
func NewResources() *Resources {
	return &Resources{}
}

type Resource func(*Resources)

// WithImages is to set image
func WithImages(images []image.Image) Resource {
	return func(resources *Resources) {
		resources.rangImages = images
	}
}
