package executor

import (
	json "encoding/json/v2"
	"fmt"
	"strings"
	"time"
)

type chatplaygroundModelData struct {
	ID        string
	Name      string
	ModelName string
	Provider  string
	Endpoint  string
}

var chatplaygroundModels = []chatplaygroundModelData{
	{"gpt-6.1-sol", "ChatGPT-6.1 Sol", "gpt-6.1-sol", "OpenAI", "azure"},
	{"gpt-6-luna", "ChatGPT-6 Luna", "gpt-6-luna", "OpenAI", "azure"},
	{"gemini-3.8-flash-l", "Gemini (Latest Model)", "gemini-3.8-flash", "google", "azure"},
	{"gemini-3.8-flash", "Gemini 3.8 Flash", "gemini-3.8-flash", "google", "azure"},
	{"gemini-3.1-pro", "Gemini 3.1 Pro", "gemini-3.1-pro-preview", "google", "azure"},
	{"claude-opus-5-5", "Claude Opus 5.5", "claude-opus-5-5", "anthropic", "azure"},
	{"claude-haiku-5-5", "Claude (Latest Model)", "claude-haiku-5-5", "anthropic", "azure"},
	{"claude-sonnet-5-5", "Claude Sonnet 5.5", "claude-sonnet-5-5", "anthropic", "azure"},
	{"claude-fable-5-1", "Claude Fable 5.1", "claude-fable-5-1", "anthropic", "azure"},
	{"kimi-k2.6", "Kimi K", "kimi-k2.6", "kimi", "azure"},
	{"deepseek-v4-pro", "DeepSeek V4 Pro", "deepseek-v4-pro", "deepseek", "azure"},
	{"deepseek-v4-flash", "DeepSeek V4 Flash", "deepseek-v4-flash", "deepseek", "azure"},
	{"grok-4.6", "Grok 4.6", "grok-4.6", "xAI", "azure"},
	{"mistral-large-3", "Mistral Large 3", "mistral-large-3", "mistral", "azure"},
	{"kimi-k3", "Kimi K3", "kimi-k3", "kimi", "lmsys"},
	{"deepseek-r1", "DeepSeek R1", "deepseek/deepseek-r1-0528", "deepseek", "lmsys"},
	{"llama-4-maverick", "Llama 4 Maverick", "meta-llama/llama-4-maverick", "meta", "lmsys"},
	{"llama-4-scout", "Llama 4 Scout", "meta-llama/llama-4-scout-17b-16e-instruct", "meta", "lmsys"},
	{"command-a", "Command A", "cohere/command-a", "Cohere", "lmsys"},
	{"command-r-plus", "Command R Plus", "cohere/command-r-plus-08-2024", "cohere", "lmsys"},
	{"minimax-m3", "MiniMax M3", "minimax/minimax-m3", "MiniMax", "lmsys"},
	{"qwen3.8-max", "Qwen 3.8 Max", "qwen/qwen3.8-max", "Qwen", "lmsys"},
	{"qwen3.7-plus", "Qwen 3.7 Plus", "qwen/qwen3.7-plus", "Qwen", "lmsys"},
	{"qwen3.6-flash", "Qwen 3.6 Flash", "qwen/qwen3.6-flash", "Qwen", "lmsys"},
	{"nova-2-lite-v1", "Nova 2 Lite", "amazon/nova-2-lite-v1", "Amazon", "lmsys"},
	{"mimo-v2.6-pro", "MiMo V2.6 Pro", "xiaomi/mimo-v2.6-pro", "Xiaomi", "lmsys"},
	{"hy3", "HY3", "tencent/hy3", "Tencent", "lmsys"},
	{"grok-4.7", "Grok 4.7", "grok-4.7", "xAI", "azure"},
	{"glm-5.3", "GLM 5.3", "z-ai/glm-5.3", "XAi", "lmsys"},
	{"glm-5.3-flash", "GLM 5.3 Flash", "z-ai/glm-5.3-flash", "ZAi", "lmsys"},
	{"perplexity-sonar-reasoning-pro", "Sonar Reasoning Pro", "sonar-reasoning-pro", "perplexity", "perplexity"},
	{"perplexity-sonar-pro", "Sonar Pro", "sonar-pro", "perplexity", "perplexity"},
	{"perplexity-sonar", "Sonar", "sonar", "perplexity", "perplexity"},
}

