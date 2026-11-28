package executor

import (
	json "encoding/json/v2"
	"fmt"
	"strings"
)

type clovaFrame struct {
	event string
	data  map[string]any
}

type clovaResult struct {
	frames    string
	content   strings.Builder
	reasoning strings.Builder
	toolCalls []any
	usage     map[string]any
	finish    string
	chunks    int
}

func clovaParseFrames(raw []byte) []clovaFrame {
	var out []clovaFrame
	for _, block := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n\n") {
		frame := clovaFrame{}
		var dataLines []string
		for _, line := range strings.Split(block, "\n") {
			trimmed := strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(trimmed, "event:"):
				frame.event = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
			case strings.HasPrefix(trimmed, "data:"):
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
			}
		}
		if len(dataLines) == 0 {
			continue
		}
		if json.Unmarshal([]byte(strings.Join(dataLines, "\n")), &frame.data) != nil || frame.data == nil {
			continue
		}
		out = append(out, frame)
	}
	return out
}

func clovaChunk(id, model string, created int64, delta map[string]any, finish string, usage map[string]any) string {
	choice := map[string]any{"index": 0, "delta": delta, "finish_reason": nil}
	if finish != "" {
		choice["finish_reason"] = finish
	}
	chunk := map[string]any{
		"id": id, "object": "chat.completion.chunk", "created": created,
		"model": model, "choices": []any{choice},
	}
	if usage != nil {
		chunk["usage"] = usage
	}
	encoded, err := json.Marshal(chunk)
	if err != nil {
		return ""
	}
	return "data: " + string(encoded) + "\n\n"
}

func clovaDelta(result *clovaResult, id, model string, created int64, field, text string) {
	delta := map[string]any{}
	if result.chunks == 0 {
		delta["role"] = "assistant"
	}
	delta[field] = text
	result.frames += clovaChunk(id, model, created, delta, "", nil)
	result.chunks++
}

func clovaUsage(data map[string]any) map[string]any {
	usage, ok := data["usage"].(map[string]any)
	if !ok {
		return nil
	}
	prompt, _ := clovaNumber(usage["promptTokens"], 0, 1<<30)
	completion, _ := clovaNumber(usage["completionTokens"], 0, 1<<30)
	total, ok := clovaNumber(usage["totalTokens"], 0, 1<<30)
	if !ok {
		total = prompt + completion
	}
	return map[string]any{
		"prompt_tokens": prompt, "completion_tokens": completion, "total_tokens": total,
	}
}

func clovaConvert(raw []byte, id, model string, created int64) clovaResult {
	result := clovaResult{}
	toolStarted := false
	for _, frame := range clovaParseFrames(raw) {
		data := frame.data
		if envelope, ok := data["result"].(map[string]any); ok {
			data = envelope
		}
		if event := frame.event; event == "token" || event == "" {
			message, _ := data["message"].(map[string]any)
			if message == nil {
				message = data
			}
			if thinking, ok := message["thinkingContent"].(string); ok && thinking != "" {
				result.reasoning.WriteString(thinking)
				clovaDelta(&result, id, model, created, "reasoning_content", thinking)
			}
			if content, ok := message["content"].(string); ok && content != "" {
				result.content.WriteString(content)
				clovaDelta(&result, id, model, created, "content", content)
			}
			if calls, ok := message["toolCalls"].([]any); ok {
				for _, rawCall := range calls {
					call, _ := rawCall.(map[string]any)
					fn, _ := call["function"].(map[string]any)
					name, _ := fn["name"].(string)
					callID, _ := call["id"].(string)
					if !toolStarted && (name != "" || callID != "") {
						delta := map[string]any{"tool_calls": []any{map[string]any{
							"index": 0, "id": callID, "type": "function",
							"function": map[string]any{"name": name, "arguments": ""},
						}}}
						if result.chunks == 0 {
							delta["role"] = "assistant"
						}
						result.frames += clovaChunk(id, model, created, delta, "", nil)
						result.chunks++
						toolStarted = true
						result.toolCalls = append(result.toolCalls, map[string]any{
							"id": callID, "type": "function",
							"function": map[string]any{"name": name, "arguments": ""},
						})
					} else if fragment, ok := fn["partialJson"].(string); ok && fragment != "" {
						if len(result.toolCalls) > 0 {
							last, _ := result.toolCalls[len(result.toolCalls)-1].(map[string]any)
							if fnMap, ok := last["function"].(map[string]any); ok {
								arguments, _ := fnMap["arguments"].(string)
								fnMap["arguments"] = arguments + fragment
							}
						}
						delta := map[string]any{"tool_calls": []any{map[string]any{
							"index": 0, "function": map[string]any{"arguments": fragment},
						}}}
						result.frames += clovaChunk(id, model, created, delta, "", nil)
						result.chunks++
					}
				}
			}
			continue
		}
		if frame.event != "result" && frame.event != "stop" {
			continue
		}
		result.usage = clovaUsage(data)
		result.finish = "stop"
		if reason, ok := data["finishReason"].(string); ok {
			switch reason {
			case "length":
				result.finish = "length"
			case "tool_calls", "toolCalls":
				result.finish = "tool_calls"
			}
		}
		if toolStarted {
			result.finish = "tool_calls"
		}
		if result.chunks == 0 {
			if message, ok := data["message"].(map[string]any); ok {
				if content, ok := message["content"].(string); ok && content != "" {
					result.content.WriteString(content)
					clovaDelta(&result, id, model, created, "content", content)
				}
			}
		}
		result.frames += clovaChunk(id, model, created, map[string]any{}, result.finish, result.usage)
		result.frames += "data: [DONE]\n\n"
	}
	if result.frames == "" {
		result.frames = clovaChunk(id, model, created, map[string]any{}, "stop", nil) + "data: [DONE]\n\n"
		result.finish = "stop"
	}
	return result
}

func clovaCompletion(raw []byte, id, model string, created int64) ([]byte, error) {
	result := clovaConvert(raw, id, model, created)
	message := map[string]any{"role": "assistant", "content": result.content.String()}
	if result.reasoning.Len() > 0 {
		message["reasoning_content"] = result.reasoning.String()
	}
	if len(result.toolCalls) > 0 {
		message["tool_calls"] = result.toolCalls
	}
	finish := result.finish
	if finish == "" {
		finish = "stop"
	}
	completion := map[string]any{
		"id": id, "object": "chat.completion", "created": created, "model": model,
		"choices": []any{map[string]any{
			"index": 0, "message": message, "finish_reason": finish,
		}},
	}
	if result.usage != nil {
		completion["usage"] = result.usage
	}
	encoded, err := json.Marshal(completion)
	if err != nil {
		return nil, fmt.Errorf("encode CLOVA completion: %w", err)
	}
	return encoded, nil
}
