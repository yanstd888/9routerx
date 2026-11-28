package executor

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"9routerx/proxy/internal/proxy"
)

type notrackMessage struct {
	Role       string `json:"role"`
	Name       string `json:"name"`
	Content    any    `json:"content"`
	ToolCallID string `json:"tool_call_id"`
	ToolCalls  []struct {
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls"`
}

func notrackBaseURL(req *Request) string {
	raw := ""
	if req.ConnData != nil {
		raw, _ = req.ConnData["baseUrl"].(string)
	}
	if strings.TrimSpace(raw) == "" && req.Config != nil {
		raw = req.Config.BaseURL
	}
	if strings.TrimSpace(raw) == "" {
		raw = notrackDefaultBaseURL
	}
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func notrackGuard(ctx context.Context, raw string) (context.Context, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ctx, proxy.UpstreamFailure(http.StatusBadRequest, "NoTrack endpoint has an invalid URL")
	}
	allowLocal := proxy.AllowPrivateProviderURLs() || proxy.IsPrivateNetworkingAllowed() || proxy.IsLoopbackAllowed(ctx)
	if !allowLocal && parsed.Scheme != "https" {
		return ctx, proxy.UpstreamFailure(http.StatusBadRequest, "NoTrack endpoint must use HTTPS")
	}
	if err := proxy.AssertDispatchURLAllowed(raw, allowLocal); err != nil {
		return ctx, proxy.UpstreamFailure(http.StatusBadRequest, "NoTrack endpoint blocked by SSRF guard")
	}
	if allowLocal {
		ctx = proxy.WithAllowLoopback(ctx)
	}
	return ctx, nil
}

func notrackCookie(raw string) string {
	cookie := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "Cookie:"))
	if cookie == "" {
		return ""
	}
	uid := notrackCookieValue(cookie, "uid")
	usr := notrackCookieValue(cookie, "si_usr_id")
	ses := notrackCookieValue(cookie, "si_ses_id")
	if uid == "" || usr == "" || ses == "" {
		return cookie
	}
	parts := []string{"uid=" + uid, "si_usr_id=" + usr, "si_ses_id=" + ses}
	if token := notrackCookieValue(cookie, "nt_session"); token != "" {
		parts = append(parts, "nt_session="+token)
	}
	return strings.Join(parts, "; ")
}

func notrackCookieValue(cookie, name string) string {
	prefix := name + "="
	for _, pair := range strings.Split(cookie, ";") {
		pair = strings.TrimSpace(pair)
		if strings.HasPrefix(pair, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(pair, prefix))
		}
	}
	return ""
}

func notrackModel(raw string) string {
	model := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(raw), "notrack-web/"), "ntw/")
	if model == "" || strings.EqualFold(model, "c") || strings.EqualFold(model, "notrack-c") {
		return "notrack-c"
	}
	return model
}

func notrackPayload(body []byte) ([]byte, error) {
	var request struct {
		Messages []notrackMessage `json:"messages"`
		Tools    []struct {
			Function struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Parameters  any    `json:"parameters"`
			} `json:"function"`
		} `json:"tools"`
		ToolChoice any      `json:"tool_choice"`
		MaxTurns   *float64 `json:"notrack_max_turns"`
		ChatID     string   `json:"notrack_chat_id"`
		Regenerate bool     `json:"notrack_regenerate"`
	}
	if json.Unmarshal(body, &request) != nil {
		return nil, fmt.Errorf("decode NoTrack request")
	}
	useTools := len(request.Tools) > 0
	if choice, ok := request.ToolChoice.(string); ok && choice == "none" {
		useTools = false
	}
	input := notrackInput(request.Messages, request.Tools, useTools)
	if input == "" {
		return nil, fmt.Errorf("empty prompt after processing messages")
	}
	payload := map[string]any{
		"user_input": input, "mode": "usual", "model": "C", "persona": "normal",
		"max_turns": 6.0, "chat_id": nil,
		"attachments": []any{}, "regenerate": request.Regenerate, "edit": false, "edit_mid": nil,
	}
	if request.MaxTurns != nil {
		payload["max_turns"] = *request.MaxTurns
	}
	if request.ChatID != "" {
		payload["chat_id"] = request.ChatID
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode NoTrack payload: %w", err)
	}
	return encoded, nil
}

