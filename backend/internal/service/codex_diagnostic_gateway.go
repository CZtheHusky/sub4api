package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"
)

// CodexGatewayDiagnosticRunner 把降智探测请求作为"官方 Codex 客户端入站请求"
// 打进本进程 gin 引擎：经完整中间件链（内部诊断令牌合成认证）→ 调度钉账号 →
// 保真转发器出站。全 repo 只维护一套指纹模拟（网关转发链路），检测自动继承
// 所有保真增强。探测零计费：不产生 usage/billing 记录、不消耗任何用户额度。
type CodexGatewayDiagnosticRunner struct {
	auth        *CodexDiagnosticInternalAuth
	gateway     *OpenAIGatewayService
	accountRepo AccountRepository
	attempts    CodexTicketAttemptRepository

	mu     sync.RWMutex
	engine http.Handler

	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// ProvideCodexGatewayDiagnosticRunner 构造 runner 并启动 codex_ticket_attempts
// 分区滚动维护 goroutine（启动一次 + 每 24h；随打票删除迁移至此）。
func ProvideCodexGatewayDiagnosticRunner(
	auth *CodexDiagnosticInternalAuth,
	gateway *OpenAIGatewayService,
	accountRepo AccountRepository,
	attempts CodexTicketAttemptRepository,
) *CodexGatewayDiagnosticRunner {
	r := &CodexGatewayDiagnosticRunner{
		auth:        auth,
		gateway:     gateway,
		accountRepo: accountRepo,
		attempts:    attempts,
		stopCh:      make(chan struct{}),
	}
	if attempts != nil {
		r.wg.Add(1)
		go r.runCleanupLoop()
	}
	return r
}

// SetEngine 由 server.ProvideRouter 构造 gin 引擎后回调注入。
func (r *CodexGatewayDiagnosticRunner) SetEngine(engine http.Handler) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.engine = engine
	r.mu.Unlock()
}

func (r *CodexGatewayDiagnosticRunner) getEngine() http.Handler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.engine
}

// Stop 停止分区清理 goroutine（wire cleanup 调用）。
func (r *CodexGatewayDiagnosticRunner) Stop() {
	if r == nil {
		return
	}
	r.stopOnce.Do(func() {
		close(r.stopCh)
	})
	r.wg.Wait()
}

// runCleanupLoop 分区滚动维护：启动时一次，之后每日一次（预建未来日分区/清理过期分区）。
func (r *CodexGatewayDiagnosticRunner) runCleanupLoop() {
	defer r.wg.Done()
	r.cleanupOnce()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.cleanupOnce()
		}
	}
}

func (r *CodexGatewayDiagnosticRunner) cleanupOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := r.attempts.Cleanup(ctx); err != nil {
		slog.Warn("[codex-diagnostic] attempt partition cleanup failed", "error", err)
	}
}

