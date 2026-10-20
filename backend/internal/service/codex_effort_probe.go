package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	httppool "github.com/MACOS-DO/sub4api/internal/pkg/httpclient"
)

// CodexEffortProbeResult 是 400 effort 探针的单模型判定结果。探针向 chatgpt.com
// 的 /backend-api/codex/responses 发送 reasoning.effort="minimal"（正常 sol/astra
// 与降智 luna 两套目录都不支持该档位），请求必在校验层被 400 拒绝，并在报错中
// 列出服务侧实际支持的档位：列表含 "none" 即被降智路由（luna 系模型独有），
// 不含则未被降智。请求不进入生成，零 token 消耗，单次约 1s。
type CodexEffortProbeResult struct {
	Model            string
	Status           string // normal | degraded | uncertain | failed
	Reason           string
	HTTPStatus       int
	DurationMS       int
	SupportedEfforts []string
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

// RunCodexEffortProbe 直连上游执行 400 effort 探针。不走网关回放链路：探针只
// 依赖校验层的档位列表，必须避开分组 effort 策略改写与 failover 换账号，保证
// 探测的就是目标账号本身。
func (s *OpenAIGatewayService) RunCodexEffortProbe(ctx context.Context, accountID int64, model string) CodexEffortProbeResult {
	result := CodexEffortProbeResult{Model: model, Status: "failed", Reason: "identity_resolution_failed"}
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
	payload, err := json.Marshal(map[string]any{
		"model": model, "store": false, "stream": true,
		"instructions": "You are a helpful assistant.",
		"input":        []any{map[string]any{"role": "user", "content": []any{map[string]string{"type": "input_text", "text": "hi"}}}},
		"reasoning":    map[string]any{"effort": "minimal"},
		// 探针本应止步于校验层；若档位目录变更导致 minimal 被受理，限住最坏 token 消耗
		"max_output_tokens": 16,
	})
	if err != nil {
		result.Reason = "request_build_failed"
		return result
	}
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, chatgptCodexURL, bytes.NewReader(payload))
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
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	client, err := httppool.GetClient(httppool.Options{ProxyURL: proxyURL, Timeout: 15 * time.Second, ResponseHeaderTimeout: 10 * time.Second})
	if err != nil {
		result.Reason = "client_build_failed"
		return result
	}
	start := time.Now()
	resp, err := client.Do(req)
	result.DurationMS = int(time.Since(start).Milliseconds())
	if err != nil {
		result.Reason = "request_failed"
		return result
	}
	defer func() { _ = resp.Body.Close() }()
	result.HTTPStatus = resp.StatusCode
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	result.Status, result.Reason, result.SupportedEfforts = classifyCodexEffortProbeResponse(resp.StatusCode, body)
	return result
}
