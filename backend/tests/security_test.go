package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"simple-captcha/cache"
	"simple-captcha/config"
	"simple-captcha/helper"
	"simple-captcha/models"
	"simple-captcha/risk"
	"simple-captcha/routes"
	"simple-captcha/services"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testRouter     *gin.Engine
	testRouterOnce sync.Once
)

// router 构建真实路由（含全部中间件），并额外挂一个回显 ClientIP 的调试路由
func router() *gin.Engine {
	testRouterOnce.Do(func() {
		gin.SetMode(gin.TestMode)
		testRouter = routes.SetupRouter()
		testRouter.GET("/__client_ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })
	})
	return testRouter
}

// botTelemetry 一组足以触发拦截阈值的机器特征
func botTelemetry() (*risk.Env, *risk.Trace) {
	return &risk.Env{Webdriver: true, Automation: []string{"cdc_adoQpoasnfa76pfcZLmcfl_Array"}},
		&risk.Trace{
			Points:      [][3]float64{{0, 0, 0}, {10, 0, 5}, {20, 0, 10}, {30, 0, 15}, {40, 0, 20}, {50, 0, 25}, {60, 0, 30}, {70, 0, 35}, {80, 0, 40}, {90, 0, 45}, {100, 0, 50}},
			Untrusted:   11,
			StartedAt:   1,
			SubmittedAt: 60,
		}
}

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
	rt := config.Runtime()
	time.Sleep(rt.Captcha.MinSolveTime() + 100*time.Millisecond)
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

	const appID = "app_aaaaaaaaaaaa"
	data, err := service.GenerateCaptcha(&services.ChallengeRequest{Type: "slide-text", ClientHash: "client-fast", AppID: appID})
	require.Nil(t, err)
	answer, meta := slideAnswer(t, data.Key)
	pow := solvePow(t, data.Key, meta)

	// 立即提交：即使答案正确也应被时间窗口拦下，并计入一次失败
	ok, msg, verifyErr := verifier.VerifyData(&services.VerifyInput{Key: data.Key, Answer: answer, ClientHash: "client-fast", Pow: pow, AppID: appID})
	assert.False(t, ok)
	require.NotNil(t, verifyErr)
	assert.Equal(t, helper.ErrCodeTooFast, verifyErr.Code)
	assert.Equal(t, helper.ErrTooFast.Message, msg)

	waitMinSolveTime()

	ok, secondKey, verifyErr := verifier.VerifyData(&services.VerifyInput{Key: data.Key, Answer: answer, ClientHash: "client-fast", Pow: pow, AppID: appID})
	assert.Nil(t, verifyErr)
	assert.True(t, ok)
	assert.NotEmpty(t, secondKey)

	// 二次验证 key 一次性消费
	riskControl := manager.GetRiskControlService()
	// 二次验证 key 属于出题的应用，一次性消费
	assert.True(t, riskControl.ConsumeSecondVerifyToken(secondKey, appID))
	assert.False(t, riskControl.ConsumeSecondVerifyToken(secondKey, appID))
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
	rt := config.Runtime()
	if rt.Pow.Required {
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
	rt := config.Runtime()
	manager := services.GetServiceManager()
	service, _ := manager.GetCaptchaService("slide-text")
	verifier, _ := manager.GetVerifyService("slide-text")

	data, err := service.GenerateCaptcha(&services.ChallengeRequest{Type: "slide-text"})
	require.Nil(t, err)
	_, meta := slideAnswer(t, data.Key)
	pow := solvePow(t, data.Key, meta)
	waitMinSolveTime()

	maxAttempts := rt.Captcha.MaxAttempts
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

// 出题后修改尝试次数上限只影响之后出的题：已出的题从入口检查到失败计数都按出题时的上限，
// 不会出现"正确答案还能通过、错误答案却被立即作废"的两套标准
func TestVerify_MaxAttemptsFixedAtIssue(t *testing.T) {
	tests := []struct {
		name           string
		issued, latest int
	}{
		{"出题后下调", 5, 1},
		{"出题后上调", 2, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restoreRuntime(t)
			setMaxAttempts := func(n int) {
				next := *config.Runtime()
				next.Captcha.MaxAttempts = n
				require.NoError(t, config.ApplyRuntime(next))
			}
			manager := services.GetServiceManager()
			service, _ := manager.GetCaptchaService("slide-text")
			verifier, _ := manager.GetVerifyService("slide-text")
			wrong := func(key string, pow *risk.PowSolution) *helper.CaptchaError {
				ok, _, verifyErr := verifier.VerifyData(&services.VerifyInput{Key: key, Answer: "1,1", Pow: pow})
				assert.False(t, ok)
				require.NotNil(t, verifyErr)
				return verifyErr
			}

			setMaxAttempts(tt.issued)
			exhausted, err := service.GenerateCaptcha(&services.ChallengeRequest{Type: "slide-text"})
			require.Nil(t, err)
			passing, err := service.GenerateCaptcha(&services.ChallengeRequest{Type: "slide-text"})
			require.Nil(t, err)
			setMaxAttempts(tt.latest)

			exhaustedAnswer, exhaustedMeta := slideAnswer(t, exhausted.Key)
			exhaustedPow := solvePow(t, exhausted.Key, exhaustedMeta)
			passingAnswer, passingMeta := slideAnswer(t, passing.Key)
			passingPow := solvePow(t, passing.Key, passingMeta)
			waitMinSolveTime()

			// 错满出题时的上限：前 issued-1 次是普通失败，第 issued 次作废题目，之后正确答案也无法通过
			for i := 1; i < tt.issued; i++ {
				assert.Equal(t, helper.ErrCodeVerifyFailed, wrong(exhausted.Key, exhaustedPow).Code, "第 %d 次", i)
			}
			assert.Equal(t, helper.ErrCodeTooManyAttempts, wrong(exhausted.Key, exhaustedPow).Code, "第 %d 次", tt.issued)
			ok, _, verifyErr := verifier.VerifyData(&services.VerifyInput{Key: exhausted.Key, Answer: exhaustedAnswer, Pow: exhaustedPow})
			assert.False(t, ok)
			require.NotNil(t, verifyErr)
			assert.Equal(t, helper.ErrCodeExpired, verifyErr.Code)

			// 错 issued-1 次后，正确答案仍能通过
			for i := 1; i < tt.issued; i++ {
				assert.Equal(t, helper.ErrCodeVerifyFailed, wrong(passing.Key, passingPow).Code, "第 %d 次", i)
			}
			ok, _, verifyErr = verifier.VerifyData(&services.VerifyInput{Key: passing.Key, Answer: passingAnswer, Pow: passingPow})
			assert.Nil(t, verifyErr)
			assert.True(t, ok)
		})
	}
}

func TestVerify_ScaledChallengeUsesImageCoordinates(t *testing.T) {
	manager := services.GetServiceManager()
	service, _ := manager.GetCaptchaService("slide-text")
	verifier, _ := manager.GetVerifyService("slide-text")

	data, err := service.GenerateCaptcha(&services.ChallengeRequest{Type: "slide-text", Scale: 2, Difficulty: 3})
	require.Nil(t, err)
	assert.Equal(t, 600, data.ImageWidth)
	assert.Equal(t, 400, data.ImageHeight)
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

func TestRateLimiter_SlidingWindow(t *testing.T) {
	cfg := config.LoadConfig()
	if cfg.Admin.IPLimitPerMin <= 0 {
		t.Skip("ADMIN_IP_LIMIT_PER_MINUTE 为 0，管理员限流已关闭")
	}

	riskControl := services.NewRiskControlService()
	testIP := fmt.Sprintf("203.0.113.%d-%d", time.Now().UnixNano()%250, time.Now().UnixNano())

	// 管理员接口按 ADMIN_IP_LIMIT_PER_MINUTE 严格限流
	for i := 0; i < cfg.Admin.IPLimitPerMin; i++ {
		assert.True(t, riskControl.CheckAdminIP(testIP), "第 %d 次请求应放行", i+1)
	}
	assert.False(t, riskControl.CheckAdminIP(testIP), "超过上限应被拒绝")

	// 验证码请求与管理员接口分开计数，互不挤占
	assert.Equal(t, 1, riskControl.TrackCaptchaIP(testIP))

	// 另一个 IP 不受影响
	assert.True(t, riskControl.CheckAdminIP(testIP+"-other"))
}

// 同一出口 IP 的验证码请求量只让题目变难，任何请求量都不拒绝；业务后端的 S2S 状态查询不计入出口 IP
func TestRouter_SharedEgressEscalatesInsteadOfBlocking(t *testing.T) {
	rt := config.Runtime()
	threshold := rt.Captcha.IPEscalatePerMinute
	if threshold <= 0 {
		t.Skip("CAPTCHA_IP_ESCALATE_PER_MINUTE 为 0，按 IP 加难已关闭")
	}

	const egressIP = "198.51.100.77"
	bucket := services.CaptchaRateLimitKey(egressIP)
	mem := cache.Shared()
	mem.Del(cache.KindRateLimit, bucket)
	t.Cleanup(func() { mem.Del(cache.KindRateLimit, bucket) })

	// 直接往窗口里预填请求记录，模拟同一出口下很多人在同一分钟里出题
	seed := func(n int) {
		mem.Del(cache.KindRateLimit, bucket)
		for i := 0; i < n; i++ {
			mem.Hit(bucket, time.Minute, n+1)
		}
	}
	// 用一个全新的 UA 后缀，保证客户端历史状态为空，难度只受出口 IP 请求量影响；
	// 请求头按真实浏览器补齐，否则请求头核验会额外加难
	userAgent := browserUA(fmt.Sprintf("shared-egress-test/%d", time.Now().UnixNano()))
	send := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Origin", "http://"+req.Host)
		req.RemoteAddr = egressIP + ":4321"
		withBrowserHeaders(req, userAgent)
		w := httptest.NewRecorder()
		router().ServeHTTP(w, req)
		return w
	}
	// 出题只有 POST；空请求体即不携带环境信号；以管理后台系统应用的身份同源访问
	issue := func(path string) *httptest.ResponseRecorder {
		return send(http.MethodPost, path+"?appId="+services.ConsoleAppID)
	}
	get := func(path string) *httptest.ResponseRecorder { return send(http.MethodGet, path) }
	levelOf := func(w *httptest.ResponseRecorder) int {
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		record, err := services.NewBaseService().GetCaptchaData(body["key"].(string))
		require.Nil(t, err)
		return services.ParseMeta(record).Difficulty
	}

	// 每次请求自身也会计入窗口：预填 n 条后实际计数为 n+1
	const path = "/api/v1/captchas/slide-text"
	base := services.ClampDifficulty(rt.Captcha.DefaultDifficulty)

	seed(0)
	assert.Equal(t, base, levelOf(issue(path)), "低于加难阈值时保持基础难度")

	// S2S 状态查询走应用密钥鉴权，不计入出口 IP 的请求量
	assert.Equal(t, http.StatusUnauthorized, get("/api/v1/captchas/states/"+strings.Repeat("a", 32)).Code)
	assert.Equal(t, 1, mem.WindowCount(bucket, time.Minute))

	seed(threshold)
	assert.Equal(t, services.ClampDifficulty(base+1), levelOf(issue(path)), "超过加难阈值 1 倍加 1 级")

	seed(2 * threshold)
	assert.Equal(t, services.ClampDifficulty(base+2), levelOf(issue(path)), "超过加难阈值 2 倍加 2 级")

	// 请求量再大也只是加难到顶，不拒绝；封顶后的请求不再写入窗口
	flood := 50 * threshold
	seed(flood)
	assert.Equal(t, services.ClampDifficulty(base+2), levelOf(issue(path)), "任何请求量都不因 IP 被拒绝")
	assert.Equal(t, flood, mem.WindowCount(bucket, time.Minute))
}

func TestClientRiskStore_EscalatesDifficulty(t *testing.T) {
	rt := config.Runtime()
	store := services.NewClientRiskStore()
	policy := services.NewChallengePolicy(store)
	clientHash := fmt.Sprintf("test-client-%d", time.Now().UnixNano())

	base := rt.Captcha.DefaultDifficulty
	assert.Equal(t, base, policy.Decide(services.IssueInput{ClientHash: clientHash}).Level)

	store.Record(clientHash, false, 10)
	store.Record(clientHash, false, 10)
	assert.Equal(t, services.ClampDifficulty(base+1), policy.Decide(services.IssueInput{ClientHash: clientHash}).Level, "连续失败 2 次升一级")

	store.Record(clientHash, false, 90)
	store.Record(clientHash, false, 90)
	assert.Equal(t, services.MaxDifficulty, policy.Decide(services.IssueInput{ClientHash: clientHash}).Level, "失败 4 次且高风险分应升到顶")

	state := store.Get(clientHash)
	assert.Equal(t, 4, state.Fails)
	assert.Equal(t, 90, state.LastScore)
}

func TestRiskEngine_BlocksObviousBot(t *testing.T) {
	rt := config.Runtime()
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
	assert.GreaterOrEqual(t, result.Score, rt.Risk.BlockThreshold)
	if rt.Risk.Mode == "enforce" {
		assert.True(t, engine.ShouldBlock(result))
	}
}

func TestClientRiskStore_ConcurrentRecordsAreAtomic(t *testing.T) {
	store := services.NewClientRiskStore()
	clientHash := fmt.Sprintf("test-concurrent-%d", time.Now().UnixNano())

	const workers = 40
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			store.Record(clientHash, false, 20)
		}()
	}
	wg.Wait()

	state := store.Get(clientHash)
	assert.Equal(t, workers, state.Fails, "并发失败不能互相覆盖")
	assert.Equal(t, 20, state.LastScore)

	// 成功一次：成功数 +1，失败数衰减 1
	store.Record(clientHash, true, 0)
	state = store.Get(clientHash)
	assert.Equal(t, workers-1, state.Fails)
	assert.Equal(t, 1, state.Successes)

	// 失败计数不会衰减到负数
	fresh := fmt.Sprintf("test-fresh-%d", time.Now().UnixNano())
	store.Record(fresh, true, 0)
	assert.Equal(t, 0, store.Get(fresh).Fails)
}

