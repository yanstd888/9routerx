package executor

import (
	json "encoding/json/v2"
	"fmt"
	"strings"
)

const (
	clovaMaxTokens         = 4096
	clovaReasoningMaxTokens = 32768
	clovaMinToolTokens      = 1024
)

func clovaReasoningModel(model string) bool { return strings.EqualFold(model, "HCX-007") }
func clovaVisionModel(model string) bool    { return strings.EqualFold(model, "HCX-005") }

func clovaNumber(value any, min, max float64) (float64, bool) {
	number, ok := value.(float64)
	if !ok {
		return 0, false
	}
	if number < min {
		number = min
	}
	if number > max {
		number = max
	}
	return number, true
}

func clovaThinkingEffort(value any) string {
	effort, ok := value.(string)
	if !ok {
		return ""
	}
	switch strings.ToLower(effort) {
	case "minimal":
		return "low"
	case "none", "low", "medium", "high":
		return strings.ToLower(effort)
	}
	return ""
}

func clovaContent(content any, vision bool) any {
	if text, ok := content.(string); ok {
		if !vision {
			return text
		}
		return []any{map[string]any{"type": "text", "text": text}}
	}
	parts, ok := content.([]any)
	if !ok {
		return ""
	}
	out := make([]any, 0, len(parts))
	for _, raw := range parts {
		part, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if text, ok := part["text"].(string); ok {
			out = append(out, map[string]any{"type": "text", "text": text})
			continue
		}
		if !vision {
			continue
		}
		image, _ := part["image_url"].(map[string]any)
		rawURL, _ := image["url"].(string)
		if rawURL == "" {
			rawURL, _ = part["url"].(string)
		}
		if rawURL == "" {
			continue
		}
		if strings.HasPrefix(rawURL, "data:") {
			out = append(out, map[string]any{"type": "image_url", "dataUri": map[string]any{"data": rawURL}})
		} else {
			out = append(out, map[string]any{"type": "image_url", "imageUrl": map[string]any{"url": rawURL}})
		}
	}
	return out
}

func clovaTools(tools any) []any {
	values, ok := tools.([]any)
	if !ok {
		return nil
	}
	out := make([]any, 0, len(values))
	for _, raw := range values {
		tool, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		fn, _ := tool["function"].(map[string]any)
		name, _ := fn["name"].(string)
		if name == "" {
			name, _ = tool["name"].(string)
		}
		if name == "" {
			continue
		}
		description, _ := fn["description"].(string)
		if description == "" {
			description, _ = tool["description"].(string)
		}
		if description == "" {
			description = "Tool: " + name
		}
		entry := map[string]any{"type": "function", "function": map[string]any{"name": name, "description": description}}
		if params, ok := fn["parameters"]; ok {
			entry["function"].(map[string]any)["parameters"] = params
		} else if params, ok := tool["parameters"]; ok {
			entry["function"].(map[string]any)["parameters"] = params
		}
		out = append(out, entry)
	}
	return out
}

func clovaArguments(raw any) map[string]any {
	if value, ok := raw.(map[string]any); ok {
		return value
	}
	if text, ok := raw.(string); ok && strings.TrimSpace(text) != "" {
		var parsed map[string]any
		if json.Unmarshal([]byte(text), &parsed) == nil {
			return parsed
		}
	}
	return map[string]any{}
}

