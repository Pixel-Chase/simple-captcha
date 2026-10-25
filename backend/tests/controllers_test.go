package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"simple-captcha/controllers"
	"simple-captcha/middleware"
	"simple-captcha/models"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func init() {
	// 确保测试运行在正确的工作目录（需要 images 目录）
	if _, err := os.Stat("images"); os.IsNotExist(err) {
		if wd, err := os.Getwd(); err == nil {
			projectRoot := filepath.Dir(wd)
			if _, err := os.Stat(filepath.Join(projectRoot, "images")); err == nil {
				os.Chdir(projectRoot)
			}
		}
	}
}

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandlerMiddleware())

	captchaGroup := r.Group("/api/v1/captchas")
	captchaGroup.GET("/:type",
		middleware.ValidateCaptchaRequest(),
		controllers.GetCaptchaHandler)
	captchaGroup.POST("/:type/:key/attempts",
		middleware.ValidateVerifyRequest(),
		controllers.VerifyCaptchaHandler)
	captchaGroup.GET("/states/:key",
		middleware.ValidateStateRequest(),
		controllers.ValidateCaptchaHandler)

	return r
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	return response
}

func TestGetCaptchaHandler_ValidRequest(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/api/v1/captchas/click-text?mode=dark", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "no-store, no-cache, must-revalidate", w.Header().Get("Cache-Control"))

	response := decodeBody(t, w)
	assert.Equal(t, "click-text", response["type"])
	assert.Len(t, response["key"], 32, "key 应为 32 位十六进制随机串")
	assert.NotEmpty(t, response["image"])
	assert.NotEmpty(t, response["thumb"])
	// 未带 dpr 时按最低出图倍率 2x 下发 3:2 画布
	assert.EqualValues(t, 600, response["imageWidth"])
	assert.EqualValues(t, 400, response["imageHeight"])
	assert.EqualValues(t, 2, response["scale"])
	assert.Contains(t, response, "pow", "默认应下发 PoW 挑战")
}

func TestGetCaptchaHandler_ScaleFromDpr(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/api/v1/captchas/slide-text?dpr=3", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	response := decodeBody(t, w)
	assert.EqualValues(t, 3, response["scale"])
	assert.EqualValues(t, 900, response["imageWidth"])
	assert.EqualValues(t, 600, response["imageHeight"])
	// 滑块拼图块尺寸也应随倍率放大：所有难度等级在 1x 下的区间为 40-70 像素
	assert.GreaterOrEqual(t, response["thumbWidth"], float64(120))
	assert.LessOrEqual(t, response["thumbWidth"], float64(210))
}

