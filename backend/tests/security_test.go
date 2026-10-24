package tests

import (
	"encoding/json"
	"fmt"
	"regexp"
	"simple-captcha/config"
	"simple-captcha/helper"
	"simple-captcha/risk"
	"simple-captcha/services"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var hexKeyPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// slideAnswer 从缓存记录中读出滑块题的正确答案（图像像素坐标）
func slideAnswer(t *testing.T, key string) (string, *services.ChallengeMeta) {
	base := services.NewBaseService()
	record, err := base.GetCaptchaData(key)
	require.Nil(t, err)

	data, ok := record["data"].(map[string]interface{})
	require.True(t, ok)
	x, _ := data["x"].(float64)
	y, _ := data["y"].(float64)
	return fmt.Sprintf("%d,%d", int(x), int(y)), services.ParseMeta(record)
}

// solvePow 为指定题目计算 PoW 解答
func solvePow(t *testing.T, key string, meta *services.ChallengeMeta) *risk.PowSolution {
	if meta.PowBits == 0 {
		return nil
	}
	solution, ok := risk.SolvePow(meta.PowNonce, key, meta.PowBits, 0)
	require.True(t, ok)
	return &risk.PowSolution{Nonce: meta.PowNonce, Solution: solution}
}

func waitMinSolveTime() {
	time.Sleep(config.LoadConfig().Captcha.MinSolveTime + 100*time.Millisecond)
}

func TestGenerateKey_RandomAndIndependentOfAnswer(t *testing.T) {
	manager := services.GetServiceManager()
	service, _ := manager.GetCaptchaService("rotate")

	first, err := service.GenerateCaptcha(&services.ChallengeRequest{Type: "rotate"})
	require.Nil(t, err)
	second, err := service.GenerateCaptcha(&services.ChallengeRequest{Type: "rotate"})
	require.Nil(t, err)

	assert.Regexp(t, hexKeyPattern, first.Key)
	assert.Regexp(t, hexKeyPattern, second.Key)
	assert.NotEqual(t, first.Key, second.Key)

	// key 不能再等于答案的 MD5
	record, getErr := services.NewBaseService().GetCaptchaData(first.Key)
	require.Nil(t, getErr)
	answerBytes, _ := json.Marshal(record["data"])
	assert.NotEqual(t, helper.StringToMD5(string(answerBytes)), first.Key)
}

func TestVerify_ClientMismatchInvalidatesChallenge(t *testing.T) {
	manager := services.GetServiceManager()
	service, _ := manager.GetCaptchaService("slide-text")
	verifier, _ := manager.GetVerifyService("slide-text")

	data, err := service.GenerateCaptcha(&services.ChallengeRequest{Type: "slide-text", ClientHash: "client-a"})
	require.Nil(t, err)
	answer, meta := slideAnswer(t, data.Key)
	waitMinSolveTime()

	ok, _, verifyErr := verifier.VerifyData(&services.VerifyInput{
		Key: data.Key, Answer: answer, ClientHash: "client-b", Pow: solvePow(t, data.Key, meta),
	})
	assert.False(t, ok)
	require.NotNil(t, verifyErr)
	assert.Equal(t, helper.ErrCodeClientMismatch, verifyErr.Code)

	// 题目已作废，即使换回正确客户端也无法再用
	ok, _, verifyErr = verifier.VerifyData(&services.VerifyInput{
		Key: data.Key, Answer: answer, ClientHash: "client-a", Pow: solvePow(t, data.Key, meta),
	})
	assert.False(t, ok)
	require.NotNil(t, verifyErr)
	assert.Equal(t, helper.ErrCodeExpired, verifyErr.Code)
}

func TestVerify_TooFastRejectedThenSucceeds(t *testing.T) {
	manager := services.GetServiceManager()
	service, _ := manager.GetCaptchaService("slide-text")
	verifier, _ := manager.GetVerifyService("slide-text")

	data, err := service.GenerateCaptcha(&services.ChallengeRequest{Type: "slide-text", ClientHash: "client-fast"})
	require.Nil(t, err)
	answer, meta := slideAnswer(t, data.Key)
	pow := solvePow(t, data.Key, meta)

	// 立即提交：即使答案正确也应被时间窗口拦下，并计入一次失败
	ok, msg, verifyErr := verifier.VerifyData(&services.VerifyInput{Key: data.Key, Answer: answer, ClientHash: "client-fast", Pow: pow})
	assert.False(t, ok)
	require.NotNil(t, verifyErr)
	assert.Equal(t, helper.ErrCodeTooFast, verifyErr.Code)
	assert.Equal(t, helper.ErrTooFast.Message, msg)

	waitMinSolveTime()

	ok, secondKey, verifyErr := verifier.VerifyData(&services.VerifyInput{Key: data.Key, Answer: answer, ClientHash: "client-fast", Pow: pow})
	assert.Nil(t, verifyErr)
	assert.True(t, ok)
	assert.NotEmpty(t, secondKey)

	// 二次验证 key 一次性消费
	riskControl := manager.GetRiskControlService()
	assert.True(t, riskControl.ValidateSecondVerifyToken(secondKey))
	assert.False(t, riskControl.ValidateSecondVerifyToken(secondKey))
}

func TestVerify_PowInvalidRejected(t *testing.T) {
	manager := services.GetServiceManager()
	service, _ := manager.GetCaptchaService("slide-text")
	verifier, _ := manager.GetVerifyService("slide-text")

	data, err := service.GenerateCaptcha(&services.ChallengeRequest{Type: "slide-text"})
	require.Nil(t, err)
	require.Greater(t, data.PowBits, 0)
	answer, meta := slideAnswer(t, data.Key)
	waitMinSolveTime()

	ok, _, verifyErr := verifier.VerifyData(&services.VerifyInput{
		Key: data.Key, Answer: answer, Pow: &risk.PowSolution{Nonce: meta.PowNonce, Solution: "definitely-wrong"},
	})
	assert.False(t, ok)
	require.NotNil(t, verifyErr)
	assert.Equal(t, helper.ErrCodePowInvalid, verifyErr.Code)

	// nonce 被篡改同样拒绝
	solution, _ := risk.SolvePow("other-nonce", data.Key, meta.PowBits, 0)
	ok, _, verifyErr = verifier.VerifyData(&services.VerifyInput{
		Key: data.Key, Answer: answer, Pow: &risk.PowSolution{Nonce: "other-nonce", Solution: solution},
	})
	assert.False(t, ok)
	require.NotNil(t, verifyErr)
	assert.Equal(t, helper.ErrCodePowInvalid, verifyErr.Code)
}

func TestVerify_PowOptionalWhenNotRequired(t *testing.T) {
	cfg := config.LoadConfig()
	if cfg.Captcha.PowRequired {
		t.Skip("CAPTCHA_POW_REQUIRED=true，跳过可选 PoW 用例")
	}

	manager := services.GetServiceManager()
	service, _ := manager.GetCaptchaService("slide-text")
	verifier, _ := manager.GetVerifyService("slide-text")

	data, err := service.GenerateCaptcha(&services.ChallengeRequest{Type: "slide-text"})
	require.Nil(t, err)
	answer, _ := slideAnswer(t, data.Key)
	waitMinSolveTime()

	ok, _, verifyErr := verifier.VerifyData(&services.VerifyInput{Key: data.Key, Answer: answer})
	assert.Nil(t, verifyErr)
	assert.True(t, ok, "未强制 PoW 时旧版 SDK 仍可通过")
}

func TestVerify_WrongAnswerConsumesAttempts(t *testing.T) {
	manager := services.GetServiceManager()
	service, _ := manager.GetCaptchaService("slide-text")
	verifier, _ := manager.GetVerifyService("slide-text")

	data, err := service.GenerateCaptcha(&services.ChallengeRequest{Type: "slide-text"})
	require.Nil(t, err)
	_, meta := slideAnswer(t, data.Key)
	pow := solvePow(t, data.Key, meta)
	waitMinSolveTime()

	maxAttempts := config.LoadConfig().Captcha.MaxAttempts
	for i := 1; i < maxAttempts; i++ {
		ok, _, verifyErr := verifier.VerifyData(&services.VerifyInput{Key: data.Key, Answer: "1,1", Pow: pow})
		assert.False(t, ok)
		require.NotNil(t, verifyErr)
		assert.Equal(t, helper.ErrCodeVerifyFailed, verifyErr.Code, "第 %d 次", i)
	}

	ok, _, verifyErr := verifier.VerifyData(&services.VerifyInput{Key: data.Key, Answer: "1,1", Pow: pow})
	assert.False(t, ok)
	require.NotNil(t, verifyErr)
	assert.Equal(t, helper.ErrCodeTooManyAttempts, verifyErr.Code)
}

func TestVerify_ScaledChallengeUsesImageCoordinates(t *testing.T) {
	manager := services.GetServiceManager()
	service, _ := manager.GetCaptchaService("slide-text")
	verifier, _ := manager.GetVerifyService("slide-text")

	data, err := service.GenerateCaptcha(&services.ChallengeRequest{Type: "slide-text", Scale: 2, Difficulty: 3})
	require.Nil(t, err)
	assert.Equal(t, 600, data.ImageWidth)
	assert.Equal(t, 440, data.ImageHeight)
	assert.Equal(t, 2, data.Scale)

	answer, meta := slideAnswer(t, data.Key)
	assert.Equal(t, 2, meta.Scale)
	pow := solvePow(t, data.Key, meta)
	waitMinSolveTime()

	// 偏差 10 像素：难度 3 容差 6px × 倍率 2 = 12px，应通过
	var x, y int
	fmt.Sscanf(answer, "%d,%d", &x, &y)
	ok, _, verifyErr := verifier.VerifyData(&services.VerifyInput{Key: data.Key, Answer: fmt.Sprintf("%d,%d", x+10, y), Pow: pow})
	assert.Nil(t, verifyErr)
	assert.True(t, ok)
}

func TestRateLimiter_RedisSlidingWindow(t *testing.T) {
	cfg := config.LoadConfig()
	if cfg.Security.RateLimitBackend != "redis" {
		t.Skip("RATE_LIMIT_BACKEND != redis")
	}

	riskControl := services.NewRiskControlService()
	testIP := fmt.Sprintf("203.0.113.%d-%d", time.Now().UnixNano()%250, time.Now().UnixNano())

	for i := 0; i < cfg.Captcha.IPRateLimit; i++ {
		assert.True(t, riskControl.CheckIPLimit(testIP), "第 %d 次请求应放行", i+1)
	}
	assert.False(t, riskControl.CheckIPLimit(testIP), "超过上限应被拒绝")

	// 另一个 IP 不受影响
	assert.True(t, riskControl.CheckIPLimit(testIP+"-other"))
}

func TestClientRiskStore_EscalatesDifficulty(t *testing.T) {
	store := services.NewClientRiskStore()
	policy := services.NewChallengePolicy(store)
	clientHash := fmt.Sprintf("test-client-%d", time.Now().UnixNano())

	base := config.LoadConfig().Captcha.DefaultDifficulty
	assert.Equal(t, base, policy.DifficultyFor(clientHash))

	store.Record(clientHash, false, 10)
	store.Record(clientHash, false, 10)
	assert.Equal(t, services.ClampDifficulty(base+1), policy.DifficultyFor(clientHash), "连续失败 2 次升一级")

	store.Record(clientHash, false, 90)
	store.Record(clientHash, false, 90)
	assert.Equal(t, services.MaxDifficulty, policy.DifficultyFor(clientHash), "失败 4 次且高风险分应升到顶")

	state := store.Get(clientHash)
	assert.Equal(t, 4, state.Fails)
	assert.Equal(t, 90, state.LastScore)
}

func TestRiskEngine_BlocksObviousBot(t *testing.T) {
	cfg := config.LoadConfig()
	engine := services.NewRiskEngine(nil)
	if !engine.Enabled() {
		t.Skip("RISK_ENABLED=false")
	}

	result := engine.Evaluate(&services.EvaluationContext{
		Type: "slide-text",
		Env:  &risk.Env{Webdriver: true, Automation: []string{"cdc_adoQpoasnfa76pfcZLmcfl_Array"}},
		Trace: &risk.Trace{
			Points:      [][3]float64{{0, 0, 0}, {10, 0, 5}, {20, 0, 10}, {30, 0, 15}, {40, 0, 20}, {50, 0, 25}, {60, 0, 30}, {70, 0, 35}, {80, 0, 40}, {90, 0, 45}, {100, 0, 50}},
			Untrusted:   11,
			StartedAt:   1,
			SubmittedAt: 60,
		},
	})
	require.NotNil(t, result)
	assert.GreaterOrEqual(t, result.Score, cfg.Risk.BlockThreshold)
	if cfg.Risk.Mode == "enforce" {
		assert.True(t, engine.ShouldBlock(result))
	}
}
