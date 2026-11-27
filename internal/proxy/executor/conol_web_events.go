package executor

import (
	json "encoding/json/v2"
	"strings"
)

func conolChunk(id, model, content string, created int64, finish string) string {
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

func conolFrames(text, id, model string, created int64) string {
	var out strings.Builder
	if text != "" {
		out.WriteString(conolChunk(id, model, text, created, ""))
	}
	out.WriteString(conolChunk(id, model, "", created, "stop"))
	out.WriteString("data: [DONE]\n\n")
	return out.String()
}

func conolCompletion(text, id, model string, created int64) []byte {
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