// RunGatewayDiagnostic 经网关指纹通路对目标账号打一发 ModelTrace 探测。
// challenge 必须由调用方先生成，保证请求正文与最终判定用同一次抽签。
// 返回值：output 可评分输出；httpStatus 网关响应码；gatewayErrCode 网关结构化
// 错误码（展示用）；reason 非空表示本次探测没有拿到可评分的输出。
func (r *CodexGatewayDiagnosticRunner) RunGatewayDiagnostic(ctx context.Context, accountID int64, model string, challenge ModelTraceChallenge, timeout time.Duration) (output string, httpStatus int, gatewayErrCode string, reason string) {
	if r == nil || r.gateway == nil || r.accountRepo == nil || r.auth == nil {
		return "", 0, "", "unavailable"
	}
	engine := r.getEngine()
	if engine == nil {
		return "", 0, "", "engine_unavailable"
	}
	probeCtx, err := r.gateway.PrepareCodexProbeContext(ctx)
	if err != nil {
		reason = "template_unavailable"
		if errors.Is(err, ErrCodexProbeTemplateInvalid) {
			reason = "template_invalid"
		}
		return "", 0, "", reason
	}
	account, err := r.accountRepo.GetByID(probeCtx, accountID)
	if err != nil || account == nil {
		return "", 0, "", "identity_resolution_failed"
	}
	groupID, ok := ResolveCodexDiagnosticGroup(account)
	if !ok {
		return "", 0, "", "no_group"
	}
	body, probeHeaders, err := r.gateway.BuildCodexDiagnosticRequest(probeCtx, accountID, model, challenge)
	if err != nil {
		reason = "request_build_failed"
		if errors.Is(err, ErrCodexProbeIdentity) {
			reason = "identity_resolution_failed"
		}
		return "", 0, "", reason
	}
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	modelCtx, cancel := context.WithTimeout(WithCodexDiagnosticGroup(WithCodexTicketDiagnostic(probeCtx, accountID), groupID), timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(modelCtx, http.MethodPost, "/v1/responses", bytes.NewReader(body))
	if err != nil {
		return "", 0, "", "request_build_failed"
	}
	for name, values := range probeHeaders {
		request.Header[name] = values
	}
	request.Header.Set("Authorization", "Bearer "+r.auth.Token())
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.RemoteAddr = "127.0.0.1:0"
	writer := httptest.NewRecorder()
	engine.ServeHTTP(writer, request)

	httpStatus = writer.Code
	if httpStatus < 200 || httpStatus >= 300 {
		return "", httpStatus, codexDiagnosticGatewayError(writer.Body.Bytes()), "http_status"
	}
	diagnosticText, complete := codexDiagnosticOutput(writer.Body.Bytes())
	if !complete || strings.TrimSpace(diagnosticText) == "" {
		return "", httpStatus, "", "response_incomplete"
	}
	return diagnosticText, httpStatus, "", ""
}

// ResolveCodexDiagnosticGroup 解析诊断目标账号所属的 OpenAI 分组（取最小
// group id，保证调度钉账号在确定的分组内进行）。无 OpenAI 分组时返回 false。
func ResolveCodexDiagnosticGroup(account *Account) (int64, bool) {
	if account == nil {
		return 0, false
	}
	groupID := int64(0)
	for _, ag := range account.AccountGroups {
		if ag.GroupID <= 0 || ag.Group == nil || ag.Group.Platform != PlatformOpenAI {
			continue
		}
		if groupID == 0 || ag.GroupID < groupID {
			groupID = ag.GroupID
		}
	}
	if groupID == 0 {
		for _, g := range account.Groups {
			if g == nil || g.ID <= 0 || g.Platform != PlatformOpenAI {
				continue
			}
			if groupID == 0 || g.ID < groupID {
				groupID = g.ID
			}
		}
	}
	return groupID, groupID > 0
}

// codexDiagnosticGatewayError 从网关错误响应中提取结构化错误码（reason /
// error.code），只放行安全的字符集，截断超长响应。
func codexDiagnosticGatewayError(raw []byte) string {
	if len(raw) > 16<<10 {
		raw = raw[:16<<10]
	}
	for _, path := range []string{"reason", "error.code"} {
		code := gjson.GetBytes(raw, path).String()
		if code == "" || len(code) > 64 {
			continue
		}
		for _, character := range code {
			if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '_' || character == '-' || character == '.' {
				continue
			}
			code = ""
			break
		}
		if code != "" {
			return code
		}
	}
	return ""
}

// codexDiagnosticOutput 解析网关响应的模型输出文本：SSE 流（delta 拼接，
// completed 兜底）或非流式 JSON（status=completed）。bool 返回响应是否完整。
func codexDiagnosticOutput(raw []byte) (string, bool) {
	completed := false
	streamed := false
	var output strings.Builder
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 2<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			continue
		}
		streamed = true
		switch gjson.Get(payload, "type").String() {
		case "response.output_text.delta":
			_, _ = output.WriteString(gjson.Get(payload, "delta").String())
		case "response.completed":
			completed = true
			if output.Len() == 0 {
				gjson.Get(payload, "response.output").ForEach(func(_, message gjson.Result) bool {
					message.Get("content").ForEach(func(_, part gjson.Result) bool {
						if part.Get("type").String() == "output_text" {
							_, _ = output.WriteString(part.Get("text").String())
						}
						return true
					})
					return true
				})
			}
		}
	}
	if streamed {
		return output.String(), completed && scanner.Err() == nil
	}
	var body map[string]any
	if json.Unmarshal(raw, &body) != nil {
		return "", false
	}
	if gjson.GetBytes(raw, "status").String() != "completed" {
		return "", false
	}
	if text := gjson.GetBytes(raw, "output_text").String(); text != "" {
		return text, true
	}
	return gjson.GetBytes(raw, "output.0.content.0.text").String(), true
}
