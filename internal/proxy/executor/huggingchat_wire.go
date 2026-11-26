package executor

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"9routerx/proxy/internal/proxy"
)

type huggingchatFormBody struct {
	data        []byte
	contentType string
}

func huggingchatCookie(raw string) string {
	cookie := strings.TrimSpace(raw)
	cookie = strings.TrimSpace(strings.TrimPrefix(cookie, "Cookie:"))
	if cookie == "" {
		return ""
	}
	if strings.Contains(cookie, "hf-chat=") {
		return cookie
	}
	if !strings.Contains(cookie, "=") {
		return "hf-chat=" + cookie
	}
	return cookie
}

func huggingchatBaseURL(req *Request) string {
	raw := ""
	if req.ConnData != nil {
		raw, _ = req.ConnData["baseUrl"].(string)
	}
	if strings.TrimSpace(raw) == "" && req.Config != nil {
		raw = req.Config.BaseURL
	}
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	base = strings.TrimSuffix(base, "/conversation")
	for _, suffix := range []string{"/chat/completions"} {
		base = strings.TrimSuffix(base, suffix)
	}
	if base == "" {
		return huggingchatDefaultBaseURL
	}
	return base
}

func huggingchatGuard(ctx context.Context, raw string) (context.Context, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ctx, proxy.UpstreamFailure(http.StatusBadRequest, "HuggingChat endpoint has an invalid URL")
	}
	allowLocal := proxy.AllowPrivateProviderURLs() || proxy.IsPrivateNetworkingAllowed() || proxy.IsLoopbackAllowed(ctx)
	if !allowLocal && parsed.Scheme != "https" {
		return ctx, proxy.UpstreamFailure(http.StatusBadRequest, "HuggingChat endpoint must use HTTPS")
	}
	if err := proxy.AssertDispatchURLAllowed(raw, allowLocal); err != nil {
		return ctx, proxy.UpstreamFailure(http.StatusBadRequest, "HuggingChat endpoint blocked by SSRF guard")
	}
	if allowLocal {
		ctx = proxy.WithAllowLoopback(ctx)
	}
	return ctx, nil
}

func huggingchatMergeCookies(cookie string, setCookies []string) string {
	parts := map[string]string{}
	for _, part := range strings.Split(cookie, ";") {
		pair := strings.TrimSpace(part)
		if idx := strings.IndexByte(pair, '='); idx > 0 {
			parts[pair[:idx]] = pair
		}
	}
	for _, header := range setCookies {
		pair := strings.TrimSpace(strings.SplitN(header, ";", 2)[0])
		if idx := strings.IndexByte(pair, '='); idx > 0 {
			parts[pair[:idx]] = pair
		}
	}
	keys := make([]string, 0, len(parts))
	for key := range parts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values := make([]string, 0, len(keys))
	for _, key := range keys {
		values = append(values, parts[key])
	}
	return strings.Join(values, "; ")
}

func huggingchatBodyModel(body []byte) string {
	var parsed struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &parsed) != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Model)
}

func huggingchatText(content any) string {
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
		if text, ok := part["text"].(string); ok {
			out = append(out, text)
		}
	}
	return strings.Join(out, "\n")
}

func huggingchatPrompt(body []byte) (string, string) {
	var request struct {
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	if json.Unmarshal(body, &request) != nil {
		return "", ""
	}
	var system []string
	type turn struct{ role, text string }
	var turns []turn
	for _, message := range request.Messages {
		text := huggingchatText(message.Content)
		if text == "" {
			continue
		}
		switch message.Role {
		case "system", "developer":
			system = append(system, text)
		case "user", "assistant":
			turns = append(turns, turn{message.Role, text})
		}
	}
	if len(turns) == 1 && turns[0].role == "user" {
		return turns[0].text, strings.Join(system, "\n\n")
	}
	lines := make([]string, 0, len(turns)+1)
	for _, item := range turns {
		label := "User"
		if item.role == "assistant" {
			label = "Assistant"
		}
		lines = append(lines, label+": "+item.text)
	}
	lines = append(lines, "Assistant:")
	return strings.Join(lines, "\n\n"), strings.Join(system, "\n\n")
}

func huggingchatForm(prompt, system, parent string) huggingchatFormBody {
	boundary := fmt.Sprintf("huggingchat-%x", time.Now().UnixNano())
	payload := map[string]any{
		"inputs": prompt, "is_retry": false, "is_continue": false,
		"generationId": boundary, "selectedMcpServerNames": []string{},
		"selectedMcpServers": []string{}, "timezone": "UTC", "id": parent,
	}
	if system != "" {
		payload["preprompt"] = system
	}
	encoded, _ := json.Marshal(payload)
	var out bytes.Buffer
	fmt.Fprintf(&out, "--%s\r\nContent-Disposition: form-data; name=\"data\"\r\n\r\n%s\r\n--%s--\r\n", boundary, encoded, boundary)
	return huggingchatFormBody{data: out.Bytes(), contentType: "multipart/form-data; boundary=" + boundary}
}

func huggingchatConvert(raw []byte, id, model string) (string, []byte) {
	var frames strings.Builder
	var content, reasoning strings.Builder
	emit := func(field, text string) {
		delta := map[string]any{"role": "assistant"}
		if content.Len()+reasoning.Len() > 0 {
			delta = map[string]any{}
		}
		delta[field] = text
		chunk := map[string]any{
			"id": id, "object": "chat.completion.chunk", "created": time.Now().Unix(),
			"model": model,
			"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}},
		}
		if encoded, err := json.Marshal(chunk); err == nil {
			frames.WriteString("data: ")
			frames.Write(encoded)
			frames.WriteString("\n\n")
		}
	}
	sawToken := false
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event map[string]any
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		kind, _ := event["type"].(string)
		switch kind {
		case "stream":
			if token, ok := event["token"].(string); ok {
				token = strings.ReplaceAll(token, "\x00", "")
				if token != "" {
					sawToken = true
					content.WriteString(token)
					emit("content", token)
				}
			}
		case "reasoning":
			if token, ok := event["token"].(string); ok {
				token = strings.ReplaceAll(token, "\x00", "")
				if token != "" {
					reasoning.WriteString(token)
					emit("reasoning_content", token)
				}
			}
		case "finalAnswer":
			if text, ok := event["text"].(string); ok && text != "" && !sawToken {
				content.WriteString(text)
				emit("content", text)
			}
		case "status":
			if status, _ := event["status"].(string); status == "finished" || status == "error" {
				sawToken = true
			}
		}
	}
	finish := map[string]any{
		"id": id, "object": "chat.completion.chunk", "created": time.Now().Unix(),
		"model": model,
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}},
	}
	if encoded, err := json.Marshal(finish); err == nil {
		frames.WriteString("data: ")
		frames.Write(encoded)
		frames.WriteString("\n\n")
	}
	frames.WriteString("data: [DONE]\n\n")
	tokens := content.Len() / 4
	if tokens < 1 {
		tokens = 1
	}
	message := map[string]any{"role": "assistant", "content": content.String()}
	if reasoning.Len() > 0 {
		message["reasoning_content"] = reasoning.String()
	}
	completion, _ := json.Marshal(map[string]any{
		"id": id, "object": "chat.completion", "created": time.Now().Unix(), "model": model,
		"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": "stop"}},
		"usage": map[string]any{
			"prompt_tokens": tokens, "completion_tokens": tokens, "total_tokens": tokens * 2,
		},
	})
	return frames.String(), completion
}
