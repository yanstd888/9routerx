package executor

import (
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"strings"

	"9routerx/proxy/internal/thinking"
)

// ForwardMoonshot receives prepared Chat Completions input; client bridges remain
// owned by ForwardOpenAI, including Claude and Responses clients.
func ForwardMoonshot(w http.ResponseWriter, req *Request) error {
	body, err := NormalizeMoonshotRequest(req.Body, req.IsStream, req.ConnData)
	if err != nil {
		return fmt.Errorf("normalize moonshot request: %w", err)
	}
	local := *req
	local.Body = body
	cfg := *req.Config
	cfg.StaticHeaders = make(map[string]string, len(req.Config.StaticHeaders)+1)
	for key, value := range req.Config.StaticHeaders {
		cfg.StaticHeaders[key] = value
	}
	if !req.IsStream {
		cfg.StaticHeaders["Accept"] = "application/json"
	}
	local.Config = &cfg
	return ForwardOpenAI(w, &local)
}

// NormalizeMoonshotRequest follows OmniRoute 632d84f5 executors/moonshot.ts.
// Invalid K3.1 efforts stay on the wire so the provider's rejection is visible.
func NormalizeMoonshotRequest(body []byte, stream bool, data map[string]any) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode chat body: %w", err)
	}
	model, _ := payload["model"].(string)
	model = thinking.StripSuffix(strings.TrimPrefix(model, "moonshot/"))
	if model == "" {
		return nil, fmt.Errorf("missing model")
	}
	payload["model"] = model
	payload["stream"] = stream
	if stream && data["disableStreamOptions"] != true {
		options, _ := payload["stream_options"].(map[string]any)
		if options == nil {
			options = map[string]any{}
		}
		options["include_usage"] = true
		payload["stream_options"] = options
	} else {
		delete(payload, "stream_options")
	}
	moonshotToolNames(payload)
	normalized := strings.ToLower(model)
	// K3 aliases retain their distinct wire ids, but share sampling restrictions.
	k3Alias := normalized == "k3" || normalized == "k3-256k"
	if strings.HasPrefix(normalized, "kimi-") || k3Alias {
		moonshotSanitizeTools(payload)
		moonshotRootToolUnions(payload)
		k3 := k3Alias || moonshotFamily(normalized, "kimi-k3", ".-")
		k31 := moonshotFamily(normalized, "kimi-k3.1", "-")
		k27 := moonshotFamily(normalized, "kimi-k2.7-code", "-")
		k26 := moonshotFamily(normalized, "kimi-k2.6", "-")
		ceiling := 262144.0
		if k3 {
			ceiling = 1048576
		}
		if value, exists := payload["max_tokens"]; exists {
			if _, supplied := payload["max_completion_tokens"]; !supplied {
				payload["max_completion_tokens"] = value
			}
			delete(payload, "max_tokens")
		}
		if value, ok := payload["max_completion_tokens"].(float64); ok && value > ceiling {
			payload["max_completion_tokens"] = ceiling
		}
		delete(payload, "temperature")
		if k3 || k27 || k26 {
			for _, key := range []string{"top_p", "frequency_penalty", "presence_penalty", "n", "logprobs", "top_logprobs"} {
				delete(payload, key)
			}
		}
		switch {
		case k31:
			moonshotK31Thinking(payload)
		case k3:
			moonshotStripThinking(payload)
			payload["reasoning_effort"] = "max"
		default:
			moonshotK2Thinking(payload, k27)
			if (k27 || k26) && payload["tool_choice"] == "required" {
				payload["tool_choice"] = "auto"
			}
		}
		if !k3 && !k27 {
			moonshotLegacyReasoning(payload)
		}
	}
	out, err := json.Marshal(payload, json.Deterministic(true))
	if err != nil {
		return nil, fmt.Errorf("encode chat body: %w", err)
	}
	return out, nil
}

func moonshotFamily(model, family, separators string) bool {
	return model == family || strings.HasPrefix(model, family) && len(model) > len(family) && strings.ContainsRune(separators, rune(model[len(family)]))
}

func moonshotStripThinking(body map[string]any) {
	delete(body, "thinking")
	delete(body, "reasoning")
	delete(body, "enable_thinking")
}

func moonshotK31Thinking(body map[string]any) {
	effort, supplied := body["reasoning_effort"]
	if !supplied {
		if reasoning, ok := body["reasoning"].(map[string]any); ok {
			effort, supplied = reasoning["effort"]
		}
	}
	thinking, _ := body["thinking"].(map[string]any)
	if !supplied {
		effort, supplied = thinking["effort"]
	}
	if !supplied {
		effort = "max"
		if body["enable_thinking"] == false || thinking["type"] == "disabled" {
			effort = "none"
		}
	}
	if value, ok := effort.(string); ok {
		aliases := map[string]string{"ultra": "max", "xhigh": "max", "medium": "high", "minimum": "low", "light": "low", "low": "low", "high": "high", "max": "max"}
		if mapped, exists := aliases[strings.ToLower(value)]; exists {
			effort = mapped
		}
	}
	moonshotStripThinking(body)
	body["reasoning_effort"] = effort
}

func moonshotK2Thinking(body map[string]any, preserved bool) {
	existing, _ := body["thinking"].(map[string]any)
	reasoning, _ := body["reasoning"].(map[string]any)
	effort := body["reasoning_effort"]
	if effort == nil {
		effort = reasoning["effort"]
	}
	requested, _ := effort.(string)
	requested = strings.ToLower(requested)
	disabled := requested == "none" || requested == "off" || body["enable_thinking"] == false || existing["type"] == "disabled"
	enabled := requested != "" || body["enable_thinking"] == true || existing["type"] == "enabled" || existing["type"] == "adaptive" || existing["keep"] == "all"
	moonshotStripThinking(body)
	delete(body, "reasoning_effort")
	switch {
	case preserved:
		body["thinking"] = map[string]any{"type": "enabled", "keep": "all"}
	case disabled:
		body["thinking"] = map[string]any{"type": "disabled"}
	case enabled:
		value := map[string]any{"type": "enabled"}
		if existing["keep"] == "all" {
			value["keep"] = "all"
		}
		body["thinking"] = value
	}
}

func moonshotLegacyReasoning(body map[string]any) {
	messages, _ := body["messages"].([]any)
	for _, raw := range messages {
		message, ok := raw.(map[string]any)
		if !ok || message["role"] != "assistant" || moonshotReasoningCarrier(message["reasoning_content"]) || moonshotReasoningCarrier(message["reasoning"]) {
			continue
		}
		message["reasoning_content"] = " "
	}
}

func moonshotReasoningCarrier(value any) bool {
	switch content := value.(type) {
	case string:
		return strings.TrimSpace(content) != ""
	case []any:
		return len(content) > 0
	default:
		return false
	}
}
