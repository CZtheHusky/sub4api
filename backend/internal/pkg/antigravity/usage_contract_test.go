//go:build unit

package antigravity

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Gemini usageMetadata 只有 cachedContentTokenCount（缓存命中），没有缓存写入的 token 类别：
// 两条转换路径产出的 Claude usage 中 cache_creation_input_tokens 恒为 0，缓存命中计入
// cache_read_input_tokens 并从 input_tokens 中扣除。
func TestGeminiUsageMapping_NoCacheCreationTokens(t *testing.T) {
	const geminiBody = `{"candidates":[{"content":{"parts":[{"text":"hi"}],"role":"model"},"finishReason":"STOP"}],` +
		`"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":20,"cachedContentTokenCount":30,"thoughtsTokenCount":5}}`

	t.Run("non-stream", func(t *testing.T) {
		_, usage, err := TransformGeminiToClaude([]byte(geminiBody), "gemini-3.1-pro-preview")
		require.NoError(t, err)
		require.NotNil(t, usage)
		require.Equal(t, 70, usage.InputTokens)
		require.Equal(t, 25, usage.OutputTokens)
		require.Equal(t, 30, usage.CacheReadInputTokens)
		require.Zero(t, usage.CacheCreationInputTokens)
	})

	t.Run("stream", func(t *testing.T) {
		p := NewStreamingProcessor("gemini-3.1-pro-preview")
		out := p.ProcessLine(`data: {"response":` + geminiBody + `}`)
		require.True(t, strings.Contains(string(out), `"message_start"`))
		require.NotContains(t, string(out), `"cache_creation_input_tokens"`, "message_start 的 usage 不应携带缓存写入分项")
		_, usage := p.Finish()
		require.NotNil(t, usage)
		require.Equal(t, 70, usage.InputTokens)
		require.Equal(t, 30, usage.CacheReadInputTokens)
		require.Zero(t, usage.CacheCreationInputTokens)
	})
}

// prompt 被拦截（promptFeedback.blockReason）时上游可能只报 cachedContentTokenCount、
// 缺 promptTokenCount：input_tokens 差值下限为 0，缓存命中仍照上游所报计入。
func TestGeminiUsageMapping_MissingPromptTokenCountKeepsInputNonNegative(t *testing.T) {
	const blockedBody = `{"candidates":[{"content":{"parts":[{"text":"blocked"}],"role":"model"},"finishReason":"STOP"}],` +
		`"usageMetadata":{"candidatesTokenCount":2,"cachedContentTokenCount":40,"thoughtsTokenCount":0}}`

	t.Run("non-stream", func(t *testing.T) {
		_, usage, err := TransformGeminiToClaude([]byte(blockedBody), "gemini-3.1-pro-preview")
		require.NoError(t, err)
		require.NotNil(t, usage)
		require.Zero(t, usage.InputTokens)
		require.Equal(t, 40, usage.CacheReadInputTokens)
	})

	t.Run("stream", func(t *testing.T) {
		p := NewStreamingProcessor("gemini-3.1-pro-preview")
		out := p.ProcessLine(`data: {"response":` + blockedBody + `}`)
		require.True(t, strings.Contains(string(out), `"message_start"`))
		require.Contains(t, string(out), `"input_tokens":0`, "message_start 的 input_tokens 不应为负")
		require.Contains(t, string(out), `"cache_read_input_tokens":40`)
		_, usage := p.Finish()
		require.NotNil(t, usage)
		require.Zero(t, usage.InputTokens)
		require.Equal(t, 40, usage.CacheReadInputTokens)
	})
}
