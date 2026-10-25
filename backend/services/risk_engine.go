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
		PowExpected: e.cfg.Captcha.PowEnabled,
		PowProvided: ctx.Pow != nil && ctx.Pow.Solution != "",
	})
	return &result
}

// ShouldBlock 是否应直接拒绝本次校验：仅在 enforce 模式且分数达到拦截阈值时为真
func (e *RiskEngine) ShouldBlock(result *risk.Result) bool {
	if !e.Enabled() || result == nil {
		return false
	}
	return e.cfg.Risk.Mode == "enforce" && result.Score >= e.cfg.Risk.BlockThreshold
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

	entry := map[string]interface{}{
		"time":    time.Now().Format(time.RFC3339),
		"type":    ctx.Type,
		"client":  ctx.ClientHash,
		"ip":      ctx.IP,
		"key":     ctx.Key,
		"success": success,
		"score":   score,
		"mode":    e.cfg.Risk.Mode,
	}
	if result != nil {
		entry["reasons"] = result.Reasons
		entry["features"] = result.Features
	}
	if ctx.Trace != nil {
		entry["pointerType"] = ctx.Trace.PointerType
		entry["untrusted"] = ctx.Trace.Untrusted
	}
	if ctx.Env != nil {
		entry["ua"] = ctx.Env.UA
		entry["webdriver"] = ctx.Env.Webdriver
		entry["automation"] = ctx.Env.Automation
		entry["fp"] = ctx.Env.Fingerprint
	}

	if payload, err := json.Marshal(entry); err == nil {
		log.Printf("风控评估: %s", payload)
	}

	if e.store != nil {
		e.store.LogTelemetry(entry)
	}
}
