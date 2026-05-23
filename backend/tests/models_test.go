package tests

import (
	"simple-captcha/models"
	"testing"

	"github.com/stretchr/testify/assert"
)

// 测试验证码请求验证
func TestCaptchaRequest_Validate_Success(t *testing.T) {
	tests := []struct {
		name string
		req  models.CaptchaRequest
	}{
		{
			name: "click-text with light mode",
			req: models.CaptchaRequest{
				Type: "click-text",
				Mode: "light",
			},
		},
		{
			name: "click-shape with dark mode",
			req: models.CaptchaRequest{
				Type: "click-shape",
				Mode: "dark",
			},
		},
		{
			name: "rotate without mode",
			req: models.CaptchaRequest{
				Type: "rotate",
			},
		},
		{
			name: "slide-text with light mode",
			req: models.CaptchaRequest{
				Type: "slide-text",
				Mode: "light",
			},
		},
		{
			name: "slide-region with dark mode",
			req: models.CaptchaRequest{
				Type: "slide-region",
				Mode: "dark",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			assert.NoError(t, err, "Expected no error for valid request")
		})
	}
}

func TestCaptchaRequest_Validate_InvalidType(t *testing.T) {
	tests := []struct {
		name string
		req  models.CaptchaRequest
	}{
		{
			name: "invalid type",
			req: models.CaptchaRequest{
				Type: "invalid-type",
				Mode: "light",
			},
		},
		{
			name: "empty type",
			req: models.CaptchaRequest{
				Type: "",
				Mode: "light",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			assert.Error(t, err)
		})
	}
}

func TestCaptchaRequest_Validate_InvalidMode(t *testing.T) {
	req := models.CaptchaRequest{
		Type: "click-text",
		Mode: "invalid-mode",
	}

	err := req.Validate()
	assert.Error(t, err)
}

// 测试验证请求验证
func TestVerifyRequest_Validate_Success(t *testing.T) {
	tests := []struct {
		name string
		req  models.VerifyRequest
	}{
		{
			name: "click-text with dots",
			req: models.VerifyRequest{
				Type: "click-text",
				Key:  "test-key",
				Dots: "100,200,300,400",
			},
		},
		{
			name: "click-shape with dots",
			req: models.VerifyRequest{
				Type: "click-shape",
				Key:  "test-key",
				Dots: "150,250",
			},
		},
		{
			name: "rotate with angle",
			req: models.VerifyRequest{
				Type:  "rotate",
				Key:   "test-key",
				Angle: "90",
			},
		},
		{
			name: "slide-text with point",
			req: models.VerifyRequest{
				Type:  "slide-text",
				Key:   "test-key",
				Point: "100,200",
			},
		},
		{
			name: "slide-region with point",
			req: models.VerifyRequest{
				Type:  "slide-region",
				Key:   "test-key",
				Point: "150,250",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			assert.NoError(t, err)
		})
	}
}

func TestVerifyRequest_Validate_MissingParams(t *testing.T) {
	tests := []struct {
		name string
		req  models.VerifyRequest
	}{
		{
			name: "click-text missing dots",
			req: models.VerifyRequest{
				Type: "click-text",
				Key:  "test-key",
				// Dots 为空
			},
		},
		{
			name: "click-shape missing dots",
			req: models.VerifyRequest{
				Type: "click-shape",
				Key:  "test-key",
				// Dots 为空
			},
		},
		{
			name: "rotate missing angle",
			req: models.VerifyRequest{
				Type: "rotate",
				Key:  "test-key",
				// Angle 为空
			},
		},
		{
			name: "slide-text missing point",
			req: models.VerifyRequest{
				Type: "slide-text",
				Key:  "test-key",
				// Point 为空
			},
		},
		{
			name: "slide-region missing point",
			req: models.VerifyRequest{
				Type: "slide-region",
				Key:  "test-key",
				// Point 为空
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			assert.Error(t, err)
		})
	}
}

func TestVerifyRequest_Validate_InvalidType(t *testing.T) {
	req := models.VerifyRequest{
		Type: "invalid-type",
		Key:  "test-key",
		Dots: "100,200",
	}

	err := req.Validate()
	assert.Error(t, err)
}

func TestVerifyRequest_Validate_InvalidKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{
			name: "empty key",
			key:  "",
		},
		{
			name: "too long key",
			key:  string(make([]byte, 101)), // 超过100字符
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := models.VerifyRequest{
				Type: "click-text",
				Key:  tt.key,
				Dots: "100,200",
			}

			err := req.Validate()
			assert.Error(t, err)
		})
	}
}

// 测试状态请求验证
func TestStateRequest_Validate_Success(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{
			name: "normal key",
			key:  "test-key-123",
		},
		{
			name: "single character key",
			key:  "a",
		},
		{
			name: "max length key",
			key:  string(make([]byte, 100)), // 100字符
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 填充key内容
			if len(tt.key) == 100 {
				for i := range []byte(tt.key) {
					[]byte(tt.key)[i] = 'a'
				}
			}

			req := models.StateRequest{
				Key: tt.key,
			}

			err := req.Validate()
			assert.NoError(t, err)
		})
	}
}

func TestStateRequest_Validate_InvalidKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{
			name: "empty key",
			key:  "",
		},
		{
			name: "too long key",
			key:  string(make([]byte, 101)), // 超过100字符
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 填充key内容
			if len(tt.key) == 101 {
				for i := range []byte(tt.key) {
					[]byte(tt.key)[i] = 'a'
				}
			}

			req := models.StateRequest{
				Key: tt.key,
			}

			err := req.Validate()
			assert.Error(t, err)
		})
	}
}

// 基准测试
func BenchmarkCaptchaRequest_Validate(b *testing.B) {
	req := models.CaptchaRequest{
		Type: "click-text",
		Mode: "light",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req.Validate()
	}
}

func BenchmarkVerifyRequest_Validate(b *testing.B) {
	req := models.VerifyRequest{
		Type: "click-text",
		Key:  "test-key",
		Dots: "100,200,300,400",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req.Validate()
	}
}

func BenchmarkStateRequest_Validate(b *testing.B) {
	req := models.StateRequest{
		Key: "test-key-123",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req.Validate()
	}
}
