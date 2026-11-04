//go:build unit

package service

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/MACOS-DO/sub4api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Gemini 原生非流式请求在上游以流式返回时，必须聚合跨帧的所有 parts：
// 早期帧的 functionCall 不能因为后续帧带新 parts 而丢失，
// 文本与 functionCall 按原始顺序保留在最终响应中。
func TestAntigravityGeminiNonStreamCollectKeepsAllParts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	frames := `data: {"response":{"candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup","args":{"q":"hi"}}}]}}]}}` + "\n\n" +
		`data: {"response":{"candidates":[{"content":{"parts":[{"text":"done"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2}}}` + "\n\n"

	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1beta/models/gemini-3.1-pro:generateContent", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(frames))}

	result, err := svc.handleGeminiStreamToNonStreaming(c, resp, time.Now())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, recorder.Code)

	body := recorder.Body.String()
	require.Contains(t, body, `"functionCall"`)
	require.Contains(t, body, `"lookup"`)
	require.Contains(t, body, `"text"`)
	require.Contains(t, body, "done")
	// functionCall 先于最终文本，顺序保持。
	require.Less(t, strings.Index(body, `"lookup"`), strings.Index(body, `"done"`))
}

// 图片 part 出现在早期帧、后续帧只有 finishReason 时，图片不能被丢弃。
func TestAntigravityGeminiNonStreamCollectKeepsEarlyImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	frames := `data: {"response":{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"aW1nMQ=="}}]}}]}}` + "\n\n" +
		`data: {"response":{"candidates":[{"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":1}}}` + "\n\n"

	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1beta/models/gemini-3.1-flash-image:generateContent", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(frames))}

	_, err := svc.handleGeminiStreamToNonStreaming(c, resp, time.Now())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "aW1nMQ==")
}
