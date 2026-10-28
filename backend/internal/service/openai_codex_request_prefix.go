package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Codex 0.164 sends instructions as developer input, with thread-scoped stable
// prefix IDs so retries and resumed WS requests can reuse their input prefix.
func normalizeCodexRequestPrefix(body []byte, threadID string) ([]byte, bool, error) {
	instructions := gjson.GetBytes(body, "instructions")
	input := gjson.GetBytes(body, "input")
	var items []json.RawMessage
	if input.IsArray() {
		input.ForEach(func(_, item gjson.Result) bool {
			items = append(items, json.RawMessage(item.Raw))
			return true
		})
	} else if input.Type == gjson.String && instructions.Type == gjson.String && instructions.Str != "" {
		item, err := marshalOpenAIUpstreamJSON(map[string]any{"type": "message", "role": "user", "content": input.Str})
		if err != nil {
			return body, false, err
		}
		items = append(items, item)
	} else if input.Exists() && input.Type != gjson.Null && input.Type != gjson.String {
		return body, false, nil
	}

	changed := false
	if instructions.Type == gjson.String && instructions.Str != "" {
		index := 0
		for index < len(items) && gjson.GetBytes(items[index], "type").String() == "additional_tools" {
			index++
		}
		var existing map[string]any
		if index < len(items) {
			if err := decodeOpenAIJSONUseNumber(items[index], &existing); err != nil {
				return body, false, err
			}
		}
		if !isOpenAIResponsesLiteInstruction(existing, instructions.Str) {
			message, err := marshalOpenAIUpstreamJSON(map[string]any{
				"type": "message", "role": "developer",
				"content": []any{map[string]any{"type": "input_text", "text": instructions.Str}},
			})
			if err != nil {
				return body, false, err
			}
			items = append(items, nil)
			copy(items[index+1:], items[index:])
			items[index] = message
			changed = true
		}
	}

	prefixNamespace := uuid.NewSHA1(uuid.NameSpaceOID, []byte(threadID))
	inPrefix := true
	for i, item := range items {
		role := gjson.GetBytes(item, "role").String()
		if role == "system" {
			var err error
			item, err = sjson.SetBytes(item, "role", "developer")
			if err != nil {
				return body, false, err
			}
			role = "developer"
			changed = true
		}
		kind := gjson.GetBytes(item, "type").String()
		inPrefix = inPrefix && (kind == "additional_tools" || ((kind == "message" || kind == "") && role == "developer"))
		if inPrefix && threadID != "" && !gjson.GetBytes(item, "id").Exists() {
			prefix := ""
			var text []byte
			if kind == "additional_tools" {
				tools := gjson.GetBytes(item, "tools")
				if tools.IsArray() && len(tools.Array()) > 0 {
					// shortcut: hash emitted tool JSON; exact Rust field order needs a tool-schema serializer.
					var compact bytes.Buffer
					if err := json.Compact(&compact, []byte(tools.Raw)); err != nil {
						return body, false, err
					}
					prefix, text = "at", compact.Bytes()
				}
			} else {
				content := gjson.GetBytes(item, "content")
				if content.Type == gjson.String {
					prefix, text = "msg", []byte(content.Str)
				} else if parts := content.Array(); content.IsArray() && len(parts) == 1 && parts[0].Get("type").String() == "input_text" {
					prefix, text = "msg", []byte(parts[0].Get("text").String())
				}
			}
			if prefix != "" {
				var err error
				item, err = sjson.SetBytes(item, "id", prefix+"_"+uuid.NewSHA1(prefixNamespace, text).String())
				if err != nil {
					return body, false, err
				}
				changed = true
			}
		}
		items[i] = item
	}
	updated := body
	if changed {
		raw, err := marshalOpenAIUpstreamJSON(items)
		if err != nil {
			return body, false, err
		}
		updated, err = sjson.SetRawBytes(updated, "input", raw)
		if err != nil {
			return body, false, err
		}
	}
	if instructions.Exists() && (instructions.Type == gjson.String || instructions.Type == gjson.Null) {
		var err error
		updated, err = sjson.DeleteBytes(updated, "instructions")
		if err != nil {
			return body, false, fmt.Errorf("remove migrated Codex instructions: %w", err)
		}
		changed = true
	}
	return updated, changed, nil
}

func codexInputHasInstructions(body []byte) bool {
	found := false
	gjson.GetBytes(body, "input").ForEach(func(_, item gjson.Result) bool {
		role := item.Get("role").String()
		if role == "developer" || role == "system" {
			content := item.Get("content")
			found = (content.Type == gjson.String && strings.TrimSpace(content.Str) != "") || (content.IsArray() && len(content.Array()) > 0)
		}
		return !found
	})
	return found
}
