package executor

import (
	json "encoding/json/v2"
	"fmt"
	"strings"
)

func nlpcloudResponse(raw []byte) (string, string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "{") {
		var payload struct {
			Response string `json:"response"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return "", "", fmt.Errorf("decode NLP Cloud JSON: %w", err)
		}
		return payload.Response, "json", nil
	}
	var deltas []string
	for _, block := range strings.Split(strings.ReplaceAll(trimmed, "\r\n", "\n"), "\n\n") {
		var lines []string
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "data:"):
				lines = append(lines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			case strings.HasPrefix(line, "event:"), strings.HasPrefix(line, "id:"), strings.HasPrefix(line, ":"):
				continue
			default:
				lines = append(lines, strings.TrimSpace(line))
			}
		}
		value := strings.Join(lines, "\n")
		if value == "" || value == "[DONE]" {
			continue
		}
		if delta := nlpcloudDelta(value); delta != "" {
			deltas = append(deltas, delta)
		}
	}
	return strings.Join(deltas, ""), "sse", nil
}

func nlpcloudDelta(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "[DONE]" {
		return ""
	}
	if strings.HasPrefix(trimmed, "{") {
		var payload map[string]any
		if json.Unmarshal([]byte(trimmed), &payload) == nil {
			for _, key := range []string{"response", "content", "token"} {
				if text, ok := payload[key].(string); ok {
					return text
				}
			}
		}
	}
	return trimmed
}

func nlpcloudChunk(id, model string, created int64, delta map[string]any, reason string) string {
	choice := map[string]any{"index": 0, "delta": delta, "finish_reason": nil}
	if reason != "" {
		choice["finish_reason"] = reason
	}
	frame := map[string]any{
		"id": id, "object": "chat.completion.chunk", "created": created,
		"model": model, "choices": []any{choice},
	}
	encoded, err := json.Marshal(frame)
	if err != nil {
		return ""
	}
	return "data: " + string(encoded) + "\n\n"
}

func nlpcloudFrames(raw []byte, id, model string, created int64) string {
	var out strings.Builder
	out.WriteString(nlpcloudChunk(id, model, created, map[string]any{"role": "assistant"}, ""))
	text, format, err := nlpcloudResponse(raw)
	if err == nil {
		if format == "json" && text != "" {
			out.WriteString(nlpcloudChunk(id, model, created, map[string]any{"content": text}, ""))
		} else if format == "sse" {
			for _, block := range strings.Split(strings.ReplaceAll(strings.TrimSpace(string(raw)), "\r\n", "\n"), "\n\n") {
				var lines []string
				for _, line := range strings.Split(block, "\n") {
					switch {
					case strings.HasPrefix(line, "data:"):
						lines = append(lines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
					case strings.HasPrefix(line, "event:"), strings.HasPrefix(line, "id:"), strings.HasPrefix(line, ":"):
						continue
					default:
						lines = append(lines, strings.TrimSpace(line))
					}
				}
				value := strings.Join(lines, "\n")
				if delta := nlpcloudDelta(value); delta != "" && delta != "[DONE]" {
					out.WriteString(nlpcloudChunk(id, model, created, map[string]any{"content": delta}, ""))
				}
			}
		}
	}
	out.WriteString(nlpcloudChunk(id, model, created, map[string]any{}, "stop"))
	out.WriteString("data: [DONE]\n\n")
	return out.String()
}

func nlpcloudCompletion(raw []byte, id, model string, created int64) ([]byte, error) {
	text, _, err := nlpcloudResponse(raw)
	if err != nil {
		return nil, err
	}
	tokens := len(text) / 4
	if tokens < 1 {
		tokens = 1
	}
	completion := map[string]any{
		"id": id, "object": "chat.completion", "created": created, "model": model,
		"choices": []any{map[string]any{
			"index":         0,
			"message":       map[string]any{"role": "assistant", "content": text},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{
			"prompt_tokens": tokens, "completion_tokens": tokens, "total_tokens": tokens * 2,
		},
	}
	encoded, err := json.Marshal(completion)
	if err != nil {
		return nil, fmt.Errorf("encode NLP Cloud completion: %w", err)
	}
	return encoded, nil
}