func TestVerify_RiskBlockedFailsOnlyAtAnswerComparison(t *testing.T) {
	manager := services.GetServiceManager()
	service, _ := manager.GetCaptchaService("slide-text")
	verifier, _ := manager.GetVerifyService("slide-text")

	data, err := service.GenerateCaptcha(&services.ChallengeRequest{Type: "slide-text"})
	require.Nil(t, err)
	answer, meta := slideAnswer(t, data.Key)
	pow := solvePow(t, data.Key, meta)
	waitMinSolveTime()

	// 正确答案 + 风控拦截：对外表现与答案错误一致，并消耗一次尝试机会
	ok, msg, verifyErr := verifier.VerifyData(&services.VerifyInput{Key: data.Key, Answer: answer, Pow: pow, RiskBlocked: true})
	assert.False(t, ok)
	require.NotNil(t, verifyErr)
	assert.Equal(t, helper.ErrCodeVerifyFailed, verifyErr.Code)
	assert.Equal(t, "验证失败", msg)

	record, getErr := services.NewBaseService().GetCaptchaData(data.Key)
	require.Nil(t, getErr)
	assert.Equal(t, float64(1), record["attempts"], "拦截应消耗一次尝试机会")

	// 同一答案在未拦截时能通过，证明上一步失败的唯一原因是风控
	ok, _, verifyErr = verifier.VerifyData(&services.VerifyInput{Key: data.Key, Answer: answer, Pow: pow})
	assert.True(t, ok)
	assert.Nil(t, verifyErr)
}

