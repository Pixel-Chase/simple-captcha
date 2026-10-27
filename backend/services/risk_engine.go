package services

import (
	"encoding/json"
	"log"
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
	// AppID 提交请求的应用，写入风控观察
	AppID string
}

// RiskEngine 风控评分引擎：把 risk 包的纯计算与客户端状态存储、风控观察记录串起来
type RiskEngine struct {
	settings settingsSource
	store    *ClientRiskStore
}

// NewRiskEngine 创建风控评分引擎；策略值每次使用时从 config.Runtime() 读取
func NewRiskEngine(store *ClientRiskStore) *RiskEngine {
	return &RiskEngine{store: store}
}

// Enabled 是否启用评分
func (e *RiskEngine) Enabled() bool {
	return e != nil && e.settings.get().Risk.Enabled
}

// Evaluate 对本次提交打分，未启用时返回 nil
func (e *RiskEngine) Evaluate(ctx *EvaluationContext) *risk.Result {
	if e == nil || ctx == nil {
		return nil
	}
	rt := e.settings.get()
	if !rt.Risk.Enabled {
		return nil
	}

	result := risk.Evaluate(risk.Input{
		Type:        ctx.Type,
		Trace:       ctx.Trace,
		Env:         ctx.Env,
		Request:     ctx.Request,
		PowExpected: rt.Pow.Enabled,
		PowProvided: ctx.Pow != nil && ctx.Pow.Solution != "",
		EnvScoreCap: rt.Risk.EnvScoreCap,
	})
	return &result
}

// ShouldBlock 是否应直接拒绝本次校验：enforce 模式、总分达到拦截阈值，
// 且行为分不低于策略 risk.blockMinBehavior 或命中硬性自动化标记。
// 环境信号可被伪造也可能因 VDI、隐私浏览器等正常原因命中，单靠环境信号不拦截
func (e *RiskEngine) ShouldBlock(result *risk.Result) bool {
	if e == nil || result == nil {
		return false
	}
	rt := e.settings.get()
	if !rt.Risk.Enabled || rt.Risk.Mode != "enforce" || result.Score < rt.Risk.BlockThreshold {
		return false
	}
	return result.HardMarker || result.BehaviorScore >= rt.Risk.BlockMinBehavior
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
	// AppID 出题请求的应用，写入风控观察
	AppID string
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

	// 遥测只保存脱敏后的 IP（前两段）与 UA 摘要，原始值不落库
	entry := map[string]interface{}{
		"kind":       "issue",
		"time":       time.Now().Format(time.RFC3339),
		"app":        ctx.AppID,
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

	// 遥测只保存脱敏后的 IP（前两段）与 UA 摘要，原始值不落库
	entry := map[string]interface{}{
		"kind":    "verify",
		"time":    time.Now().Format(time.RFC3339),
		"app":     ctx.AppID,
		"type":    ctx.Type,
		"client":  ctx.ClientHash,
		"ip":      MaskIP(ctx.IP),
		"key":     ctx.Key,
		"success": success,
		"score":   score,
		"mode":    e.settings.get().Risk.Mode,
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
