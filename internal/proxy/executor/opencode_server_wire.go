package executor

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"net/url"
	"strings"

	"9routerx/proxy/internal/netguard"
	"9routerx/proxy/internal/translator"
)

// OpenCode CLI Server message parts. The server accepts bare text parts and
// file parts; every turn is flattened into one labelled text part so the model
// sees [system]/[user]/[assistant]/[tool] boundaries without agent tools.
const (
	opencodeServerCallOpen  = "[tool_call]"
	opencodeServerCallClose = "[/tool_call]"
	opencodeServerToolOpen  = "[tool]"
	opencodeServerToolClose = "[/tool]"
)

type opencodeServerToolCall struct {
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Arguments jsontext.Value `json:"arguments,omitempty"`
}

type opencodeServerChatMessage struct {
	Role       string         `json:"role"`
	Content    jsontext.Value `json:"content"`
	ToolCallID string         `json:"tool_call_id"`
	ToolCalls  []struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls"`
}

type opencodeServerChatRequest struct {
	Model    string                      `json:"model"`
	Messages []opencodeServerChatMessage `json:"messages"`
}

type opencodeServerInput struct {
	model  string
	system string
	title  string
	parts  []map[string]any
}

type opencodeServerTokenCounts struct {
	Input     int `json:"input"`
	Output    int `json:"output"`
	Reasoning int `json:"reasoning"`
	Cache     struct {
		Read  int `json:"read"`
		Write int `json:"write"`
	} `json:"cache"`
}

func (t opencodeServerTokenCounts) usage() *translator.OpenAIUsage {
	if t.Input == 0 && t.Output == 0 && t.Reasoning == 0 && t.Cache.Read == 0 && t.Cache.Write == 0 {
		return nil
	}
	return &translator.OpenAIUsage{
		PromptTokens:             t.Input + t.Cache.Read,
		CompletionTokens:         t.Output + t.Reasoning,
		CachedTokens:             t.Cache.Read,
		CacheCreationInputTokens: t.Cache.Write,
		PromptTokensDetails:      &translator.PromptTokensDetails{CachedTokens: t.Cache.Read},
		CompletionTokensDetails:  &translator.CompletionTokensDetails{ReasoningTokens: t.Reasoning},
		PromptCacheIncluded:      true,
	}
}

func opencodeServerInputFrom(ctx context.Context, req *Request) (opencodeServerInput, context.Context, error) {
	body := req.Body
	var err error
	if req.ClaudeClient {
		body, err = translator.TranslateClaudeToOpenAI(body)
		if err != nil {
			return opencodeServerInput{}, ctx, fmt.Errorf("translate Claude request: %w", err)
		}
	}
	if translator.IsResponsesClient(ctx) {
		converted, err := translator.ResponsesToChatRequest(body)
		if err != nil {
			return opencodeServerInput{}, ctx, fmt.Errorf("translate Responses request: %w", err)
		}
		body = converted.Body
		ctx = translator.WithResponsesBridge(ctx)
		if len(converted.CustomToolNames) > 0 {
			ctx = translator.WithCustomToolNames(ctx, converted.CustomToolNames)
		}
	}
	var chat opencodeServerChatRequest
	if err := json.Unmarshal(body, &chat); err != nil {
		return opencodeServerInput{}, ctx, fmt.Errorf("parse chat request: %w", err)
	}
	var system, turns strings.Builder
	var files []map[string]any
	for _, msg := range chat.Messages {
		text := opencodeServerContentText(msg.Content, &files)
		switch msg.Role {
		case "system":
			if system.Len() > 0 {
				system.WriteString("\n")
			}
			system.WriteString(text)
		case "assistant":
			for _, call := range msg.ToolCalls {
				if envelope := opencodeServerEncodeCall(call.ID, call.Function.Name, call.Function.Arguments, false); envelope != "" {
					text += "\n" + envelope
				}
			}
		case "tool":
			if envelope := opencodeServerEncodeCall(msg.ToolCallID, "", string(msg.Content), true); envelope != "" {
				text += "\n" + envelope
			}
		}
		if text == "" {
			continue
		}
		turns.WriteString("[" + msg.Role + "]\n" + text + "\n")
	}
	systemText := strings.TrimSpace(system.String())
	prompt := systemText
	if prompt != "" {
		prompt = "[system]\n" + prompt + "\n"
	}
	prompt += strings.TrimSpace(turns.String())
	parts := make([]map[string]any, 0, len(files)+1)
	if prompt != "" {
		parts = append(parts, map[string]any{"type": "text", "text": prompt})
	}
	parts = append(parts, files...)
	return opencodeServerInput{model: chat.Model, system: systemText, title: opencodeServerTitle(chat.Messages), parts: parts}, ctx, nil
}
func opencodeServerContentText(content jsontext.Value, files *[]map[string]any) string {
	raw := strings.TrimSpace(string(content))
	if raw == "" || raw == "null" {
		return ""
	}
	if strings.HasPrefix(raw, "\"") {
		var text string
		if json.Unmarshal(content, &text) == nil {
			return text
		}
		return ""
	}
	var blocks []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL struct {
			URL string `json:"url"`
		} `json:"image_url"`
		File struct {
			URL      string `json:"url"`
			FileData string `json:"file_data"`
			MimeType string `json:"media_type"`
		} `json:"file"`
	}
	if json.Unmarshal(content, &blocks) != nil {
		return ""
	}
	var text strings.Builder
	for _, block := range blocks {
		switch {
		case block.Text != "":
			text.WriteString(block.Text)
		case block.ImageURL.URL != "":
			opencodeServerAppendFile(files, block.ImageURL.URL, "")
		case block.File.FileData != "":
			opencodeServerAppendFile(files, block.File.FileData, block.File.MimeType)
		case block.File.URL != "":
			opencodeServerAppendFile(files, block.File.URL, block.File.MimeType)
		}
	}
	return text.String()
}
func opencodeServerAppendFile(files *[]map[string]any, raw, mime string) {
	if !opencodeServerFileAllowed(raw) {
		return
	}
	part := map[string]any{"type": "file", "url": raw}
	if mime != "" {
		part["mime"] = mime
	}
	*files = append(*files, part)
}