// TestVerify_RiskBlockedMatchesNormalPathErrors 被拦截的请求在答案比对之前的每个阶段，
// 返回值都必须与未拦截时完全一致，否则攻击者可以故意触发前置错误来探测自己是否被拦截
func TestVerify_RiskBlockedMatchesNormalPathErrors(t *testing.T) {
	rt := config.Runtime()
	manager := services.GetServiceManager()
	service, _ := manager.GetCaptchaService("slide-text")
	verifier, _ := manager.GetVerifyService("slide-text")

	type outcome struct {
		ok   bool
		msg  string
		code int
	}
	run := func(in *services.VerifyInput) outcome {
		ok, msg, err := verifier.VerifyData(in)
		o := outcome{ok: ok, msg: msg}
		if err != nil {
			o.code = err.Code
		}
		return o
	}
	fresh := func(t *testing.T, clientHash string) (string, string, *risk.PowSolution) {
		data, err := service.GenerateCaptcha(&services.ChallengeRequest{Type: "slide-text", ClientHash: clientHash})
		require.Nil(t, err)
		answer, meta := slideAnswer(t, data.Key)
		return data.Key, answer, solvePow(t, data.Key, meta)
	}

	cases := []struct {
		name  string
		skip  bool
		build func(t *testing.T) *services.VerifyInput
		want  int
	}{
		{
			name: "未知 key",
			build: func(t *testing.T) *services.VerifyInput {
				return &services.VerifyInput{Key: strings.Repeat("0", 32), Answer: "1,1"}
			},
			want: helper.ErrCodeExpired,
		},
		{
			name: "提交过快",
			skip: rt.Captcha.MinSolveTime() <= 0,
			build: func(t *testing.T) *services.VerifyInput {
				key, answer, pow := fresh(t, "")
				return &services.VerifyInput{Key: key, Answer: answer, Pow: pow}
			},
			want: helper.ErrCodeTooFast,
		},
		{
			name: "客户端不匹配",
			skip: !rt.Captcha.BindClient,
			build: func(t *testing.T) *services.VerifyInput {
				key, answer, pow := fresh(t, "client-a")
				waitMinSolveTime()
				return &services.VerifyInput{Key: key, Answer: answer, Pow: pow, ClientHash: "client-b"}
			},
			want: helper.ErrCodeClientMismatch,
		},
		{
			name: "PoW 无效",
			build: func(t *testing.T) *services.VerifyInput {
				key, answer, pow := fresh(t, "")
				require.NotNil(t, pow, "默认配置应启用 PoW")
				waitMinSolveTime()
				return &services.VerifyInput{Key: key, Answer: answer, Pow: &risk.PowSolution{Nonce: pow.Nonce, Solution: "definitely-wrong"}}
			},
			want: helper.ErrCodePowInvalid,
		},
		{
			name: "答案格式错误",
			build: func(t *testing.T) *services.VerifyInput {
				key, _, pow := fresh(t, "")
				waitMinSolveTime()
				return &services.VerifyInput{Key: key, Answer: "not-a-point", Pow: pow}
			},
			want: helper.ErrCodeInvalidParam,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip {
				t.Skip("当前配置未启用该检查")
			}
			normal := tc.build(t)
			blocked := tc.build(t)
			blocked.RiskBlocked = true

			got := run(normal)
			gotBlocked := run(blocked)
			assert.Equal(t, tc.want, got.code, "未拦截路径应命中预期的前置错误")
			assert.Equal(t, got, gotBlocked, "拦截与否的返回值必须一致")
		})
	}
}

