package executor

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"9routerx/proxy/internal/proxy"
)

const syntxUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

func syntxBaseURL(req *Request) string {
	raw := req.Config.BaseURL
	if req.ConnData != nil {
		if custom, ok := req.ConnData["baseUrl"].(string); ok && strings.TrimSpace(custom) != "" {
			raw = custom
		}
	}
	if strings.TrimSpace(raw) == "" {
		return syntxDefaultBaseURL
	}
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func syntxGuard(ctx context.Context, raw string) (context.Context, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ctx, proxy.UpstreamFailure(http.StatusBadRequest, "SYNTX endpoint has an invalid URL")
	}
	allowLocal := proxy.AllowPrivateProviderURLs() || proxy.IsPrivateNetworkingAllowed() || proxy.IsLoopbackAllowed(ctx)
	if !allowLocal && parsed.Scheme != "https" {
		return ctx, proxy.UpstreamFailure(http.StatusBadRequest, "SYNTX endpoint must use HTTPS")
	}
	if err := proxy.AssertDispatchURLAllowed(raw, allowLocal); err != nil {
		return ctx, proxy.UpstreamFailure(http.StatusBadRequest, "SYNTX endpoint blocked by SSRF guard")
	}
	if allowLocal {
		ctx = proxy.WithAllowLoopback(ctx)
	}
	return ctx, nil
}

func syntxAuthHeaders(token string) map[string]string {
	return map[string]string{
		"accept": "application/json, text/plain, */*", "accept-language": "en",
		"authorization": "Bearer " + token,
		"origin": "https://syntx.ai", "referer": "https://syntx.ai/", "user-agent": syntxUserAgent,
	}
}

func syntxModel(requested string, body []byte) (syntxModelData, bool) {
	raw := strings.TrimSpace(requested)
	if raw == "" {
		var parsed struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(body, &parsed) == nil {
			raw = strings.TrimSpace(parsed.Model)
		}
	}
	raw = strings.TrimPrefix(raw, "syntx/")
	raw = strings.TrimPrefix(raw, "stx/")
	for _, model := range syntxModels {
		if strings.EqualFold(model.ID, raw) || strings.EqualFold(model.Name, raw) {
			return model, true
		}
	}
	if idx := strings.LastIndexByte(raw, '/'); idx >= 0 {
		raw = raw[idx+1:]
		for _, model := range syntxModels {
			if strings.EqualFold(model.ID, raw) || strings.EqualFold(model.Name, raw) {
				return model, true
			}
		}
	}
	return syntxModelData{}, false
}

func syntxPrompt(body []byte, modelThinking bool) (string, string, bool) {
	var request struct {
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
		ReasoningEffort string `json:"reasoning_effort"`
	}
	if json.Unmarshal(body, &request) != nil {
		return "", "", modelThinking
	}
	lines := make([]string, 0, len(request.Messages))
	title := "9RouterX"
	for _, message := range request.Messages {
		text := syntxContent(message.Content)
		if text == "" {
			continue
		}
		role := message.Role
		if role == "" {
			role = "user"
		}
		lines = append(lines, "["+strings.ToUpper(role[:1])+role[1:]+"]\n"+text)
		if role == "user" && title == "9RouterX" {
			title = text
			if len(title) > 80 {
				title = title[:80]
			}
		}
	}
	text := strings.Join(lines, "\n\n")
	thinking := modelThinking && strings.EqualFold(request.ReasoningEffort, "none") == false
	return text, title, thinking
}

func syntxContent(content any) string {
	if text, ok := content.(string); ok {
		return text
	}
	parts, ok := content.([]any)
	if !ok {
		return ""
	}
	var out []string
	for _, raw := range parts {
		if part, ok := raw.(map[string]any); ok {
			if text, ok := part["text"].(string); ok {
				out = append(out, text)
			}
		}
	}
	return strings.Join(out, "\n")
}

func syntxCreateChat(ctx context.Context, client *http.Client, base, token, title string) (string, error) {
	payload, _ := json.Marshal(map[string]any{"title": title, "scope": "text"})
	headers := syntxAuthHeaders(token)
	headers["content-type"] = "application/json"
	response, err := proxy.DoRequest(ctx, client, http.MethodPost, base+"/api/v1/chats", headers, payload)
	if err != nil {
		return "", fmt.Errorf("SYNTX create chat failed")
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("SYNTX create chat failed")
	}
	var parsed struct {
		ChatUUID string `json:"chat_uuid"`
		ID       string `json:"id"`
	}
	if json.Unmarshal(raw, &parsed) != nil {
		return "", fmt.Errorf("SYNTX create chat response invalid")
	}
	if parsed.ChatUUID != "" {
		return parsed.ChatUUID, nil
	}
	return parsed.ID, nil
}