func chatplaygroundModel(requested string, body []byte) (chatplaygroundModelData, bool) {
	raw := strings.TrimSpace(requested)
	if raw == "" {
		var parsed struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(body, &parsed) == nil {
			raw = strings.TrimSpace(parsed.Model)
		}
	}
	raw = strings.TrimPrefix(raw, "chatplayground/")
	raw = strings.TrimPrefix(raw, "cpl/")
	needle := strings.ToLower(raw)
	if needle == "" {
		return chatplaygroundModelData{}, false
	}
	for _, model := range chatplaygroundModels {
		if strings.EqualFold(model.ID, needle) || strings.EqualFold(model.ModelName, needle) || strings.EqualFold(model.Name, needle) {
			return model, true
		}
	}
	if idx := strings.LastIndexByte(needle, '/'); idx >= 0 {
		needle = needle[idx+1:]
		for _, model := range chatplaygroundModels {
			if strings.EqualFold(model.ID, needle) || strings.EqualFold(model.ModelName, needle) || strings.EqualFold(model.Name, needle) {
				return model, true
			}
		}
	}
	return chatplaygroundModelData{}, false
}

func chatplaygroundPayload(body []byte, model chatplaygroundModelData) ([]byte, error) {
	var request struct {
		Messages []any  `json:"messages"`
		ChatID   string `json:"chatId"`
	}
	if json.Unmarshal(body, &request) != nil {
		return nil, fmt.Errorf("decode ChatPlayground request")
	}
	for _, raw := range request.Messages {
		message, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if text, ok := message["content"].(string); ok && len(text) > chatplaygroundMaxMessage {
			return nil, fmt.Errorf("message exceeds ChatPlayground character limit")
		}
	}
	var payload map[string]any
	switch model.Endpoint {
	case "perplexity":
		payload = map[string]any{"messages": request.Messages, "modelName": model.ModelName, "botId": model.ID}
	case "lmsys":
		payload = map[string]any{"messages": request.Messages, "model": model.ModelName, "botId": model.ID}
	default:
		upstream := model.ModelName
		if model.Provider != "" {
			upstream = strings.ToLower(model.Provider) + "/" + model.ModelName
		}
		payload = map[string]any{
			"messages": request.Messages, "model": upstream,
			"chatId": request.ChatID, "isRegenerate": false, "promptTemplate": nil,
			"fileUrl": nil, "botId": model.ID, "submissionId": fmt.Sprintf("cpl-%x", time.Now().UnixNano()), "noSave": true,
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode ChatPlayground payload")
	}
	return encoded, nil
}

func chatplaygroundStripID(text string, trim bool) string {
	cleaned := strings.ReplaceAll(text, "\u200b", "")
	for {
		start := strings.Index(cleaned, "CHAT_ID:")
		if start < 0 {
			break
		}
		end := start + len("CHAT_ID:")
		for end < len(cleaned) {
			ch := cleaned[end]
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-' {
				end++
				continue
			}
			break
		}
		cleaned = cleaned[:start] + cleaned[end:]
	}
	if trim {
		return strings.TrimSpace(cleaned)
	}
	return cleaned
}

func chatplaygroundChunk(id, model, content string, created int64, finish string) string {
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

func chatplaygroundFrames(content, id, model string, created int64) string {
	var out strings.Builder
	if content != "" {
		out.WriteString(chatplaygroundChunk(id, model, content, created, ""))
	}
	out.WriteString(chatplaygroundChunk(id, model, "", created, "stop"))
	out.WriteString("data: [DONE]\n\n")
	return out.String()
}

func chatplaygroundCompletion(content, id, model string, created int64) []byte {
	completion := map[string]any{
		"id": id, "object": "chat.completion", "created": created, "model": model,
		"choices": []any{map[string]any{
			"index": 0, "message": map[string]any{"role": "assistant", "content": content}, "finish_reason": "stop",
		}},
		"usage": map[string]any{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
	}
	encoded, _ := json.Marshal(completion)
	return encoded
}