// opencodeServerFileAllowed admits only inline data URLs and public HTTPS
// URLs. Internal names, literal addresses and every reserved IP range are
// dropped before a part can reach the local server.
func opencodeServerFileAllowed(raw string) bool {
	if strings.HasPrefix(raw, "data:") {
		return len(raw) <= 16<<20
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Host == "" {
		return false
	}
	host := u.Hostname()
	lower := strings.ToLower(host)
	if host == "localhost" || strings.HasSuffix(lower, ".local") || strings.HasSuffix(lower, ".internal") || !strings.Contains(host, ".") {
		return false
	}
	if ip := netguard.ParseIP(host); ip != nil {
		return !netguard.IsPrivateOrReserved(ip)
	}
	return true
}
func opencodeServerEncodeCall(id, name, arguments string, result bool) string {
	envelope := map[string]any{"id": id}
	if result {
		envelope = map[string]any{"call_id": id, "content": arguments}
	} else {
		envelope["name"] = name
		if arguments != "" {
			envelope["arguments"] = jsontext.Value(arguments)
		}
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return ""
	}
	open, close := opencodeServerCallOpen, opencodeServerCallClose
	if result {
		open, close = opencodeServerToolOpen, opencodeServerToolClose
	}
	return open + string(encoded) + close
}

// opencodeServerExtractCalls turns the prompt envelope back into OpenAI tool
// calls and removes it from the visible answer.
func opencodeServerExtractCalls(text string) (string, []translator.OpenAIToolCall) {
	var calls []translator.OpenAIToolCall
	var clean strings.Builder
	for text != "" {
		start := strings.Index(text, opencodeServerCallOpen)
		if start < 0 {
			clean.WriteString(text)
			break
		}
		clean.WriteString(text[:start])
		rest := text[start+len(opencodeServerCallOpen):]
		end := strings.Index(rest, opencodeServerCallClose)
		if end < 0 {
			clean.WriteString(opencodeServerCallOpen)
			text = rest
			continue
		}
		var envelope opencodeServerToolCall
		if json.Unmarshal([]byte(rest[:end]), &envelope) == nil && envelope.Name != "" {
			args := string(envelope.Arguments)
			if args == "" {
				args = "{}"
			}
			calls = append(calls, translator.OpenAIToolCall{
				ID: envelope.ID, Type: "function",
				Function: translator.OpenAIFunctionCall{Name: envelope.Name, Arguments: args},
			})
		}
		text = rest[end+len(opencodeServerCallClose):]
	}
	return strings.TrimSpace(clean.String()), calls
}
func opencodeServerTitle(messages []opencodeServerChatMessage) string {
	for _, msg := range messages {
		if msg.Role != "user" {
			continue
		}
		text := opencodeServerContentText(msg.Content, new([]map[string]any))
		if text = strings.TrimSpace(text); text != "" {
			if len(text) > 80 {
				text = text[:80]
			}
			return text
		}
	}
	return "9RouterX"
}
