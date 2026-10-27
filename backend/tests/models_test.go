package tests

import (
	"simple-captcha/models"
	"simple-captcha/risk"
	"strings"
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
			assert.Nil(t, err, "Expected no error for valid request")
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
			assert.NotNil(t, err)
		})
	}
}

func TestCaptchaRequest_Validate_InvalidMode(t *testing.T) {
	req := models.CaptchaRequest{
		Type: "click-text",
		Mode: "invalid-mode",
	}

	err := req.Validate()
	assert.NotNil(t, err)
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
			assert.Nil(t, err)
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
			assert.NotNil(t, err)
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
	assert.NotNil(t, err)
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
			assert.NotNil(t, err)
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
			assert.Nil(t, err)
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
			assert.NotNil(t, err)
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

func TestIssueRequest_Validate(t *testing.T) {
	valid := models.IssueRequest{Type: "auto", Mode: "dark", Dpr: 2}
	assert.Nil(t, valid.Validate())
	assert.Equal(t, 2, valid.Scale())

	noBody := models.IssueRequest{Type: "slide-text"}
	assert.Nil(t, noBody.Validate(), "不带 mode / dpr / env 也合法")
	assert.Equal(t, 1, noBody.Scale())

	badMode := models.IssueRequest{Type: "click-text", Mode: "blue"}
	assert.NotNil(t, badMode.Validate())

	badDpr := models.IssueRequest{Type: "click-text", Dpr: 4}
	assert.NotNil(t, badDpr.Validate())

	badType := models.IssueRequest{Type: "unknown"}
	assert.NotNil(t, badType.Validate())

	tooManyLanguages := models.IssueRequest{Type: "rotate", Env: &risk.Env{Languages: make([]string, 33)}}
	assert.NotNil(t, tooManyLanguages.Validate(), "语言列表超过 32 项应被拒绝")

	longField := models.IssueRequest{Type: "rotate", Env: &risk.Env{UA: strings.Repeat("u", 513)}}
	assert.NotNil(t, longField.Validate(), "UA 超过 512 字节应被拒绝")

	okEnv := models.IssueRequest{Type: "rotate", Env: &risk.Env{UA: "Mozilla/5.0", Languages: []string{"zh-CN"}}}
	assert.Nil(t, okEnv.Validate())
}

func TestIssueRequest_Validate_Anomalies(t *testing.T) {
	sixteen := make([]string, 16)
	for i := range sixteen {
		sixteen[i] = "zero_outer_size"
	}
	ok := models.IssueRequest{Type: "rotate", Env: &risk.Env{Anomalies: sixteen}}
	assert.Nil(t, ok.Validate(), "16 个标记在上限内")

	tooMany := models.IssueRequest{Type: "rotate", Env: &risk.Env{Anomalies: make([]string, 17)}}
	assert.NotNil(t, tooMany.Validate(), "17 个标记应被拒绝")

	tooLong := models.IssueRequest{Type: "rotate", Env: &risk.Env{Anomalies: []string{strings.Repeat("a", 65)}}}
	assert.NotNil(t, tooLong.Validate(), "单个标记超过 64 字节应被拒绝")

	verify := models.VerifyRequest{Type: "slide-text", Key: "k", Point: "1,1", Env: &risk.Env{Anomalies: make([]string, 17)}}
	assert.NotNil(t, verify.Validate(), "校验接口同样受上限约束")
}
