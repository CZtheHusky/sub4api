//go:build unit

package antigravity

import (
	"errors"
	"testing"
)

// 上游以 MALFORMED_FUNCTION_CALL 结束时工具调用参数无效，
// 非流式转换必须拒绝而不是产出一条看似成功的响应。
func TestMalformedFunctionCallTransform(t *testing.T) {
	for _, body := range []string{
		`{"candidates":[{"finishReason":"MALFORMED_FUNCTION_CALL","content":{"parts":[{"thoughtSignature":"test-signature"}]}}]}`,
		`{"response":{"candidates":[{"finishReason":"MALFORMED_FUNCTION_CALL"}]}}`,
	} {
		response, _, err := TransformGeminiToClaude([]byte(body), "test-model")
		if !errors.Is(err, ErrMalformedFunctionCall) || response != nil {
			t.Fatalf("expected malformed call error without successful response: %s %v", response, err)
		}
	}
}
