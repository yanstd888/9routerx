package executor

import (
	json "encoding/json/v2"
	"strings"
)

type adaptaEvent struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Delta string `json:"delta"`
}

func adaptaStream(raw []byte) (string, bool) {
	var text strings.Builder
	streamErr := false
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || !strings.HasPrefix(payload, "{") {
			continue
		}
		var event adaptaEvent
		if json.Unmarshal([]byte(payload), &event) != nil {
			continue
		}
		switch event.Type {
		case "text-delta":
			if event.ID != "quick-response" && event.Delta != "" {
				text.WriteString(event.Delta)
			}
		case "error":
			streamErr = true
		}
	}
	return text.String(), streamErr
}

func adaptaChunk(id, model, content string, created int64, finish string) string {
	choice := map[string]any{"index": 0, "delta": map[string]any{"content": content}, "finish_reason": nil}
	if finish != "" {
		choice["delta"] = map[string]any{}
		choice["finish_reason"] = finish
	}
	chunk := map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": model, "choices": []any{choice}}
	encoded, err := json.Marshal(chunk)
	if err != nil {
		return ""
	}
	return "data: " + string(encoded) + "\n\n"
}

func adaptaFrames(text, id, model string, created int64) string {
	var out strings.Builder
	if text != "" {
		out.WriteString(adaptaChunk(id, model, text, created, ""))
	}
	out.WriteString(adaptaChunk(id, model, "", created, "stop"))
	out.WriteString("data: [DONE]\n\n")
	return out.String()
}

func adaptaCompletion(text, id, model string, created int64) []byte {
	completion := map[string]any{
		"id": id, "object": "chat.completion", "created": created, "model": model,
		"choices": []any{map[string]any{
			"index": 0, "message": map[string]any{"role": "assistant", "content": text}, "finish_reason": "stop",
		}},
		"usage": map[string]any{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
	}
	encoded, _ := json.Marshal(completion)
	return encoded
}