func notrackContent(content any) string {
	if text, ok := content.(string); ok {
		return text
	}
	parts, ok := content.([]any)
	if !ok {
		return ""
	}
	var out []string
	for _, raw := range parts {
		switch value := raw.(type) {
		case string:
			out = append(out, value)
		case map[string]any:
			if value["type"] == "image_url" {
				out = append(out, "[image]")
			} else if text, ok := value["text"].(string); ok {
				out = append(out, text)
			}
		}
	}
	return strings.Join(out, "\n")
}

func notrackTruncate(text string) string {
	if utf8.RuneCountInString(text) <= notrackMaxInputChars {
		return text
	}
	runes := []rune(text)
	head := int(float64(notrackMaxInputChars) * 0.7)
	tail := notrackMaxInputChars - head - 20
	if tail < 0 {
		tail = 0
	}
	return string(runes[:head]) + "\n[…truncated…]\n" + string(runes[len(runes)-tail:])
}

func notrackInput(messages []notrackMessage, tools []struct {
	Function struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Parameters  any    `json:"parameters"`
	} `json:"function"`
}, useTools bool) string {
	var lines []string
	if useTools {
		lines = append(lines, "[System]\n"+notrackToolPrompt(tools))
	}
	for _, message := range messages {
		text := strings.TrimSpace(notrackContent(message.Content))
		if text == "" && len(message.ToolCalls) == 0 {
			continue
		}
		if message.Role == "tool" {
			name := message.Name
			if name == "" {
				name = message.ToolCallID
			}
			lines = append(lines, "[Tool Result: "+name+"]\n"+text)
			continue
		}
		if message.Role == "assistant" && len(message.ToolCalls) > 0 {
			block := "[Assistant]"
			for _, call := range message.ToolCalls {
				arguments := call.Function.Arguments
				if arguments == "" {
					arguments = "{}"
				}
				block += "\n<\u200btool_call>{\"name\":\"" + call.Function.Name + "\",\"arguments\":" + arguments + "}</\u200btool_call>"
			}
			if text != "" {
				block += "\n" + text
			}
			lines = append(lines, block)
			continue
		}
		role := message.Role
		if role == "" {
			role = "user"
		}
		lines = append(lines, "["+strings.ToUpper(role[:1])+role[1:]+"]\n"+text)
	}
	joined := strings.Join(lines, "\n\n")
	if len(lines) == 1 && messages[0].Role == "user" && !useTools {
		return notrackTruncate(strings.TrimSpace(notrackContent(messages[0].Content)))
	}
	if len(lines) == 2 && messages[0].Role == "system" && messages[1].Role == "user" && !useTools {
		system := strings.TrimSpace(notrackContent(messages[0].Content))
		user := strings.TrimSpace(notrackContent(messages[1].Content))
		if system != "" {
			joined = "(请遵循以下指引回答：" + system + ")\n\n" + user
		} else {
			joined = user
		}
	}
	return notrackTruncate(joined)
}

func notrackToolPrompt(tools []struct {
	Function struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Parameters  any    `json:"parameters"`
	} `json:"function"`
}) string {
	lines := []string{
		"The client provides tools beyond your built-in ones. They are invoked via a plain-text protocol.",
		"To call a tool, emit <\u200btool_call>{\"name\":\"tool_name\",\"arguments\":{}}</\u200btool_call>.",
		"Tool results arrive as [Tool Result: tool_name]. Available tools:",
	}
	for _, tool := range tools {
		name := tool.Function.Name
		if name == "" {
			continue
		}
		line := "- " + name
		if tool.Function.Description != "" {
			line += ": " + tool.Function.Description
		}
		if encoded, err := json.Marshal(tool.Function.Parameters); err == nil {
			line += "\n  parameters: " + string(encoded)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
