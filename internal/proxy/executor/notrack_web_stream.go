package executor

import (
	json "encoding/json/v2"
	"fmt"
	"strings"
	"unicode/utf8"
)

type notrackEvent struct {
	Type      string `json:"type"`
	ChatID    string `json:"chat_id"`
	MessageID string `json:"message_id"`
	Chunk     string `json:"chunk"`
	Content   string `json:"content"`
	Turn      int    `json:"turn"`
}

func notrackEvents(raw []byte) []notrackEvent {
	var events []notrackEvent
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" || !strings.HasPrefix(payload, "{") {
			continue
		}
		var event notrackEvent
		if json.Unmarshal([]byte(payload), &event) == nil {
			events = append(events, event)
		}
	}
	return events
}

func notrackChunk(id, model string, created int64, delta map[string]any, finish string) string {
	choice := map[string]any{"index": 0, "delta": delta, "finish_reason": nil}
	if finish != "" {
		choice["finish_reason"] = finish
	}
	chunk := map[string]any{
		"id": id, "object": "chat.completion.chunk", "created": created, "model": model,
		"system_fingerprint": "fp_notrack", "choices": []any{choice},
	}
	encoded, err := json.Marshal(chunk)
	if err != nil {
		return ""
	}
	return "data: " + string(encoded) + "\n\n"
}

func notrackFrames(raw []byte, id, model string, created int64) string {
	var out strings.Builder
	roleEmitted, anyDelta := false, false
	full := ""
	emitRole := func() {
		if roleEmitted {
			return
		}
		roleEmitted = true
		out.WriteString(notrackChunk(id, model, created, map[string]any{"role": "assistant", "content": ""}, ""))
	}
	for _, event := range notrackEvents(raw) {
		switch event.Type {
		case "thinking":
			emitRole()
			out.WriteString(notrackChunk(id, model, created, map[string]any{"reasoning": "[thinking]"}, ""))
		case "delta":
			if event.Chunk != "" {
				anyDelta = true
				full += event.Chunk
				emitRole()
				out.WriteString(notrackChunk(id, model, created, map[string]any{"content": event.Chunk}, ""))
			}
		case "message":
			if event.Content != "" && !anyDelta {
				anyDelta = true
				full += event.Content
				emitRole()
				out.WriteString(notrackChunk(id, model, created, map[string]any{"content": event.Content}, ""))
			}
		}
	}
	if !roleEmitted {
		emitRole()
	}
	out.WriteString(notrackChunk(id, model, created, map[string]any{}, "stop"))
	out.WriteString("data: [DONE]\n\n")
	return out.String()
}

func notrackEstimateTokens(text string) int {
	if text == "" {
		return 1
	}
	tokens, cjk := 0, 0
	for _, r := range text {
		if (r >= 0x4e00 && r <= 0x9fff) || (r >= 0x3400 && r <= 0x4dbf) ||
			(r >= 0x3040 && r <= 0x309f) || (r >= 0x30a0 && r <= 0x30ff) ||
			(r >= 0xac00 && r <= 0xd7af) || (r >= 0xff00 && r <= 0xffef) {
			cjk++
		} else {
			tokens += utf8.RuneLen(r)
		}
	}
	count := tokens/4 + cjk + 1
	return count
}

func notrackCompletion(raw []byte, id, model string, created int64) ([]byte, error) {
	content, fallback := "", ""
	anyDelta := false
	for _, event := range notrackEvents(raw) {
		switch event.Type {
		case "delta":
			if event.Chunk != "" {
				anyDelta = true
				content += event.Chunk
			}
		case "message":
			if event.Content != "" {
				fallback = event.Content
			}
		}
	}
	if !anyDelta {
		content = fallback
	}
	clean, calls := notrackParseCalls(content)
	message := map[string]any{"role": "assistant", "content": clean}
	finish := "stop"
	if len(calls) > 0 {
		message["tool_calls"] = calls
		finish = "tool_calls"
	}
	completionTokens := notrackEstimateTokens(content)
	promptTokens := notrackEstimateTokens(id)
	completion := map[string]any{
		"id": id, "object": "chat.completion", "created": created, "model": model,
		"choices": []any{map[string]any{
			"index": 0, "message": message, "finish_reason": finish,
		}},
		"usage": map[string]any{
			"prompt_tokens": promptTokens, "completion_tokens": completionTokens,
			"total_tokens": promptTokens + completionTokens,
		},
	}
	encoded, err := json.Marshal(completion)
	if err != nil {
		return nil, fmt.Errorf("encode NoTrack completion: %w", err)
	}
	return encoded, nil
}

func notrackParseCalls(raw string) (string, []any) {
	marker := "<\u200btool_call>"
	if !strings.Contains(raw, marker) {
		return raw, nil
	}
	var calls []any
	for {
		start := strings.Index(raw, marker)
		if start < 0 {
			break
		}
		rest := raw[start+len(marker):]
		end := strings.Index(rest, "</\u200btool_call>")
		if end < 0 {
			break
		}
		body := strings.TrimSpace(rest[:end])
		var parsed struct {
			Name      string `json:"name"`
			Arguments any    `json:"arguments"`
		}
		if json.Unmarshal([]byte(body), &parsed) == nil && parsed.Name != "" {
			arguments := "{}"
			if encoded, err := json.Marshal(parsed.Arguments); err == nil {
				arguments = string(encoded)
			}
			calls = append(calls, map[string]any{
				"id": fmt.Sprintf("call_%d", len(calls)), "type": "function",
				"function": map[string]any{"name": parsed.Name, "arguments": arguments},
			})
		}
		raw = raw[:start] + rest[end+len("</\u200btool_call>"):]
	}
	return strings.TrimSpace(raw), calls
}
