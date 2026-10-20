package admin

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/MACOS-DO/sub4api/internal/pkg/response"
	"github.com/MACOS-DO/sub4api/internal/service"

	"github.com/gin-gonic/gin"
)

type codexDiagnosticRunner interface {
	RunGatewayDiagnostic(ctx context.Context, accountID int64, model string, challenge service.ModelTraceChallenge, timeout time.Duration) (output string, httpStatus int, gatewayErrCode string, reason string)
}

// SetCodexDiagnosticDependencies 接线降智检测所需的依赖：网关诊断 runner
// （httptest 回放）、探测记录仓库（history）与设置服务（指纹库刷新）。
func (h *AccountHandler) SetCodexDiagnosticDependencies(runner codexDiagnosticRunner, attempts service.CodexTicketAttemptRepository, settings *service.SettingService) {
	h.codexDiagnosticRunner = runner
	h.codexTicketAttempts = attempts
	h.codexDiagnosticSettings = settings
}

type codexDiagnosticItem struct {
	Model            string  `json:"model"`
	Status           string  `json:"status"`
	Reason           string  `json:"reason,omitempty"`
	PredictedModel   string  `json:"predicted_model,omitempty"`
	Probability      float64 `json:"probability,omitempty"`
	ParsedCount      int     `json:"parsed_number_count,omitempty"`
	HTTPStatus       int     `json:"http_status,omitempty"`
	GatewayErrorCode string  `json:"gateway_error_code,omitempty"`
}

func codexDiagnosticAccountID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return 0, false
	}
	return id, true
}

// DiagnoseCodexModels 手动降智检测：逐模型经网关指纹通路探测 + 三态判定。
// 输入仅模型列表——探测走进程级内部诊断令牌，不需要管理员选择 API Key，
// 也不消耗任何用户额度。
func (h *AccountHandler) DiagnoseCodexModels(c *gin.Context) {
	h.diagnoseCodexModels(c, service.NewModelTraceChallenge)
}

func (h *AccountHandler) diagnoseCodexModels(c *gin.Context, generateChallenge func() (service.ModelTraceChallenge, error)) {
	accountID, ok := codexDiagnosticAccountID(c)
	if !ok {
		return
	}
	if h.codexDiagnosticRunner == nil {
		response.ErrorWithDetails(c, http.StatusUnprocessableEntity, "Codex diagnostic unavailable", "CODEX_DIAGNOSTIC_UNAVAILABLE", nil)
		return
	}
	var input struct {
		Models []string `json:"models"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || len(input.Models) == 0 || len(input.Models) > 32 {
		response.BadRequest(c, "Select up to 32 models")
		return
	}
	allowed, err := service.ModelTraceDiagnosticModels()
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	modelSet := make(map[string]bool, len(allowed))
	for _, model := range allowed {
		modelSet[model] = true
	}
	seen := make(map[string]bool, len(input.Models))
	for _, model := range input.Models {
		if !modelSet[model] || seen[model] {
			response.BadRequest(c, "Invalid or duplicate model")
			return
		}
		seen[model] = true
	}
	results := make([]codexDiagnosticItem, 0, len(input.Models))
	for _, model := range input.Models {
		if c.Request.Context().Err() != nil {
			break
		}
		item, attempt := h.diagnoseCodexModel(c, accountID, model, generateChallenge)
		if attempt != nil && h.codexTicketAttempts != nil {
			// 落库不阻断诊断结果：用脱离请求生命周期的上下文，避免客户端断开导致丢失记录
			ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 5*time.Second)
			if err := h.codexTicketAttempts.Insert(ctx, attempt); err != nil {
				slog.Warn("codex_ticket_attempt_insert_failed", "account_id", accountID, "model", model, "error", err)
			}
			cancel()
		}
		results = append(results, item)
	}
	response.Success(c, gin.H{"items": results, "canceled": c.Request.Context().Err() != nil})
}

// diagnoseCodexModel 探测单个模型。attempt 仅在探测真正打进网关后返回
// （preflight 失败如模板不可用/无分组/身份解析失败不落库）。
func (h *AccountHandler) diagnoseCodexModel(c *gin.Context, accountID int64, model string, generateChallenge func() (service.ModelTraceChallenge, error)) (codexDiagnosticItem, *service.CodexTicketAttempt) {
	item := codexDiagnosticItem{Model: model, Status: "failed"}
	challenge, challengeErr := generateChallenge()
	if challengeErr != nil {
		item.Reason = "challenge_generation_failed"
		return item, nil
	}
	start := time.Now()
	output, httpStatus, gatewayErrCode, reason := h.codexDiagnosticRunner.RunGatewayDiagnostic(c.Request.Context(), accountID, model, challenge, 120*time.Second)
	item.HTTPStatus = httpStatus
	item.GatewayErrorCode = gatewayErrCode
	if httpStatus <= 0 {
		// 预检失败：请求未真正打进网关，不落库。
		item.Reason = reason
		return item, nil
	}
	expectedCount := challenge.ExpectedCount
	attempt := &service.CodexTicketAttempt{
		AccountID:              accountID,
		Model:                  model,
		OccurredAt:             start,
		Outcome:                "error",
		Trigger:                "diagnostic",
		HTTPStatus:             &httpStatus,
		DurationMS:             int(time.Since(start).Milliseconds()),
		ReasonCode:             "gateway_request_failed",
		VerificationMethod:     "modeltrace",
		FingerprintCommit:      service.ModelTraceBankCommit(),
		ChallengeExpectedCount: &expectedCount,
	}
	if reason != "" {
		item.Reason = reason
		if reason == "response_incomplete" {
			attempt.Outcome = "miss"
			attempt.ReasonCode = reason
		}
		return item, attempt
	}
	verdict := service.EvaluateCodexDiagnostic(model, output, challenge)
	item.Status = verdict.Status
	item.Reason = verdict.Reason
	item.PredictedModel = verdict.PredictedModel
	item.Probability = verdict.Probability
	item.ParsedCount = verdict.ParsedCount
	attempt.Outcome = "success"
	attempt.ReasonCode = verdict.Reason
	attempt.ParsedNumberCount = &verdict.ParsedCount
	if verdict.PredictedModel != "" {
		attempt.FingerprintPredictedModel = verdict.PredictedModel
		attempt.FingerprintProbability = &verdict.Probability
	}
	switch verdict.Status {
	case "normal":
		matched := true
		attempt.FingerprintMatched = &matched
		attempt.ReasonCode = ""
	case "degraded":
		matched := false
		attempt.FingerprintMatched = &matched
	case "uncertain":
		// 库缺参考类：无法确认是否匹配，FingerprintMatched 保持 nil。
		attempt.ReasonCode = "reference_class_unavailable"
	}
	return item, attempt
}
