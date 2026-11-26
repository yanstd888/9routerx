package executor

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"9routerx/proxy/internal/proxy"
)

func oneminaiGuard(ctx context.Context, raw string) (context.Context, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ctx, proxy.UpstreamFailure(http.StatusBadRequest, "1min.ai endpoint has an invalid URL")
	}
	allowLocal := proxy.AllowPrivateProviderURLs() || proxy.IsPrivateNetworkingAllowed() || proxy.IsLoopbackAllowed(ctx)
	if !allowLocal && parsed.Scheme != "https" {
		return ctx, proxy.UpstreamFailure(http.StatusBadRequest, "1min.ai endpoint must use HTTPS")
	}
	if err := proxy.AssertDispatchURLAllowed(raw, allowLocal); err != nil {
		return ctx, proxy.UpstreamFailure(http.StatusBadRequest, "1min.ai endpoint blocked by SSRF guard")
	}
	if allowLocal {
		ctx = proxy.WithAllowLoopback(ctx)
	}
	return ctx, nil
}

func oneminaiBodyModel(body []byte) string {
	var parsed struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &parsed) != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Model)
}

func oneminaiContent(content any) string {
	if text, ok := content.(string); ok {
		return text
	}
	parts, ok := content.([]any)
	if !ok {
		return ""
	}
	var out []string
	for _, raw := range parts {
		part, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if part["type"] == "text" {
			if text, ok := part["text"].(string); ok {
				out = append(out, text)
			}
		}
	}
	return strings.Join(out, "\n")
}

func oneminaiPrompt(body []byte) string {
	var request struct {
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	if json.Unmarshal(body, &request) != nil || len(request.Messages) == 0 {
		return ""
	}
	if len(request.Messages) == 1 && request.Messages[0].Role == "user" {
		return oneminaiContent(request.Messages[0].Content)
	}
	labels := map[string]string{"system": "System", "developer": "System", "user": "User", "assistant": "Assistant"}
	lines := make([]string, 0, len(request.Messages))
	for _, message := range request.Messages {
		text := oneminaiContent(message.Content)
		if text == "" {
			continue
		}
		label := labels[strings.ToLower(message.Role)]
		if label == "" {
			label = message.Role
		}
		lines = append(lines, label+": "+text)
	}
	return strings.Join(lines, "\n\n")
}

func oneminaiError(detail string, status int) string {
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(detail), &parsed) == nil && parsed.Error.Message != "" {
		return parsed.Error.Message
	}
	detail = strings.TrimSpace(detail)
	if len(detail) > 200 {
		detail = detail[:200]
	}
	if detail == "" {
		return fmt.Sprintf("1min.ai failed with status %d", status)
	}
	return detail
}

func oneminaiChunk(id, model string, created int64, delta map[string]any, finish string) string {
	choice := map[string]any{"index": 0, "delta": delta, "finish_reason": nil}
	if finish != "" {
		choice["finish_reason"] = finish
	}
	chunk := map[string]any{
		"id": id, "object": "chat.completion.chunk", "created": created,
		"model": model, "choices": []any{choice},
	}
	encoded, err := json.Marshal(chunk)
	if err != nil {
		return ""
	}
	return "data: " + string(encoded) + "\n\n"
}

func oneminaiFrames(raw []byte, id, model string, created int64) string {
	var out strings.Builder
	chunks := 0
	for _, block := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n\n") {
		event, data := "", ""
		for _, line := range strings.Split(block, "\n") {
			trimmed := strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(trimmed, "event:"):
				event = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
			case strings.HasPrefix(trimmed, "data:"):
				data = strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
			}
		}
		if event == "content" {
			var parsed struct {
				Content string `json:"content"`
			}
			if json.Unmarshal([]byte(data), &parsed) == nil && parsed.Content != "" {
				delta := map[string]any{"content": parsed.Content}
				if chunks == 0 {
					delta["role"] = "assistant"
				}
				out.WriteString(oneminaiChunk(id, model, created, delta, ""))
				chunks++
			}
		}
	}
	out.WriteString(oneminaiChunk(id, model, created, map[string]any{}, "stop"))
	out.WriteString("data: [DONE]\n\n")
	return out.String()
}

func oneminaiCompletion(raw []byte, id, model string, created int64) ([]byte, error) {
	var parsed struct {
		AIRecord struct {
			AIRecordDetail struct {
				ResultObject []any `json:"resultObject"`
			} `json:"aiRecordDetail"`
		} `json:"aiRecord"`
	}
	if json.Unmarshal(raw, &parsed) != nil {
		return nil, fmt.Errorf("decode 1min.ai response")
	}
	var content strings.Builder
	for _, part := range parsed.AIRecord.AIRecordDetail.ResultObject {
		if text, ok := part.(string); ok {
			content.WriteString(text)
		}
	}
	completion := map[string]any{
		"id": id, "object": "chat.completion", "created": created, "model": model,
		"choices": []any{map[string]any{
			"index": 0, "message": map[string]any{"role": "assistant", "content": content.String()},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
	}
	encoded, err := json.Marshal(completion)
	if err != nil {
		return nil, fmt.Errorf("encode 1min.ai completion: %w", err)
	}
	return encoded, nil
}
