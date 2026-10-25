package tests

import (
	"bytes"
	"context"
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
	"github.com/go-redis/redis/v8"
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

func TestRateLimiter_RedisSlidingWindow(t *testing.T) {
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
	cfg := config.LoadConfig()
	threshold := cfg.Captcha.IPEscalatePerMin
	if threshold <= 0 {
		t.Skip("CAPTCHA_IP_ESCALATE_PER_MINUTE 为 0，按 IP 加难已关闭")
	}

	const egressIP = "198.51.100.77"
	bucket := services.RateLimitKeyPrefix + egressIP
	rdb := cache.GetRedisClient()
	ctx := context.Background()
	require.NoError(t, rdb.Del(ctx, bucket).Err())
	t.Cleanup(func() { rdb.Del(ctx, bucket) })

	// 直接往窗口里预填请求记录，模拟同一出口下很多人在同一分钟里出题
	seed := func(n int) {
		require.NoError(t, rdb.Del(ctx, bucket).Err())
		now := time.Now().UnixMilli()
		members := make([]*redis.Z, 0, n)
		for i := 0; i < n; i++ {
			members = append(members, &redis.Z{Score: float64(now), Member: fmt.Sprintf("seed-%d", i)})
		}
		if n > 0 {
			require.NoError(t, rdb.ZAdd(ctx, bucket, members...).Err())
		}
	}
	// 用一个全新的 UA，保证客户端历史状态为空，难度只受出口 IP 请求量影响
	userAgent := fmt.Sprintf("shared-egress-test/%d", time.Now().UnixNano())
	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = egressIP + ":4321"
		req.Header.Set("User-Agent", userAgent)
		w := httptest.NewRecorder()
		router().ServeHTTP(w, req)
		return w
	}
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
	base := services.ClampDifficulty(cfg.Captcha.DefaultDifficulty)

	seed(0)
	assert.Equal(t, base, levelOf(get(path)), "低于加难阈值时保持基础难度")

	// S2S 状态查询走应用密钥鉴权，不计入出口 IP 的请求量
	assert.Equal(t, http.StatusUnauthorized, get("/api/v1/captchas/states/"+strings.Repeat("a", 32)).Code)
	assert.EqualValues(t, 1, rdb.ZCard(ctx, bucket).Val())

	seed(threshold)
	assert.Equal(t, services.ClampDifficulty(base+1), levelOf(get(path)), "超过加难阈值 1 倍加 1 级")

	seed(2 * threshold)
	assert.Equal(t, services.ClampDifficulty(base+2), levelOf(get(path)), "超过加难阈值 2 倍加 2 级")

	// 请求量再大也只是加难到顶，不拒绝；封顶后的请求不再写入窗口
	flood := 50 * threshold
	seed(flood)
	assert.Equal(t, services.ClampDifficulty(base+2), levelOf(get(path)), "任何请求量都不因 IP 被拒绝")
	assert.EqualValues(t, flood, rdb.ZCard(ctx, bucket).Val())
}

func TestClientRiskStore_EscalatesDifficulty(t *testing.T) {
	store := services.NewClientRiskStore()
	policy := services.NewChallengePolicy(store)
	clientHash := fmt.Sprintf("test-client-%d", time.Now().UnixNano())

	base := config.LoadConfig().Captcha.DefaultDifficulty
	assert.Equal(t, base, policy.DifficultyFor(clientHash, 0))

	store.Record(clientHash, false, 10)
	store.Record(clientHash, false, 10)
	assert.Equal(t, services.ClampDifficulty(base+1), policy.DifficultyFor(clientHash, 0), "连续失败 2 次升一级")

	store.Record(clientHash, false, 90)
	store.Record(clientHash, false, 90)
	assert.Equal(t, services.MaxDifficulty, policy.DifficultyFor(clientHash, 0), "失败 4 次且高风险分应升到顶")

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
	cfg := config.LoadConfig()
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
			skip: cfg.Captcha.MinSolveTime <= 0,
			build: func(t *testing.T) *services.VerifyInput {
				key, answer, pow := fresh(t, "")
				return &services.VerifyInput{Key: key, Answer: answer, Pow: pow}
			},
			want: helper.ErrCodeTooFast,
		},
		{
			name: "客户端不匹配",
			skip: !cfg.Captcha.BindClient,
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
	cfg := config.LoadConfig()
	if !cfg.Risk.Enabled || cfg.Risk.Mode != "enforce" {
		t.Skip("RISK_MODE != enforce")
	}

	manager := services.GetServiceManager()
	service, _ := manager.GetCaptchaService("slide-text")
	data, err := service.GenerateCaptcha(&services.ChallengeRequest{Type: "slide-text"})
	require.Nil(t, err)
	answer, meta := slideAnswer(t, data.Key)
	pow := solvePow(t, data.Key, meta)
	env, trace := botTelemetry()

	post := func(t *testing.T) (int, string) {
		body, _ := json.Marshal(map[string]interface{}{"point": answer, "pow": pow, "env": env, "trace": trace})
		req := httptest.NewRequest(http.MethodPost, cfg.Server.APIPrefix+"/captchas/slide-text/"+data.Key+"/attempts", bytes.NewReader(body))
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
	if cfg.Captcha.MinSolveTime > 0 {
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
	req := httptest.NewRequest(http.MethodPost, cfg.Server.APIPrefix+"/captchas/slide-text/"+strings.Repeat("a", 32)+"/attempts", strings.NewReader(body))
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
