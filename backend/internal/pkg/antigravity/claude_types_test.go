package antigravity

import (
	"encoding/json"
	"testing"
)

func TestDefaultModels_ContainsNewAndLegacyImageModels(t *testing.T) {
	t.Parallel()

	models := DefaultModels()
	byID := make(map[string]ClaudeModel, len(models))
	for _, m := range models {
		byID[m.ID] = m
	}

	requiredIDs := []string{
		"claude-fable-5-1",
		"claude-fable-5",
		"claude-opus-4-8",
		"claude-opus-4-6-thinking",
		"gemini-2.5-flash-image",
		"gemini-2.5-flash-image-preview",
		"gemini-3.1-flash-image",
		"gemini-3.1-flash-image-preview",
		"gemini-3-pro-image", // legacy compatibility
		"gemini-3.6-flash",
		"gemini-3.6-flash-high",
		"gemini-3.6-flash-low",
		"gemini-3.6-flash-medium",
		"gemini-3.6-flash-tiered",
		"gemini-3.7-flash",
		"gemini-3.7-flash-high",
		"gemini-3.7-flash-low",
		"gemini-3.7-flash-medium",
		"gemini-3.7-flash-tiered",
		"gemini-3.8-flash",
		"gemini-3.8-flash-high",
		"gemini-3.8-flash-low",
		"gemini-3.8-flash-medium",
		"gemini-3.8-flash-tiered",
	}

	for _, id := range requiredIDs {
		if _, ok := byID[id]; !ok {
			t.Fatalf("expected model %q to be exposed in DefaultModels", id)
		}
	}
}

// Anthropic 官方在 thinking 块上始终携带 signature 键（content_block_start 时为空）；
// 严格客户端缺少该键会解析失败，即使签名为空也必须输出。
func TestClaudeContentItem_ThinkingAlwaysHasSignature(t *testing.T) {
	t.Parallel()

	b, err := json.Marshal(ClaudeContentItem{Type: "thinking", Thinking: "draft"})
	if err != nil {
		t.Fatalf("marshal thinking item: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("decode thinking item: %v", err)
	}
	if _, ok := decoded["signature"]; !ok {
		t.Fatalf("thinking item must always include signature key, got %s", b)
	}
	if _, ok := decoded["thinking"]; !ok {
		t.Fatalf("thinking item must always include thinking key, got %s", b)
	}

	// 非 thinking 块保持原有序列化，不额外注入 signature。
	b, err = json.Marshal(ClaudeContentItem{Type: "text", Text: "hi"})
	if err != nil {
		t.Fatalf("marshal text item: %v", err)
	}
	decoded = nil
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("decode text item: %v", err)
	}
	if _, ok := decoded["signature"]; ok {
		t.Fatalf("text item should not carry signature key, got %s", b)
	}
}