func TestGetCaptchaHandler_AutoTypeResolvedByServer(t *testing.T) {
	router := setupTestRouter()
	allowed := map[string]bool{"click-text": true, "click-shape": true, "slide-text": true, "slide-region": true, "rotate": true}

	for i := 0; i < 5; i++ {
		req, _ := http.NewRequest("GET", "/api/v1/captchas/auto", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		response := decodeBody(t, w)
		assert.True(t, allowed[response["type"].(string)], "auto 应被解析为具体题型: %v", response["type"])
	}
}

func TestGetCaptchaHandler_InvalidType(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/api/v1/captchas/invalid-type", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	response := decodeBody(t, w)
	assert.EqualValues(t, 400, response["code"])
	assert.NotEmpty(t, response["error"])
}

func TestVerifyCaptchaHandler_MissingParams(t *testing.T) {
	router := setupTestRouter()

	jsonData, _ := json.Marshal(map[string]interface{}{})
	req, _ := http.NewRequest("POST", "/api/v1/captchas/click-text/test-key/attempts", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	response := decodeBody(t, w)
	assert.EqualValues(t, 400, response["code"])
}

func TestVerifyCaptchaHandler_BodyCannotOverrideKey(t *testing.T) {
	router := setupTestRouter()

	// 请求体里的 key/type 不应覆盖 URI 参数
	jsonData, _ := json.Marshal(map[string]interface{}{"dots": "1,2", "key": "another", "type": "rotate"})
	req, _ := http.NewRequest("POST", "/api/v1/captchas/click-text/not-exist-key/attempts", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	response := decodeBody(t, w)
	assert.EqualValues(t, 1004, response["code"], "不存在的 key 应返回已失效")
}

func TestVerifyCaptchaHandler_OversizedTraceRejected(t *testing.T) {
	router := setupTestRouter()

	points := make([][3]float64, 601)
	jsonData, _ := json.Marshal(map[string]interface{}{
		"point": "1,2",
		"trace": map[string]interface{}{"points": points},
	})
	req, _ := http.NewRequest("POST", "/api/v1/captchas/slide-text/some-key/attempts", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCaptchaStateHandler_InvalidKey(t *testing.T) {
	router := setupTestRouter()

	req, _ := http.NewRequest("GET", "/api/v1/captchas/states/invalid-key", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	response := decodeBody(t, w)
	assert.EqualValues(t, 1004, response["code"])
}

func TestValidateCaptchaRequest_Success(t *testing.T) {
	req := &models.CaptchaRequest{Type: "click-text", Mode: "light"}
	assert.Nil(t, req.Validate())

	auto := &models.CaptchaRequest{Type: "auto"}
	assert.Nil(t, auto.Validate())
}

func TestValidateCaptchaRequest_InvalidType(t *testing.T) {
	req := &models.CaptchaRequest{Type: "invalid-type", Mode: "light"}
	assert.NotNil(t, req.Validate())
}

func TestCaptchaRequest_Scale(t *testing.T) {
	cases := map[float64]int{0: 1, 1: 1, 1.5: 2, 2: 2, 2.6: 3, 3: 3, 9: 3}
	for dpr, expected := range cases {
		req := &models.CaptchaRequest{Dpr: dpr}
		assert.Equal(t, expected, req.Scale(), "dpr=%v", dpr)
	}
}

func TestValidateVerifyRequest_Success(t *testing.T) {
	req := &models.VerifyRequest{Type: "click-text", Key: "test-key", Dots: "100,200,300,400"}
	assert.Nil(t, req.Validate())
}

func TestValidateVerifyRequest_MissingDots(t *testing.T) {
	req := &models.VerifyRequest{Type: "click-text", Key: "test-key"}
	assert.NotNil(t, req.Validate())
}

func TestValidateVerifyRequest_RotateSuccess(t *testing.T) {
	req := &models.VerifyRequest{Type: "rotate", Key: "test-key", Angle: "90"}
	assert.Nil(t, req.Validate())
}

func TestValidateVerifyRequest_SlideSuccess(t *testing.T) {
	req := &models.VerifyRequest{Type: "slide-text", Key: "test-key", Point: "100,200"}
	assert.Nil(t, req.Validate())
}

func TestValidateStateRequest_Success(t *testing.T) {
	req := &models.StateRequest{Key: "test-key-123"}
	assert.Nil(t, req.Validate())
}

func TestValidateStateRequest_EmptyKey(t *testing.T) {
	req := &models.StateRequest{Key: ""}
	assert.NotNil(t, req.Validate())
}

func TestValidateStateRequest_TooLongKey(t *testing.T) {
	longKey := make([]byte, 101)
	for i := range longKey {
		longKey[i] = 'a'
	}
	req := &models.StateRequest{Key: string(longKey)}
	assert.NotNil(t, req.Validate())
}

func TestMiddlewareValidation_MissingParams(t *testing.T) {
	router := setupTestRouter()

	jsonData, _ := json.Marshal(map[string]interface{}{})
	req, _ := http.NewRequest("POST", "/api/v1/captchas/click-text/test-key/attempts", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	response := decodeBody(t, w)
	assert.EqualValues(t, 400, response["code"])
}

// 基准测试
func BenchmarkValidateRequest(b *testing.B) {
	req := &models.CaptchaRequest{Type: "click-text", Mode: "light"}
	for i := 0; i < b.N; i++ {
		req.Validate()
	}
}

func BenchmarkGetCaptchaHandler(b *testing.B) {
	router := setupTestRouter()

	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("GET", "/api/v1/captchas/click-text", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}