func TestVerifyEndpoint_RiskBlockLooksLikeNormalFailure(t *testing.T) {
	rt := config.Runtime()
	if !rt.Risk.Enabled || rt.Risk.Mode != "enforce" {
		t.Skip("RISK_MODE != enforce")
	}

	manager := services.GetServiceManager()
	service, _ := manager.GetCaptchaService("slide-text")
	data, err := service.GenerateCaptcha(&services.ChallengeRequest{Type: "slide-text", AppID: services.ConsoleAppID})
	require.Nil(t, err)
	answer, meta := slideAnswer(t, data.Key)
	pow := solvePow(t, data.Key, meta)
	env, trace := botTelemetry()

	post := func(t *testing.T) (int, string) {
		body, _ := json.Marshal(map[string]interface{}{"point": answer, "pow": pow, "env": env, "trace": trace})
		req := asConsoleApp(httptest.NewRequest(http.MethodPost, config.APIPrefix+"/captchas/slide-text/"+data.Key+"/attempts", bytes.NewReader(body)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router().ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp struct {
			Code  int    `json:"code"`
			Error string `json:"error"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		return resp.Code, resp.Error
	}
	attempts := func(t *testing.T) float64 {
		record, getErr := services.NewBaseService().GetCaptchaData(data.Key)
		require.Nil(t, getErr)
		v, _ := record["attempts"].(float64)
		return v
	}

	// 被拦截的请求若提交过快，仍应得到与正常路径一致的"提交过快"，而不是提前暴露 1003
	expectAttempts := float64(0)
	if rt.Captcha.MinSolveTime() > 0 {
		code, _ := post(t)
		assert.Equal(t, helper.ErrCodeTooFast, code, "拦截路径上的前置检查响应必须与正常路径一致")
		expectAttempts++
		assert.Equal(t, expectAttempts, attempts(t))
	}

	// 正确答案 + 机器特征：对外与答案错误无法区分，并消耗一次尝试机会
	waitMinSolveTime()
	code, msg := post(t)
	assert.Equal(t, helper.ErrCodeVerifyFailed, code, "风控拦截对外应与答案错误无法区分")
	assert.Equal(t, helper.ErrVerifyFailed.Message, msg)
	expectAttempts++
	assert.Equal(t, expectAttempts, attempts(t), "拦截应消耗一次尝试机会")
}

func TestVerifyEndpoint_RejectsOversizedBody(t *testing.T) {
	cfg := config.LoadConfig()
	limit := cfg.Security.VerifyMaxBodySize
	require.Greater(t, limit, int64(0))

	// 语法合法但超过上限的 JSON；ContentLength 置为 -1 模拟 chunked 传输，只能靠 MaxBytesReader 截断
	body := `{"point":"1,1"` + strings.Repeat(" ", int(limit)+1024) + `}`
	req := asConsoleApp(httptest.NewRequest(http.MethodPost, config.APIPrefix+"/captchas/slide-text/"+strings.Repeat("a", 32)+"/attempts", strings.NewReader(body)))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestVerifyRequest_RejectsOversizedTelemetryFields(t *testing.T) {
	base := models.VerifyRequest{Type: "slide-text", Key: "k", Point: "1,1"}

	req := base
	req.Env = &risk.Env{Languages: []string{strings.Repeat("x", 129)}}
	assert.NotNil(t, req.Validate(), "语言列表单项超长应被拒绝")

	req = base
	req.Env = &risk.Env{Automation: []string{strings.Repeat("x", 129)}}
	assert.NotNil(t, req.Validate(), "自动化标记单项超长应被拒绝")

	req = base
	req.Trace = &risk.Trace{PointerType: strings.Repeat("p", 17)}
	assert.NotNil(t, req.Validate(), "指针类型超长应被拒绝")

	req = base
	req.Env = &risk.Env{Languages: []string{"zh-CN", "en"}}
	req.Trace = &risk.Trace{PointerType: "mouse"}
	assert.Nil(t, req.Validate())
}

func TestRouter_CDNHeadersIgnoredByDefault(t *testing.T) {
	cfg := config.LoadConfig()
	if len(cfg.Security.RemoteIPHeaders) != 2 {
		t.Skip("REMOTE_IP_HEADERS 已自定义")
	}
	assert.Equal(t, []string{"X-Forwarded-For", "X-Real-IP"}, cfg.Security.RemoteIPHeaders)

	clientIP := func(remoteAddr string, headers map[string]string) string {
		req := httptest.NewRequest(http.MethodGet, "/__client_ip", nil)
		req.RemoteAddr = remoteAddr
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		router().ServeHTTP(rec, req)
		return rec.Body.String()
	}

	// 直连来源命中可信代理网段：未显式开启的 CDN 头不应被采信
	assert.Equal(t, "10.0.0.8", clientIP("10.0.0.8:4321", map[string]string{
		"CF-Connecting-IP": "198.51.100.7",
		"True-Client-IP":   "198.51.100.8",
	}))
	// 常规代理头仍然生效
	assert.Equal(t, "203.0.113.9", clientIP("10.0.0.8:4321", map[string]string{"X-Forwarded-For": "203.0.113.9"}))
	// 直连来源不在可信网段：任何代理头都不生效
	assert.Equal(t, "203.0.113.50", clientIP("203.0.113.50:4321", map[string]string{"X-Forwarded-For": "198.51.100.1"}))
}
