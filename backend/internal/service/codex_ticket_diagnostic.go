package service

import (
	"context"
)

type codexTicketDiagnosticTargetKey struct{}

func WithCodexTicketDiagnostic(ctx context.Context, accountID int64) context.Context {
	return context.WithValue(ctx, codexTicketDiagnosticTargetKey{}, accountID)
}

func isCodexTicketDiagnostic(ctx context.Context) bool {
	accountID, _ := ctx.Value(codexTicketDiagnosticTargetKey{}).(int64)
	return accountID > 0
}

// CodexDiagnosticVerdict 是一次降智判定。
// Status: normal(正常) / degraded(疑似降智) / uncertain(不确定) / failed(请求失败)。
type CodexDiagnosticVerdict struct {
	Status         string
	Reason         string
	PredictedModel string
	Probability    float64
	ParsedCount    int
}

// EvaluateCodexDiagnostic 把探测输出与请求模型比对。
//
// 指纹库是闭集：请求的模型若不在库中（当前只有 gpt-6.1-sol），分类器永远
// 不会返回它的 ID。此时用同系列参考类兜底判定——命中参考类报 uncertain
// （库缺类，无法确认具体版本），命中其它任何 GPT 类才是 degraded。这样
// 健康的 6.1-sol 不会被误判成降智，而降级到 5.6/其它系列仍会被抓到。
func EvaluateCodexDiagnostic(model, output string, challenge ModelTraceChallenge) CodexDiagnosticVerdict {
	prediction, _, err := ModelTracePredictCommitted(output, challenge.ExpectedCount)
	verdict := CodexDiagnosticVerdict{Status: "failed", Reason: "insufficient_numbers", ParsedCount: prediction.ParsedCount}
	if err != nil {
		return verdict
	}
	verdict.PredictedModel = prediction.Model
	verdict.Probability = prediction.Probability
	if prediction.Model == model {
		return CodexDiagnosticVerdict{Status: "normal", PredictedModel: prediction.Model, Probability: prediction.Probability, ParsedCount: prediction.ParsedCount}
	}
	if reference, ok := modelTraceDiagnosticReferenceClass[model]; ok {
		if prediction.Model == reference {
			verdict.Status = "uncertain"
			verdict.Reason = "reference_class_unavailable"
			return verdict
		}
	}
	verdict.Status = "degraded"
	verdict.Reason = "fingerprint_mismatch"
	return verdict
}
