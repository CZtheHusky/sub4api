package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// CodexEffortProbeResult uses the minimal-effort validation response as its verdict.
// WS metadata is observational and never changes that verdict.
type CodexEffortProbeResult struct {
	Model                      string
	Status                     string // normal | degraded | uncertain | failed
	Reason                     string
	HTTPStatus                 int
	DurationMS                 int
	SupportedEfforts           []string
	SafetyBufferingEnabled     *bool
	SafetyBufferingFasterModel string
	TurnStateLen               *int
	Transport                  string
}

var codexEffortProbeSupportedValues = regexp.MustCompile(`Supported values are: ([^.]+)`)

// classifyCodexEffortProbeResponse 依据上游对 minimal effort 探针的响应判读降智
// 状态：400 且报错列出含 "none" 的档位 → degraded；400 列表不含 "none" →
// normal；400 但解析不到列表、或请求被正常受理（档位税变更）→ uncertain；
// 鉴权与其他状态 → failed。
func classifyCodexEffortProbeResponse(httpStatus int, body []byte) (status, reason string, efforts []string) {
	switch {
	case httpStatus == http.StatusBadRequest:
		match := codexEffortProbeSupportedValues.FindSubmatch(body)
		if match == nil {
			return "uncertain", "effort_list_missing", nil
		}
		efforts = parseQuotedValues(match[1])
		if len(efforts) == 0 {
			return "uncertain", "effort_list_missing", nil
		}
		for _, effort := range efforts {
			if effort == "none" {
				return "degraded", "effort_list_contains_none", efforts
			}
		}
		return "normal", "", efforts
	case httpStatus >= 200 && httpStatus < 300:
		return "uncertain", "minimal_accepted", nil
	case httpStatus == http.StatusUnauthorized || httpStatus == http.StatusForbidden:
		return "failed", "unauthorized", nil
	default:
		return "failed", "upstream_error", nil
	}
}

func parseQuotedValues(raw []byte) []string {
	values := make([]string, 0, 8)
	for _, field := range strings.Split(string(raw), ",") {
		field = strings.TrimSpace(field)
		field = strings.TrimSuffix(field, ".")
		field = strings.TrimSpace(strings.TrimPrefix(field, "and "))
		field = strings.Trim(field, "'")
		if field != "" {
			values = append(values, field)
		}
	}
	return values
}

// CodexEffortProbeModels 返回 400 effort 探针支持的模型：降智诊断白名单中
// astra 系列的子集。探针判据（minimal 档 400 的档位列表含 none 即降智）目前
// 只在 gpt-6-astra 上验证过，其余系列不开放，避免把未验证的判读当成结论。
func CodexEffortProbeModels() ([]string, error) {
	models, err := ModelTraceDiagnosticModels()
	if err != nil {
		return nil, err
	}
	probe := make([]string, 0, len(models))
	for _, model := range models {
		if strings.Contains(model, "astra") {
			probe = append(probe, model)
		}
	}
	return probe, nil
}

const codexEffortProbeSession = "sub4api:codex-effort-probe"
const codexEffortProbeTimeout = 15 * time.Second

