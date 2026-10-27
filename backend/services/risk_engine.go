package services

import (
	"encoding/json"
	"log"
	"simple-captcha/config"
	"simple-captcha/risk"
	"time"
)

// EvaluationContext 一次校验请求的风控评估上下文
type EvaluationContext struct {
	Type       string
	ClientHash string
	IP         string
	Key        string
	Trace      *risk.Trace
	Env        *risk.Env
	Request    *risk.RequestSignals
	Pow        *risk.PowSolution
}

// RiskEngine 风控评分引擎：把 risk 包的纯计算与 Redis 状态存储、日志串起来
type RiskEngine struct {
	cfg   *config.Config
	store *ClientRiskStore
}

// NewRiskEngine 创建风控评分引擎
func NewRiskEngine(store *ClientRiskStore) *RiskEngine {
	return &RiskEngine{cfg: config.LoadConfig(), store: store}
}

// Enabled 是否启用评分
func (e *RiskEngine) Enabled() bool {
	return e != nil && e.cfg.Risk.Enabled
}

// Evaluate 对本次提交打分，未启用时返回 nil
func (e *RiskEngine) Evaluate(ctx *EvaluationContext) *risk.Result {
	if !e.Enabled() || ctx == nil {
		return nil
	}

	result := risk.Evaluate(risk.Input{
		Type:        ctx.Type,
		Trace:       ctx.Trace,
		Env:         ctx.Env,
		Request:     ctx.Request,
		PowExpected: e.cfg.Captcha.PowEnabled,
		PowProvided: ctx.Pow != nil && ctx.Pow.Solution != "",
		EnvScoreCap: e.cfg.Risk.EnvScoreCap,
	})
	return &result
}

// ShouldBlock 是否应直接拒绝本次校验：enforce 模式、总分达到拦截阈值，
// 且行为分不低于 RISK_BLOCK_MIN_BEHAVIOR 或命中硬性自动化标记。
// 环境信号可被伪造也可能因 VDI、隐私浏览器等正常原因命中，单靠环境信号不拦截
func (e *RiskEngine) ShouldBlock(result *risk.Result) bool {
	if !e.Enabled() || result == nil {
		return false
	}
	if e.cfg.Risk.Mode != "enforce" || result.Score < e.cfg.Risk.BlockThreshold {
		return false
	}
	return result.HardMarker || result.BehaviorScore >= e.cfg.Risk.BlockMinBehavior
}

// IssueContext 一次出题的遥测上下文
type IssueContext struct {
	// Type 实际下发的题型（auto 已解析）
	Type       string
	ClientHash string
	IP         string
	Key        string
	Env        *risk.Env
	Request    *risk.RequestSignals
	// ViaPost 是否通过携带环境信号的 POST 接口出题
	ViaPost bool
}

// recordFingerprints 可信代理注入的 TLS / HTTP2 指纹原值写入遥测，供运维整理指纹列表
func recordFingerprints(entry map[string]interface{}, sig *risk.RequestSignals) {
	if sig == nil {
		return
	}
	if sig.TLSFingerprint != "" {
		entry["tls"] = sig.TLSFingerprint
	}
	if sig.HTTP2Fingerprint != "" {
		entry["h2"] = sig.HTTP2Fingerprint
	}
}

// RecordIssue 记录一次出题评估：难度来源与环境得分写入遥测列表（kind=issue），命中加难原因时另输出一行日志
func (e *RiskEngine) RecordIssue(ctx *IssueContext, decision IssueDecision) {
	if !e.Enabled() || ctx == nil {
		return
	}

	// 遥测只保存脱敏后的 IP（前两段）与 UA 摘要，原始值不落 Redis
	entry := map[string]interface{}{
		"kind":       "issue",
		"time":       time.Now().Format(time.RFC3339),
		"type":       ctx.Type,
		"client":     ctx.ClientHash,
		"ip":         MaskIP(ctx.IP),
		"key":        ctx.Key,
		"level":      decision.Level,
		"base":       decision.Base,
		"boost":      decision.Boost,
		"envScore":   decision.EnvScore,
		"consistent": decision.Consistent,
		"reasons":    decision.Reasons,
		"viaPost":    ctx.ViaPost,
	}
	if ctx.Env != nil {
		entry["fp"] = ctx.Env.Fingerprint
		entry["ua"] = SummarizeUA(ctx.Env.UA)
		entry["webdriver"] = ctx.Env.Webdriver
		entry["automation"] = ctx.Env.Automation
		entry["anomalies"] = ctx.Env.Anomalies
	}
	recordFingerprints(entry, ctx.Request)

	if len(decision.Reasons) > 0 {
		if payload, err := json.Marshal(entry); err == nil {
			log.Printf("出题评估: %s", payload)
		}
	}

	if e.store != nil {
		e.store.LogTelemetry(entry)
	}
}

// Record 记录本次校验结果：更新客户端状态、输出结构化日志、追加遥测记录
func (e *RiskEngine) Record(ctx *EvaluationContext, result *risk.Result, success bool) {
	if e == nil || ctx == nil {
		return
	}

	score := 0
	if result != nil {
		score = result.Score
	}

	if e.store != nil {
		e.store.Record(ctx.ClientHash, success, score)
	}

	if !e.Enabled() {
		return
	}

	// 遥测只保存脱敏后的 IP（前两段）与 UA 摘要，原始值不落 Redis
	entry := map[string]interface{}{
		"kind":    "verify",
		"time":    time.Now().Format(time.RFC3339),
		"type":    ctx.Type,
		"client":  ctx.ClientHash,
		"ip":      MaskIP(ctx.IP),
		"key":     ctx.Key,
		"success": success,
		"score":   score,
		"mode":    e.cfg.Risk.Mode,
	}
	if result != nil {
		entry["reasons"] = result.Reasons
		entry["features"] = result.Features
		entry["envScore"] = result.EnvScore
		entry["behaviorScore"] = result.BehaviorScore
		entry["hardMarker"] = result.HardMarker
		entry["consistent"] = result.Consistent
		entry["requestReasons"] = result.RequestReasons
	}
	if ctx.Trace != nil {
		entry["pointerType"] = ctx.Trace.PointerType
		entry["untrusted"] = ctx.Trace.Untrusted
	}
	if ctx.Env != nil {
		entry["ua"] = SummarizeUA(ctx.Env.UA)
		entry["webdriver"] = ctx.Env.Webdriver
		entry["automation"] = ctx.Env.Automation
		entry["anomalies"] = ctx.Env.Anomalies
		entry["fp"] = ctx.Env.Fingerprint
	}
	recordFingerprints(entry, ctx.Request)

	if payload, err := json.Marshal(entry); err == nil {
		log.Printf("风控评估: %s", payload)
	}

	if e.store != nil {
		e.store.LogTelemetry(entry)
	}
}
