package chat

import (
	json "encoding/json/v2"
	"strings"

	"9routerx/proxy/internal/translator"
)

func moonshotClaudeConversionBody(body []byte, model *ModelInfo) []byte {
	eligible := model.Provider == "moonshot"
	for _, target := range model.ComboModels {
		if strings.HasPrefix(target, "moonshot/") {
			eligible = true
		}
	}
	if !eligible {
		return body
	}
	var source map[string]any
	if json.Unmarshal(body, &source) != nil {
		return body
	}
	config, _ := source["output_config"].(map[string]any)
	if value, exists := config["effort"]; exists {
		if _, valid := value.(string); !valid {
			delete(config, "effort")
			out, err := json.Marshal(source, json.Deterministic(true))
			if err == nil {
				return out
			}
		}
	}
	return body
}

// Match the converter's content/tool-call identity rather than counting source
// assistant turns: thinking-only turns can legitimately disappear in conversion.
func restoreMoonshotClaudeMessages(source, target map[string]any) bool {
	original, _ := source["messages"].([]any)
	prepared, _ := target["messages"].([]any)
	index := 0
	changed := false
	for _, raw := range original {
		message, ok := raw.(map[string]any)
		if !ok || message["role"] != "assistant" {
			continue
		}
		blocks, _ := message["content"].([]any)
		var parts []string
		for _, rawBlock := range blocks {
			block, ok := rawBlock.(map[string]any)
			if ok && block["type"] == "thinking" {
				if text, ok := block["thinking"].(string); ok && text != "" {
					parts = append(parts, text)
				}
			}
		}
		encoded, err := json.Marshal(map[string]any{"messages": []any{message}}, json.Deterministic(true))
		if err != nil {
			continue
		}
		converted, err := translator.TranslateClaudeToOpenAI(encoded)
		if err != nil {
			continue
		}
		var envelope map[string]any
		if json.Unmarshal(converted, &envelope) != nil {
			continue
		}
		messages, _ := envelope["messages"].([]any)
		var expected map[string]any
		for _, raw := range messages {
			candidate, ok := raw.(map[string]any)
			if ok && candidate["role"] == "assistant" {
				expected = candidate
				break
			}
		}
		if expected == nil {
			continue
		}
		for index < len(prepared) {
			candidate, ok := prepared[index].(map[string]any)
			index++
			if !ok || candidate["role"] != "assistant" || !moonshotSameMessage(expected, candidate) {
				continue
			}
			if len(parts) > 0 {
				if _, supplied := candidate["reasoning_content"]; !supplied {
					candidate["reasoning_content"] = strings.Join(parts, "")
					changed = true
				}
			}
			break
		}
	}
	return changed
}

func moonshotSameMessage(expected, candidate map[string]any) bool {
	for _, key := range []string{"content", "tool_calls"} {
		left, err := json.Marshal(expected[key], json.Deterministic(true))
		if err != nil {
			return false
		}
		right, err := json.Marshal(candidate[key], json.Deterministic(true))
		if err != nil || string(left) != string(right) {
			return false
		}
	}
	return true
}