// RunCodexEffortProbe follows the account transport and plugin routing without
// entering business connection pools, effort rewriting or account failover.
func (s *OpenAIGatewayService) RunCodexEffortProbe(ctx context.Context, accountID int64, model string) (result CodexEffortProbeResult) {
	result = CodexEffortProbeResult{Model: model, Status: "failed", Reason: "identity_resolution_failed"}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		return result
	}
	result.Reason = "no_access_token"
	if account.IsOpenAIAgentIdentity() {
		return result
	}
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil || token == "" {
		return result
	}
	payload := map[string]any{
		"model": model, "store": false, "stream": true,
		"instructions": "You are a helpful assistant.",
		"input":        []any{map[string]any{"role": "user", "content": []any{map[string]string{"type": "input_text", "text": "hi"}}}},
		"reasoning":    map[string]any{"effort": "minimal"},
		// Bound generation if the upstream starts accepting minimal in the future.
		"max_output_tokens": 16,
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	start := time.Now()
	defer func() { result.DurationMS = int(time.Since(start).Milliseconds()) }()
	decision := s.getOpenAIWSProtocolResolver().Resolve(account)
	if account.IsOpenAIOAuthLike() && decision.Transport == OpenAIUpstreamTransportResponsesWebsocketV2 && !s.pluginManager.ShouldRouteOpenAIOAuth(account) {
		var fallback bool
		result, fallback = s.runCodexEffortProbeWS(ctx, account, token, proxyURL, payload, result)
		if !fallback || ctx.Err() != nil {
			return result
		}
		slog.Info("codex_effort_probe_http_fallback", "account_id", accountID, "model", model, "reason", result.Reason)
		// Observations belong only to the final transport, never to both attempts.
		result = CodexEffortProbeResult{Model: model, Status: "failed", Reason: "request_failed"}
		delete(payload, "type")
		delete(payload, "prompt_cache_key")
		delete(payload, "client_metadata")
	}
	if ctx.Err() != nil {
		result.Reason = "request_failed"
		return result
	}
	body, err := json.Marshal(payload)
	if err != nil {
		result.Reason = "request_build_failed"
		return result
	}
	reqCtx, cancel := context.WithTimeout(WithHTTPUpstreamProfile(ctx, HTTPUpstreamProfileOpenAI), codexEffortProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, chatgptCodexURL, bytes.NewReader(body))
	if err != nil {
		result.Reason = "request_build_failed"
		return result
	}
	req.Host = "chatgpt.com"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "text/event-stream")
	canonical := resolveCodexOutboundIdentity("")
	req.Header.Set("Originator", canonical.originator)
	req.Header.Set("Version", canonical.version)
	req.Header.Set("User-Agent", canonical.userAgent)
	enforceCodexIdentityHeadersWithUA(req.Header, account.GetOpenAIUserAgent())
	setOpenAIChatGPTAccountHeaders(req.Header, account)
	result.Transport = "http"
	resp, err := s.doOpenAIUpstream(req, proxyURL, account)
	if err != nil {
		result.Reason = "request_failed"
		return result
	}
	defer func() { _ = resp.Body.Close() }()
	result.HTTPStatus = resp.StatusCode
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	result.Status, result.Reason, result.SupportedEfforts = classifyCodexEffortProbeResponse(resp.StatusCode, responseBody)
	return result
}

func (s *OpenAIGatewayService) buildCodexEffortProbeWSRequest(ctx context.Context, account *Account, token string, payload map[string]any) (string, http.Header, error) {
	source, err := s.prepareCodexAccountIdentitySource(ctx, nil, account)
	if err != nil {
		return "", nil, err
	}
	wsURL, err := s.buildOpenAIResponsesWSURL(account)
	if err != nil {
		return "", nil, err
	}
	payload["type"] = "response.create"
	payload["prompt_cache_key"] = codexEffortProbeSession
	payload["client_metadata"] = map[string]any{"session_id": codexEffortProbeSession, "thread_id": codexEffortProbeSession}
	applyCodexAccountIdentityClientMetadataMap(payload, source, 0)
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+token)
	headers.Set("OpenAI-Beta", openAIWSBetaV2Value)
	canonical := resolveCodexOutboundIdentity("")
	headers.Set("Originator", canonical.originator)
	headers.Set("User-Agent", canonical.userAgent)
	if customUA := s.codexIdentityOverrideUA(account); strings.TrimSpace(customUA) != "" {
		headers.Set("User-Agent", customUA)
	}
	enforceCodexIdentityHeadersWithUA(headers, s.codexIdentityOverrideUA(account))
	if err := resolveAndSetOpenAIChatGPTAccountHeaders(ctx, s.accountRepo, headers, account); err != nil {
		return "", nil, err
	}
	applyOpenAICodexBetaFeatures(nil, account, headers)
	if err := normalizeCodexRequestMetadataMap(payload, headers, account); err != nil {
		return "", nil, err
	}
	// Probe sessions only use the official hyphenated IDs, also when identity enforcement is off.
	deleteHeaderAllForms(headers, "session_id")
	deleteHeaderAllForms(headers, "conversation_id")
	projectCodexOutboundHeaders(headers, true)
	return wsURL, headers, nil
}

