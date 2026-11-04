//go:build unit

package service

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/MACOS-DO/sub4api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 非流式聚合必须保留带 thoughtSignature 的普通文本，包括只为承载签名而
// 存在的空文本 part；下一轮重放依赖这些不透明签名上下文。
// 对应上游 feab6d80d 对 mergeCollectedPartsToResponse 的 helper 修复，
// Master 已用 Go overlay 在原实现上独立复现两个失败。
func TestAntigravitySignedTextAggregation(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
	}{
		{name: "signed text", text: "answer"},
		{name: "signature-only text", text: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := []map[string]any{{"text": tc.text, "thoughtSignature": "opaque-signed-context"}}
			result := mergeCollectedPartsToResponse(map[string]any{}, input)
			wire, err := json.Marshal(result)
			require.NoError(t, err)
			var decoded struct {
				Candidates []struct {
					Content struct {
						Parts []map[string]any `json:"parts"`
					} `json:"content"`
				} `json:"candidates"`
			}
			require.NoError(t, json.Unmarshal(wire, &decoded))
			require.Len(t, decoded.Candidates, 1)
			require.Len(t, decoded.Candidates[0].Content.Parts, 1, "签名专用空文本 part 也必须保留")
			require.Equal(t, tc.text, decoded.Candidates[0].Content.Parts[0]["text"])
			require.Equal(t, "opaque-signed-context", decoded.Candidates[0].Content.Parts[0]["thoughtSignature"], "聚合不得丢失上游不透明重放上下文")
		})
	}

	// 跨函数调用顺序：签名文本 → functionCall → 收尾文本，签名与顺序都保留。
	t.Run("cross function call ordering", func(t *testing.T) {
		input := []map[string]any{
			{"text": "before", "thoughtSignature": "sig-before"},
			{"functionCall": map[string]any{"name": "lookup", "args": map[string]any{}}},
			{"text": "after"},
		}
		result := mergeCollectedPartsToResponse(map[string]any{}, input)
		wire, err := json.Marshal(result)
		require.NoError(t, err)
		var decoded struct {
			Candidates []struct {
				Content struct {
					Parts []map[string]any `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		require.NoError(t, json.Unmarshal(wire, &decoded))
		parts := decoded.Candidates[0].Content.Parts
		require.Len(t, parts, 3)
		require.Equal(t, "before", parts[0]["text"])
		require.Equal(t, "sig-before", parts[0]["thoughtSignature"])
		require.Contains(t, parts[1], "functionCall")
		require.Equal(t, "after", parts[2]["text"])
		require.NotContains(t, parts[2], "thoughtSignature", "后续未签名文本不得继承前面的签名")
	})
}

// 原生 Gemini 非流式聚合（handleGeminiStreamToNonStreaming）不丢
// thoughtSignature，且 usage 照常收集。
func TestAntigravityGeminiNonStreamCollectKeepsSignedText(t *testing.T) {
	gin.SetMode(gin.TestMode)
	frames := `data: {"response":{"candidates":[{"content":{"parts":[{"text":"answer","thoughtSignature":"opaque-signed-context"}]}}]}}` + "\n\n" +
		`data: {"response":{"candidates":[{"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":9,"candidatesTokenCount":4}}}` + "\n\n"

	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1beta/models/gemini-3.1-pro:generateContent", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(frames))}

	result, err := svc.handleGeminiStreamToNonStreaming(c, resp, time.Now())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 9, result.usage.InputTokens)
	require.Equal(t, 4, result.usage.OutputTokens)

	body := recorder.Body.String()
	require.Contains(t, body, `"text":"answer"`)
	require.Contains(t, body, "opaque-signed-context", "原生 Gemini 非流式聚合必须保留签名")
}

// Claude 兼容非流式聚合（collectClaudeStreamResponse）同样不丢签名：
// 签名文本经 TransformGeminiToClaude 物化为携带 signature 的 thinking 块；
// usage 随携带 parts 的最终帧保留。
func TestAntigravityClaudeNonStreamCollectKeepsSignedText(t *testing.T) {
	gin.SetMode(gin.TestMode)
	frames := `data: {"response":{"candidates":[{"content":{"parts":[{"text":"answer","thoughtSignature":"sig-answer"}]}}]}}` + "\n\n" +
		`data: {"response":{"candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup","args":{}},"thoughtSignature":"sig-call"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":3}}}` + "\n\n"

	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)
	c, _ := newAntigravityCompatContext(http.MethodPost, "/v1/messages", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(frames))}

	claudeResp, result, err := svc.collectClaudeStreamResponse(c, resp, time.Now(), "claude-sonnet-4-5")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 11, result.usage.InputTokens)
	require.Equal(t, 3, result.usage.OutputTokens)

	require.Contains(t, string(claudeResp), "answer")
	require.Contains(t, string(claudeResp), "sig-answer", "Claude 聚合路径必须把签名保留为可重放的 thinking 块")
	require.Contains(t, string(claudeResp), `"type":"thinking"`)
	require.Contains(t, string(claudeResp), `"type":"tool_use"`)
}