func syntxGenerate(ctx context.Context, client *http.Client, base, token, chatID string, model syntxModelData, text string, thinking bool) (string, error) {
	payload, err := json.Marshal(map[string]any{
		"chat_uuid": chatID, "text": text, "model": model.ID,
		"thinking": thinking, "plan": false, "deep_research": false, "tools": []any{},
	})
	if err != nil {
		return "", fmt.Errorf("encode SYNTX generate request")
	}
	target := base + "/api/v1/llm/generate?ai_name=" + url.QueryEscape(model.AIName)
	headers := syntxAuthHeaders(token)
	headers["content-type"] = "application/json"
	response, err := proxy.DoRequest(ctx, client, http.MethodPost, target, headers, payload)
	if err != nil {
		return "", fmt.Errorf("SYNTX generate failed")
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("SYNTX generate failed")
	}
	var parsed struct {
		StreamURL string `json:"stream_url"`
	}
	if json.Unmarshal(raw, &parsed) != nil || parsed.StreamURL == "" {
		return "", fmt.Errorf("SYNTX generate returned no stream url")
	}
	return parsed.StreamURL, nil
}

func syntxResolveStreamURL(raw, base string) (string, error) {
	stream, err := url.Parse(raw)
	root, rootErr := url.Parse(base)
	if err != nil || rootErr != nil || (stream.Scheme != "https" && stream.Scheme != "http") || stream.Host == "" {
		return "", proxy.UpstreamFailure(http.StatusBadGateway, "SYNTX generate returned an invalid stream URL")
	}
	if stream.Host != "sse.syntx.ai" && stream.Host != root.Host {
		return "", proxy.UpstreamFailure(http.StatusBadGateway, "SYNTX stream host is not permitted")
	}
	return raw, nil
}

func syntxStream(raw []byte) (string, map[string]any) {
	var text strings.Builder
	var usage map[string]any
	for _, block := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n\n") {
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "" || payload == "[DONE]" || !strings.HasPrefix(payload, "{") {
				continue
			}
			var event struct {
				Type          string  `json:"type"`
				Content       string  `json:"content"`
				TokensInput   float64 `json:"tokens_input"`
				TokensOutput  float64 `json:"tokens_output"`
			}
			if json.Unmarshal([]byte(payload), &event) != nil {
				continue
			}
			switch event.Type {
			case "content":
				text.WriteString(event.Content)
			case "usage_final":
				usage = map[string]any{
					"prompt_tokens": event.TokensInput, "completion_tokens": event.TokensOutput,
					"total_tokens": event.TokensInput + event.TokensOutput,
				}
			}
		}
	}
	return text.String(), usage
}

func syntxChunk(id, model, content string, created int64, finish string, usage map[string]any) string {
	choice := map[string]any{"index": 0, "delta": map[string]any{"content": content}, "finish_reason": nil}
	if finish != "" {
		choice["delta"] = map[string]any{}
		choice["finish_reason"] = finish
	}
	chunk := map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": model, "choices": []any{choice}}
	if usage != nil {
		chunk["usage"] = usage
	}
	encoded, err := json.Marshal(chunk)
	if err != nil {
		return ""
	}
	return "data: " + string(encoded) + "\n\n"
}

func syntxFrames(text, id, model string, created int64, usage map[string]any) string {
	var out strings.Builder
	if text != "" {
		out.WriteString(syntxChunk(id, model, text, created, "", nil))
	}
	out.WriteString(syntxChunk(id, model, "", created, "stop", usage))
	out.WriteString("data: [DONE]\n\n")
	return out.String()
}

func syntxCompletion(text, id, model string, created int64, usage map[string]any) []byte {
	completion := map[string]any{
		"id": id, "object": "chat.completion", "created": created, "model": model,
		"choices": []any{map[string]any{
			"index": 0, "message": map[string]any{"role": "assistant", "content": text}, "finish_reason": "stop",
		}},
	}
	if usage != nil {
		completion["usage"] = usage
	}
	encoded, _ := json.Marshal(completion)
	return encoded
}