// clovaPayload builds the native Chat Completions v3 body. Tool mode and
// structured-output mode are mutually exclusive; this port implements plain
// and tool modes from the pin and leaves response_format to the upstream
// plain schema rather than fabricating a CLOVA responseFormat envelope.
func clovaPayload(model string, body []byte) ([]byte, error) {
	var request map[string]any
	if err := json.Unmarshal(body, &request); err != nil || request == nil {
		return nil, fmt.Errorf("decode OpenAI request")
	}
	tools := clovaTools(request["tools"])
	mode := "plain"
	if len(tools) > 0 {
		mode = "tools"
	}
	vision := mode == "plain" && clovaVisionModel(model)
	messages, _ := request["messages"].([]any)
	converted := make([]any, 0, len(messages))
	for _, raw := range messages {
		message, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role, _ := message["role"].(string)
		switch role {
		case "system", "assistant":
		default:
			role = "user"
		}
		if role == "assistant" && mode == "tools" {
			calls, _ := message["tool_calls"].([]any)
			if len(calls) > 0 {
				toolCalls := make([]any, 0, len(calls))
				for _, rawCall := range calls {
					call, _ := rawCall.(map[string]any)
					fn, _ := call["function"].(map[string]any)
					name, _ := fn["name"].(string)
					id, _ := call["id"].(string)
					toolCalls = append(toolCalls, map[string]any{
						"id": id, "type": "function",
						"function": map[string]any{"name": name, "arguments": clovaArguments(fn["arguments"])},
					})
				}
				converted = append(converted, map[string]any{"role": "assistant", "content": "", "toolCalls": toolCalls})
				continue
			}
		}
		if role == "tool" && mode == "tools" {
			text, _ := message["content"].(string)
			entry := map[string]any{"role": "tool", "content": text}
			if id, ok := message["tool_call_id"].(string); ok {
				entry["toolCallId"] = id
			}
			converted = append(converted, entry)
			continue
		}
		converted = append(converted, map[string]any{"role": role, "content": clovaContent(message["content"], vision)})
	}
	payload := map[string]any{"messages": converted}
	reasoning := clovaReasoningModel(model)
	if reasoning {
		effort := clovaThinkingEffort(request["reasoning_effort"])
		if mode != "plain" || effort == "" {
			effort = "none"
		}
		payload["thinking"] = map[string]any{"effort": effort}
	}
	if temperature, ok := clovaNumber(request["temperature"], 0, 1); ok {
		payload["temperature"] = temperature
	}
	if topP, ok := clovaNumber(request["top_p"], 0, 1); ok && topP > 0 {
		payload["topP"] = topP
	}
	if topK, ok := clovaNumber(request["top_k"], 0, 128); ok && topK > 0 {
		payload["topK"] = topK
	}
	if penalty, ok := clovaNumber(request["repetition_penalty"], 0, 2); ok && penalty > 0 {
		payload["repetitionPenalty"] = penalty
	}
	cap := float64(clovaMaxTokens)
	key := "maxTokens"
	if reasoning {
		cap = clovaReasoningMaxTokens
		key = "maxCompletionTokens"
	}
	if tokens, ok := clovaNumber(firstPresent(request, "max_completion_tokens", "max_tokens"), 1, cap); ok {
		if mode == "tools" && tokens < clovaMinToolTokens {
			tokens = clovaMinToolTokens
		}
		payload[key] = int(tokens)
	} else if mode == "tools" && cap >= clovaMinToolTokens {
		payload[key] = clovaMinToolTokens
	}
	if mode == "tools" {
		payload["tools"] = tools
		switch request["tool_choice"] {
		case "none":
			payload["toolChoice"] = "none"
		case "auto", "required":
			payload["toolChoice"] = "auto"
		}
	}
	if mode == "plain" && !reasoning {
		if stop, ok := request["stop"].(string); ok && stop != "" {
			payload["stop"] = []string{stop}
		} else if stops, ok := request["stop"].([]any); ok {
			values := make([]string, 0, len(stops))
			for _, value := range stops {
				if text, ok := value.(string); ok {
					values = append(values, text)
				}
			}
			if len(values) > 0 {
				payload["stop"] = values
			}
		}
	}
	if seed, ok := clovaNumber(request["seed"], 0, 4294967295); ok && seed > 0 {
		payload["seed"] = int(seed)
	}
	if request["include_ai_filters"] == true {
		payload["includeAiFilters"] = true
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode CLOVA payload: %w", err)
	}
	return encoded, nil
}

func firstPresent(payload map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := payload[key]; ok && value != nil {
			return value
		}
	}
	return nil
}