func (s *OpenAIGatewayService) runCodexEffortProbeWS(ctx context.Context, account *Account, token, proxyURL string, payload map[string]any, result CodexEffortProbeResult) (CodexEffortProbeResult, bool) {
	wsURL, headers, err := s.buildCodexEffortProbeWSRequest(ctx, account, token, payload)
	if err != nil {
		result.Reason = "request_build_failed"
		return result, false
	}
	wsCtx, cancel := context.WithTimeout(ctx, codexEffortProbeTimeout)
	defer cancel()
	if ctx.Err() != nil {
		result.Reason = "request_failed"
		return result, false
	}
	result.Transport, result.Reason = "ws", "ws_handshake_failed"
	conn, _, _, err := s.getOpenAIWSPassthroughDialer().Dial(wsCtx, wsURL, headers, proxyURL)
	if err != nil {
		return result, ctx.Err() == nil
	}
	defer func() {
		if closer, ok := conn.(openAIWSForceCloser); ok {
			_ = closer.CloseNow()
		} else {
			_ = conn.Close()
		}
	}()
	result.Reason = "ws_write_failed"
	if err := conn.WriteJSON(wsCtx, payload); err != nil {
		return result, ctx.Err() == nil
	}
	metadataReceived := false
	// Only validation rejections get a bounded metadata drain: at most 8 frames
	// and 250ms, still inside the original 15s WS deadline.
	readCtx := wsCtx
	drainRemaining := -1
	for {
		if drainRemaining == 0 {
			return result, false
		}
		message, err := conn.ReadMessage(readCtx)
		if err != nil {
			if drainRemaining >= 0 {
				return result, false
			}
			result.Reason = "ws_read_failed"
			return result, ctx.Err() == nil
		}
		if drainRemaining > 0 {
			drainRemaining--
		}
		event := gjson.GetBytes(message, "type").String()
		if event == "codex.response.metadata" {
			metadataReceived = true
			headers := gjson.GetBytes(message, "headers")
			switch headers.Get("x-codex-safety-buffering-enabled").String() {
			case "true":
				v := true
				result.SafetyBufferingEnabled = &v
			case "false":
				v := false
				result.SafetyBufferingEnabled = &v
			}
			if v := headers.Get("x-codex-safety-buffering-faster-model"); v.Type == gjson.String {
				result.SafetyBufferingFasterModel = v.String()
			}
			if v := headers.Get(openAIWSTurnStateHeader); v.Type == gjson.String {
				length := len(v.String())
				result.TurnStateLen = &length
			}
			if drainRemaining >= 0 {
				return result, false
			}
			continue
		}
		if drainRemaining >= 0 {
			continue
		}
		if event == "error" || isOpenAIWSTerminalEvent(event) {
			if event == "error" {
				result.HTTPStatus = openAIWSErrorHTTPStatus(message)
				_, _, errorMessage := parseOpenAIWSErrorEventFields(message)
				result.Status, result.Reason, result.SupportedEfforts = classifyCodexEffortProbeResponse(result.HTTPStatus, []byte(errorMessage))
			} else if event == "response.failed" || event == "response.cancelled" || event == "response.canceled" || (event == "response.incomplete" && gjson.GetBytes(message, "response.error").Exists()) {
				fields := gjson.GetManyBytes(message, "response.error.code", "response.error.type")
				result.HTTPStatus = openAIWSErrorHTTPStatusFromRaw(fields[0].String(), fields[1].String())
				result.Status, result.Reason, result.SupportedEfforts = classifyCodexEffortProbeResponse(result.HTTPStatus, []byte(extractOpenAISSEErrorMessage(message)))
			} else {
				result.HTTPStatus = http.StatusOK
				result.Status, result.Reason = "uncertain", "minimal_accepted"
			}
			if result.HTTPStatus != http.StatusBadRequest || metadataReceived {
				return result, false
			}
			var drainCancel context.CancelFunc
			readCtx, drainCancel = context.WithTimeout(wsCtx, 250*time.Millisecond)
			defer drainCancel()
			drainRemaining = 8
			continue
		}
		if event == "response.created" || event == "response.in_progress" || event == "response.output_item.added" || event == "response.output_item.done" || isOpenAIWSTokenEvent(event) {
			result.HTTPStatus = http.StatusOK
			result.Status, result.Reason = "uncertain", "minimal_accepted"
			return result, false
		}
	}
}
