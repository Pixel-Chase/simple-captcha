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
	// 确保测试运行在正确的工作目录
	if _, err := os.Stat("images"); os.IsNotExist(err) {
		// 如果当前目录没有images文件夹，尝试切换到项目根目录
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

	// 添加中间件
	r.Use(middleware.ErrorHandlerMiddleware())

	// 注册路由
	captchaGroup := r.Group("/cgi/captcha")
	captchaGroup.GET("/:type",
		middleware.ValidateCaptchaRequest(),
		controllers.GetCaptchaHandler)
	captchaGroup.POST("/:type/:key",
		middleware.ValidateVerifyRequest(),
		controllers.VerifyCaptchaHandler)
	captchaGroup.GET("/second/:key/state",
		middleware.ValidateStateRequest(),
		controllers.CaptchaStateHandler)

	return r
}

func TestGetCaptchaHandler_ValidRequest(t *testing.T) {
	router := setupTestRouter()

	// 测试有效的验证码类型
	req, _ := http.NewRequest("GET", "/cgi/captcha/click-text", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 验证响应状态码
	assert.Equal(t, http.StatusOK, w.Code)

	// 解析响应
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// 验证响应格式
	assert.Contains(t, response, "errcode")
	assert.Contains(t, response, "errmsg")
	assert.Contains(t, response, "success")
}

func TestGetCaptchaHandler_InvalidType(t *testing.T) {
	router := setupTestRouter()

	// 测试无效的验证码类型
	req, _ := http.NewRequest("GET", "/cgi/captcha/invalid-type", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 验证响应状态码
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 解析响应
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// 验证错误响应
	assert.Equal(t, false, response["success"])
	assert.NotEqual(t, 0, response["errcode"])
}

func TestVerifyCaptchaHandler_MissingParams(t *testing.T) {
	router := setupTestRouter()

	// 测试缺少参数的请求
	requestBody := models.VerifyRequest{
		// 故意留空，测试验证逻辑
	}

	jsonData, _ := json.Marshal(requestBody)
	req, _ := http.NewRequest("POST", "/cgi/captcha/click-text/test-key", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 验证响应状态码
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 解析响应
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// 验证错误响应
	assert.Equal(t, false, response["success"])
}

func TestCaptchaStateHandler_InvalidKey(t *testing.T) {
	router := setupTestRouter()

	// 测试无效的key
	req, _ := http.NewRequest("GET", "/cgi/captcha/second/invalid-key/state", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 验证响应状态码
	assert.Equal(t, http.StatusOK, w.Code)

	// 解析响应
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// 验证错误响应
	assert.Equal(t, false, response["success"])
}

func TestValidateCaptchaRequest_Success(t *testing.T) {
	req := &models.CaptchaRequest{
		Type: "click-text",
		Mode: "light",
	}

	err := req.Validate()
	assert.NoError(t, err)
}

func TestValidateCaptchaRequest_InvalidType(t *testing.T) {
	req := &models.CaptchaRequest{
		Type: "invalid-type",
		Mode: "light",
	}

	err := req.Validate()
	assert.Error(t, err)
}

func TestValidateVerifyRequest_Success(t *testing.T) {
	req := &models.VerifyRequest{
		Type: "click-text",
		Key:  "test-key",
		Dots: "100,200,300,400",
	}

	err := req.Validate()
	assert.NoError(t, err)
}

func TestValidateVerifyRequest_MissingDots(t *testing.T) {
	req := &models.VerifyRequest{
		Type: "click-text",
		Key:  "test-key",
		// Dots 为空，应该验证失败
	}

	err := req.Validate()
	assert.Error(t, err)
}

func TestValidateVerifyRequest_RotateSuccess(t *testing.T) {
	req := &models.VerifyRequest{
		Type:  "rotate",
		Key:   "test-key",
		Angle: "90",
	}

	err := req.Validate()
	assert.NoError(t, err)
}

func TestValidateVerifyRequest_SlideSuccess(t *testing.T) {
	req := &models.VerifyRequest{
		Type:  "slide-text",
		Key:   "test-key",
		Point: "100,200",
	}

	err := req.Validate()
	assert.NoError(t, err)
}

func TestValidateStateRequest_Success(t *testing.T) {
	req := &models.StateRequest{
		Key: "test-key-123",
	}

	err := req.Validate()
	assert.NoError(t, err)
}

func TestValidateStateRequest_EmptyKey(t *testing.T) {
	req := &models.StateRequest{
		Key: "",
	}

	err := req.Validate()
	assert.Error(t, err)
}

func TestValidateStateRequest_TooLongKey(t *testing.T) {
	// 创建一个超过100字符的key
	longKey := make([]byte, 101)
	for i := range longKey {
		longKey[i] = 'a'
	}

	req := &models.StateRequest{
		Key: string(longKey),
	}

	err := req.Validate()
	assert.Error(t, err)
}

// 测试中间件验证逻辑
func TestMiddlewareValidation(t *testing.T) {
	router := setupTestRouter()

	// 测试无效的验证码类型
	req, _ := http.NewRequest("GET", "/cgi/captcha/invalid-type", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 验证响应状态码
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 解析响应
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// 验证错误响应
	assert.Equal(t, false, response["success"])
	assert.NotEqual(t, float64(0), response["errcode"])
}

func TestMiddlewareValidation_MissingParams(t *testing.T) {
	router := setupTestRouter()

	// 测试缺少参数的请求
	requestBody := map[string]interface{}{
		// 故意留空，测试验证逻辑
	}

	jsonData, _ := json.Marshal(requestBody)
	req, _ := http.NewRequest("POST", "/cgi/captcha/click-text/test-key", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 验证响应状态码
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 解析响应
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// 验证错误响应
	assert.Equal(t, false, response["success"])
}

// 基准测试
func BenchmarkValidateRequest(b *testing.B) {
	req := &models.CaptchaRequest{
		Type: "click-text",
		Mode: "light",
	}

	for i := 0; i < b.N; i++ {
		req.Validate()
	}
}

func BenchmarkGetCaptchaHandler(b *testing.B) {
	router := setupTestRouter()

	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("GET", "/cgi/captcha/click-text", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}
